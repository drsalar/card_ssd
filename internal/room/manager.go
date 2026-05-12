// Package room 房间管理器
// manager.go: 全局房间集合 + 创建/加入/离开/销毁/断线判定
package room

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"card_ssd/internal/game"
	"card_ssd/internal/logger"
	"card_ssd/internal/session"
)

// JoinError 加入房间错误码
type JoinError string

const (
	JoinErrNotFound JoinError = "ROOM_NOT_FOUND"
	JoinErrFull     JoinError = "ROOM_FULL"
	JoinErrPlaying  JoinError = "ROOM_PLAYING"
)

// JoinResult 加入结果
type JoinResult struct {
	Room      *Room
	Player    *Player
	Reconnect bool
	Err       JoinError
}

// LeaveResult 离开结果
type LeaveResult struct {
	Room      *Room
	Destroyed bool
}

// OfflineTimeout 掉线判定时长
const OfflineTimeout = 30 * time.Second

// AutoSettleHook 自动结算钩子（断线超时兜底后调用，由 game handler 注入）
// 入参为已加锁状态外的房间对象，钩子内部需自行判断是否触发结算
type AutoSettleHook func(r *Room)

var (
	rmu            sync.Mutex
	rooms          = make(map[string]*Room)
	offlineTimers  = make(map[string]*time.Timer) // openid -> timer
	autoSettleHook AutoSettleHook
	earlyEndHook   AutoSettleHook // 投票解散提前结算钩子（由 game handler 注入）
)

// SetAutoSettleHook 注入自动结算钩子
func SetAutoSettleHook(h AutoSettleHook) {
	rmu.Lock()
	autoSettleHook = h
	rmu.Unlock()
}

// SetEarlyEndHook 注入提前结算钩子（投票解散场景使用）
func SetEarlyEndHook(h AutoSettleHook) {
	rmu.Lock()
	earlyEndHook = h
	rmu.Unlock()
}

// genRoomID 生成唯一 4 位房间 ID（已被占用则重试，最多 1000 次后退化为 5 位）
func genRoomID() string {
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("%04d", 1000+rand.Intn(9000))
		if _, ok := rooms[id]; !ok {
			return id
		}
	}
	return fmt.Sprintf("%05d", 10000+rand.Intn(90000))
}

// CreateRoom 创建房间
func CreateRoom(rule Rule, hostOpenid string) *Room {
	rmu.Lock()
	defer rmu.Unlock()
	id := genRoomID()
	r := NewRoom(id, rule, hostOpenid)
	rooms[id] = r
	logger.Info("创建房间 %s 房主=%s", id, hostOpenid)
	return r
}

// GetRoom 获取房间
func GetRoom(id string) *Room {
	rmu.Lock()
	defer rmu.Unlock()
	return rooms[id]
}

// DestroyRoom 销毁房间
func DestroyRoom(id string) {
	rmu.Lock()
	r, ok := rooms[id]
	if ok {
		delete(rooms, id)
	}
	rmu.Unlock()
	if !ok || r == nil {
		return
	}
	// 停止该房间上所有 bot 定时器与投票定时器，避免销毁后继续触发动作
	r.Lock()
	r.MarkDestroyed()
	r.CancelBotTimers()
	r.CancelVoteTimer()
	r.Unlock()
	logger.Info("销毁房间 %s", id)
}

// CountRooms 当前房间数
func CountRooms() int {
	rmu.Lock()
	defer rmu.Unlock()
	return len(rooms)
}

// JoinRoom 玩家加入房间
func JoinRoom(roomID string, s *session.Session) JoinResult {
	r := GetRoom(roomID)
	if r == nil {
		return JoinResult{Err: JoinErrNotFound}
	}
	r.Lock()
	defer r.Unlock()
	// 已存在玩家 → 视为重连
	if r.GetPlayer(s.Openid) != nil {
		p := r.ReconnectPlayer(s)
		cancelOfflineTimer(s.Openid)
		s.RoomID = r.ID
		return JoinResult{Room: r, Player: p, Reconnect: true}
	}
	if r.IsFull() {
		return JoinResult{Err: JoinErrFull}
	}
	if r.Phase != PhaseWaiting && r.Phase != PhaseMatchEnd {
		return JoinResult{Err: JoinErrPlaying}
	}
	p := r.AddPlayer(s)
	s.RoomID = r.ID
	return JoinResult{Room: r, Player: p, Reconnect: false}
}

// LeaveRoom 玩家主动离开
func LeaveRoom(s *session.Session) *LeaveResult {
	if s.RoomID == "" {
		return nil
	}
	r := GetRoom(s.RoomID)
	s.RoomID = ""
	if r == nil {
		return nil
	}
	r.Lock()
	r.RemovePlayer(s.Openid)
	cancelOfflineTimer(s.Openid)
	// 房间需销毁的条件是”无任何真人玩家“（只剩 bot 也需销毁）
	needDestroy := r.HumanCount() == 0
	if !needDestroy {
		r.BroadcastState()
	}
	r.Unlock()
	if needDestroy {
		DestroyRoom(r.ID)
		return &LeaveResult{Room: r, Destroyed: true}
	}
	return &LeaveResult{Room: r, Destroyed: false}
}

// markAllOfflineIfNeeded 评估并更新房间的 AllOfflineSince 时间戳
// 调用前应已持有 r.mu。仅 Phase 为 playing/comparing/match_end 且无在线真人时记录起始时间，
// 任意一名真人重连 / 有玩家在线时清零
func markAllOfflineIfNeeded(r *Room) {
	if r == nil {
		return
	}
	// waiting 阶段沉默：调用者主动在 waiting 阶段清零不一定需要处理，这里依据在线状况统一设置
	if r.Phase == PhaseWaiting {
		r.AllOfflineSince = 0
		return
	}
	if hasOnlineHuman(r) {
		r.AllOfflineSince = 0
		return
	}
	// 仅在未设置起始时间时设置，避免重复覆盖导致超时计算偏迟
	if r.AllOfflineSince == 0 {
		r.AllOfflineSince = time.Now().UnixMilli()
	}
}

// HandleDisconnect 处理断线
// waiting 阶段直接移除；游戏阶段标记掉线 + 30 秒计时器兌底
// 顶号场景：新连接已绑定同 openid，旧连接被关闭触发本函数，需跳过以免误标 Offline
func HandleDisconnect(s *session.Session) {
	if s.RoomID == "" {
		return
	}
	// 判断是否被顶号：当前 openid 在会话表中如果不是本 session，说明已有新连接
	if s.Openid != "" {
		cur := session.GetByOpenid(s.Openid)
		if cur != nil && cur != s {
			return
		}
	}
	r := GetRoom(s.RoomID)
	if r == nil {
		return
	}
	r.Lock()
	p := r.GetPlayer(s.Openid)
	if p == nil {
		r.Unlock()
		return
	}
	if r.Phase == PhaseWaiting {
		r.RemovePlayer(s.Openid)
		needDestroy := r.HumanCount() == 0
		if !needDestroy {
			r.BroadcastState()
		}
		r.Unlock()
		if needDestroy {
			DestroyRoom(r.ID)
		}
		return
	}
	// 对局中标记掉线
	p.Offline = true
	p.OfflineSince = time.Now().UnixMilli()
	markAllOfflineIfNeeded(r)
	r.BroadcastState()
	r.Unlock()

	// 启动 30 秒兜底计时器
	cancelOfflineTimer(s.Openid)
	openid := s.Openid
	roomID := r.ID
	timer := time.AfterFunc(OfflineTimeout, func() {
		rmu.Lock()
		delete(offlineTimers, openid)
		rmu.Unlock()
		rr := GetRoom(roomID)
		if rr == nil {
			return
		}
		rr.Lock()
		pp := rr.GetPlayer(openid)
		if pp == nil || !pp.Offline {
			rr.Unlock()
			return
		}
		// 根据阶段做不同的兌底处理：
		// - Playing：自动按头 3/中 5/尾 5 提交（视为散牌），并视情况触发结算
		// - Waiting / MatchEnd：玩家长时间未回，直接踢出避免永久占座
		// - Comparing：保持 Offline 占座（积分已结算，整场内座次不变），不做处理
		needDestroy := false
		needSettle := false
		switch rr.Phase {
		case PhasePlaying:
			if !pp.Submitted && len(pp.Hand) == 13 {
				pp.Lanes = autoSplitLanes(pp.Hand)
				pp.Submitted = true
			}
			needSettle = rr.AllSubmitted()
		case PhaseWaiting, PhaseMatchEnd:
			rr.RemovePlayer(openid)
			needDestroy = rr.HumanCount() == 0
		}
		// waiting 阶段仍然在上面踢出路径里销毁；其他阶段改为登记 AllOfflineSince 交给 24h 巡检处理
		if !needDestroy {
			markAllOfflineIfNeeded(rr)
		}
		// 投票提前结算：如果有玩家被踢出后、仍在场的真人玩家全部已同意解散，需触发提前结算
		needEarlyEnd := false
		if !needDestroy && checkAllHumansVoted(rr) {
			needEarlyEnd = true
		}
		if !needDestroy {
			rr.BroadcastState()
		}
		hook := autoSettleHook
		endHook := earlyEndHook
		rr.Unlock()
		if needDestroy {
			DestroyRoom(roomID)
			return
		}
		if needEarlyEnd && endHook != nil {
			endHook(rr)
			return
		}
		if needSettle && hook != nil {
			hook(rr)
		}
	})
	rmu.Lock()
	offlineTimers[openid] = timer
	rmu.Unlock()
}

// CancelOfflineTimer 公开版本（便于外部调用）
func CancelOfflineTimer(openid string) {
	cancelOfflineTimer(openid)
}

func cancelOfflineTimer(openid string) {
	rmu.Lock()
	t, ok := offlineTimers[openid]
	if ok {
		delete(offlineTimers, openid)
	}
	rmu.Unlock()
	if t != nil {
		t.Stop()
	}
}

// autoSplitLanes 简单地按头 3/中 5/尾 5 切分（散牌兜底）
func autoSplitLanes(cards []game.Card) *game.Lanes {
	return &game.Lanes{
		Head:   cards[0:3],
		Middle: cards[3:8],
		Tail:   cards[8:13],
	}
}

// hasOnlineHuman 判断房间是否仍有在线真人玩家
// 调用前应已持有 r.mu
func hasOnlineHuman(r *Room) bool {
	for _, p := range r.Players {
		if !p.IsBot && !p.Offline {
			return true
		}
	}
	return false
}

// checkAllHumansVoted 检查房间内所有在线真人是否都已投票同意解散
// 调用前应已持有 r.mu。仅在 playing/comparing 阶段需要评估；其他阶段返回 false。
func checkAllHumansVoted(r *Room) bool {
	if r == nil {
		return false
	}
	if r.Phase != PhasePlaying && r.Phase != PhaseComparing {
		return false
	}
	humanOnline := 0
	for _, p := range r.Players {
		if p.IsBot || p.Offline {
			continue
		}
		humanOnline++
		if !p.VoteDissolve {
			return false
		}
	}
	return humanOnline > 0
}

// RoomSummary 活跃房间摘要（仅用于 HTTP 查询，不修改任何状态）
type RoomSummary struct {
	RoomID       string `json:"roomId"`
	Phase        string `json:"phase"`
	CurrentRound int    `json:"currentRound"`
	TotalRounds  int    `json:"totalRounds"`
	MaxPlayers   int    `json:"maxPlayers"`
}

// FindActiveRoomByOpenid 查询某 openid 当前所在的未结束房间
// 该方法严格只读：不修改任何 Session 与 Player 的网络状态
// 命中条件：房间存在、未销毁、且玩家仍在 Players 列表内
// 多个候选时优先返回 LastActiveAt 最新的一个；时间戳相同时按 RoomID 字典序兑底
func FindActiveRoomByOpenid(openid string) *RoomSummary {
	if openid == "" {
		return nil
	}
	rmu.Lock()
	// 复制候选房间，避免在 rmu 之内再加 r.mu 形成嵌套锁顺序问题
	candidates := make([]*Room, 0, len(rooms))
	for _, r := range rooms {
		candidates = append(candidates, r)
	}
	rmu.Unlock()
	type hit struct {
		summary *RoomSummary
		active  int64
	}
	hits := make([]hit, 0, 4)
	for _, r := range candidates {
		r.Lock()
		if r.IsDestroyed() {
			r.Unlock()
			continue
		}
		if r.GetPlayer(openid) == nil {
			r.Unlock()
			continue
		}
		hits = append(hits, hit{
			summary: &RoomSummary{
				RoomID:       r.ID,
				Phase:        string(r.Phase),
				CurrentRound: r.CurrentRound,
				TotalRounds:  r.Rule.TotalRounds,
				MaxPlayers:   r.Rule.MaxPlayers,
			},
			active: r.LastActiveAt,
		})
		r.Unlock()
	}
	if len(hits) == 0 {
		return nil
	}
	best := hits[0]
	for i := 1; i < len(hits); i++ {
		h := hits[i]
		if h.active > best.active || (h.active == best.active && h.summary.RoomID < best.summary.RoomID) {
			best = h
		}
	}
	return best.summary
}
