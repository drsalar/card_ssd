// Package room 房间模型
// room.go: 单个房间的数据结构与行为
package room

import (
	"sync"
	"time"

	"card_ssd/internal/game"
	"card_ssd/internal/protocol"
	"card_ssd/internal/session"
)

// Phase 房间阶段
type Phase string

const (
	PhaseWaiting   Phase = "waiting"   // 等待准备
	PhasePlaying   Phase = "playing"   // 已发牌、放牌中
	PhaseComparing Phase = "comparing" // 比牌动画中
	PhaseMatchEnd  Phase = "match_end" // 整场结束
)

// Rule 房间规则
type Rule struct {
	WithMa      bool `json:"withMa"`
	TotalRounds int  `json:"totalRounds"`
	MaxPlayers  int  `json:"maxPlayers"`
}

// Player 房间内玩家
type Player struct {
	Openid         string
	Nickname       string
	AvatarUrl      string
	ConnID         int64
	Score          int
	Ready          bool
	Offline        bool
	OfflineSince   int64
	Hand           []game.Card
	Lanes          *game.Lanes
	Submitted      bool
	RoundConfirmed bool
	IsBot          bool // 是否电脑玩家
}

// Room 房间
// 通过 mu 实现房间级互斥，保证并发访问下状态一致
type Room struct {
	ID           string
	Rule         Rule
	HostID       string
	Players      []*Player
	Phase        Phase
	CurrentRound int
	LastSettle   *game.SettleResult

	// botSeq 用于生成电脑玩家序号（电脑1、电脑2 ...）
	botSeq int
	// botTimers 房间内挂起的电脑玩家定时器，房间销毁时统一停止
	botTimers []*time.Timer
	// destroyed 房间销毁标记，避免回调在销毁后继续执行
	destroyed bool

	mu sync.Mutex
}

// NewRoom 创建房间实例
func NewRoom(id string, rule Rule, hostOpenid string) *Room {
	if rule.MaxPlayers < 2 {
		rule.MaxPlayers = 2
	}
	if rule.MaxPlayers > 6 {
		rule.MaxPlayers = 6
	}
	if rule.TotalRounds <= 0 {
		rule.TotalRounds = 5
	}
	return &Room{
		ID:      id,
		Rule:    rule,
		HostID:  hostOpenid,
		Players: make([]*Player, 0, rule.MaxPlayers),
		Phase:   PhaseWaiting,
	}
}

// Lock/Unlock 暴露给 handler 加锁（多步骤操作时使用）
func (r *Room) Lock()   { r.mu.Lock() }
func (r *Room) Unlock() { r.mu.Unlock() }

// IsFull 是否已满
func (r *Room) IsFull() bool { return len(r.Players) >= r.Rule.MaxPlayers }

// IsEmpty 是否为空
func (r *Room) IsEmpty() bool { return len(r.Players) == 0 }

// GetPlayer 通过 openid 查找
func (r *Room) GetPlayer(openid string) *Player {
	for _, p := range r.Players {
		if p.Openid == openid {
			return p
		}
	}
	return nil
}

// AddPlayer 加入新玩家（来自 session）
func (r *Room) AddPlayer(s *session.Session) *Player {
	p := &Player{
		Openid:    s.Openid,
		Nickname:  s.Nickname,
		AvatarUrl: s.AvatarUrl,
		ConnID:    s.ConnID,
	}
	r.Players = append(r.Players, p)
	return p
}

// ReconnectPlayer 玩家重连：刷新连接信息
func (r *Room) ReconnectPlayer(s *session.Session) *Player {
	p := r.GetPlayer(s.Openid)
	if p == nil {
		return nil
	}
	p.ConnID = s.ConnID
	if s.Nickname != "" {
		p.Nickname = s.Nickname
	}
	if s.AvatarUrl != "" {
		p.AvatarUrl = s.AvatarUrl
	}
	p.Offline = false
	p.OfflineSince = 0
	return p
}

// RemovePlayer 移除玩家，返回被移除的 Player（房主转移在此处理）
func (r *Room) RemovePlayer(openid string) *Player {
	for i, p := range r.Players {
		if p.Openid == openid {
			r.Players = append(r.Players[:i], r.Players[i+1:]...)
			if r.HostID == openid && len(r.Players) > 0 {
				r.HostID = r.Players[0].Openid
			}
			return p
		}
	}
	return nil
}

// AllReady 是否全员就绪（≥2 人）
func (r *Room) AllReady() bool {
	if len(r.Players) < 2 {
		return false
	}
	for _, p := range r.Players {
		if !p.Ready && !p.Offline {
			return false
		}
	}
	return true
}

// AllSubmitted 是否全员开牌
func (r *Room) AllSubmitted() bool {
	for _, p := range r.Players {
		if !p.Submitted && !p.Offline {
			return false
		}
	}
	return true
}

// AllRoundConfirmed 是否全员确认结算
func (r *Room) AllRoundConfirmed() bool {
	for _, p := range r.Players {
		if !p.RoundConfirmed && !p.Offline {
			return false
		}
	}
	return true
}

// ResetRound 重置一局相关数据（保留 score）
func (r *Room) ResetRound() {
	for _, p := range r.Players {
		p.Hand = nil
		p.Lanes = nil
		p.Submitted = false
		p.Ready = false
		p.RoundConfirmed = false
	}
	r.LastSettle = nil
}

// PlayerState 房间状态广播中的玩家信息
type PlayerState struct {
	Openid    string `json:"openid"`
	Nickname  string `json:"nickname"`
	AvatarUrl string `json:"avatarUrl"`
	Score     int    `json:"score"`
	Ready     bool   `json:"ready"`
	Offline   bool   `json:"offline"`
	Submitted bool   `json:"submitted"`
	IsBot     bool   `json:"isBot"`
}

// State ROOM_STATE 的载荷
type State struct {
	ID           string        `json:"id"`
	Rule         Rule          `json:"rule"`
	HostID       string        `json:"hostId"`
	Phase        Phase         `json:"phase"`
	CurrentRound int           `json:"currentRound"`
	Players      []PlayerState `json:"players"`
}

// ToState 序列化为 ROOM_STATE
func (r *Room) ToState() State {
	players := make([]PlayerState, len(r.Players))
	for i, p := range r.Players {
		players[i] = PlayerState{
			Openid:    p.Openid,
			Nickname:  p.Nickname,
			AvatarUrl: p.AvatarUrl,
			Score:     p.Score,
			Ready:     p.Ready,
			Offline:   p.Offline,
			Submitted: p.Submitted,
			IsBot:     p.IsBot,
		}
	}
	return State{
		ID:           r.ID,
		Rule:         r.Rule,
		HostID:       r.HostID,
		Phase:        r.Phase,
		CurrentRound: r.CurrentRound,
		Players:      players,
	}
}

// Broadcast 广播消息给所有在线玩家（电脑玩家不需要发消息，会被自动跳过）
func (r *Room) Broadcast(msgType string, data any, exceptOpenid string) {
	for _, p := range r.Players {
		if p.Openid == exceptOpenid {
			continue
		}
		if p.Offline {
			continue
		}
		if p.IsBot {
			continue
		}
		s := session.GetByOpenid(p.Openid)
		if s != nil {
			s.Send(msgType, data, nil)
		}
	}
}

// BroadcastState 广播 ROOM_STATE
func (r *Room) BroadcastState() {
	r.Broadcast(protocol.MsgRoomState, r.ToState(), "")
}

// =====================
// 电脑玩家（Bot）相关方法
// =====================

// AddBot 在房间内创建一个电脑玩家
// 调用前应当已加锁。返回新创建的 Player。
func (r *Room) AddBot() *Player {
	r.botSeq++
	openid := "bot_" + r.ID + "_" + itoaSimple(r.botSeq)
	nickname := "电脑" + itoaSimple(r.botSeq)
	p := &Player{
		Openid:    openid,
		Nickname:  nickname,
		AvatarUrl: "", // 默认头像由前端兜底渲染
		IsBot:     true,
	}
	r.Players = append(r.Players, p)
	return p
}

// RemoveBot 按 openid 移除电脑玩家。
// 返回是否成功移除。仅移除 IsBot=true 的玩家，避免误删真人。
func (r *Room) RemoveBot(openid string) bool {
	for i, p := range r.Players {
		if p.Openid == openid && p.IsBot {
			r.Players = append(r.Players[:i], r.Players[i+1:]...)
			return true
		}
	}
	return false
}

// BotPlayers 返回房间内所有电脑玩家（按当前次序）
func (r *Room) BotPlayers() []*Player {
	out := make([]*Player, 0, len(r.Players))
	for _, p := range r.Players {
		if p.IsBot {
			out = append(out, p)
		}
	}
	return out
}

// HumanCount 返回房间内真人玩家数量（用于销毁判定）
func (r *Room) HumanCount() int {
	cnt := 0
	for _, p := range r.Players {
		if !p.IsBot {
			cnt++
		}
	}
	return cnt
}

// AddBotTimer 登记一个 bot 定时器，房间销毁时统一停止
func (r *Room) AddBotTimer(t *time.Timer) {
	if t == nil {
		return
	}
	r.botTimers = append(r.botTimers, t)
}

// CancelBotTimers 停止并清空房间内所有 bot 定时器
// 调用前应当已加锁。
func (r *Room) CancelBotTimers() {
	for _, t := range r.botTimers {
		if t != nil {
			t.Stop()
		}
	}
	r.botTimers = nil
}

// MarkDestroyed 设置房间销毁标记（在 Manager 销毁前调用）
func (r *Room) MarkDestroyed() { r.destroyed = true }

// IsDestroyed 是否已销毁
func (r *Room) IsDestroyed() bool { return r.destroyed }

// itoaSimple 简单整数转字符串（避免引入 strconv）
func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [12]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
