// Package handler 投票解散对局相关 handler
// vote.go: 处理 VOTE_DISSOLVE / VOTE_DISSOLVE_CANCEL，以及全员同意后的提前结算切换
package handler

import (
	"encoding/json"
	"time"

	"card_ssd/internal/logger"
	"card_ssd/internal/protocol"
	"card_ssd/internal/room"
	"card_ssd/internal/session"
)

// voteDissolveTimeout 投票解散一次性倒计时时长（60 秒）
const voteDissolveTimeout = 60 * time.Second

// InitVoteHandler 在 server 启动时注入“提前结算”钩子
// 该钩子会被 manager.go 在“玩家被踢出/掉线后判定全员同意时”反向调用
func InitVoteHandler() {
	room.SetEarlyEndHook(triggerEarlyMatchEnd)
}

// HandleVoteDissolve 处理投票解散请求
// 仅 playing/comparing 阶段允许；全员同意时立即触发提前结算
func HandleVoteDissolve(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	r.Lock()
	if r.Phase != room.PhasePlaying && r.Phase != room.PhaseComparing {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "当前阶段不允许发起解散", reqID)
		return
	}
	p := r.GetPlayer(s.Openid)
	if p == nil {
		r.Unlock()
		s.SendError(protocol.ErrNotInRoom, "玩家不存在", reqID)
		return
	}
	if p.IsBot {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "电脑玩家无需投票", reqID)
		return
	}
	if !p.VoteDissolve {
		p.VoteDissolve = true
		logger.Info("房间 %s 玩家 %s 同意解散对局", r.ID, s.Openid)
	}
	// 首次发起：启动 60 秒兜底计时器
	startVoteTimerLocked(r)
	allVoted := allHumansVotedLocked(r)
	r.BroadcastState()
	r.Unlock()
	if allVoted {
		triggerEarlyMatchEnd(r)
	}
}

// HandleVoteDissolveCancel 处理撤销投票
func HandleVoteDissolveCancel(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	r.Lock()
	if r.Phase != room.PhasePlaying && r.Phase != room.PhaseComparing {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "当前阶段无法撤销", reqID)
		return
	}
	p := r.GetPlayer(s.Openid)
	if p == nil {
		r.Unlock()
		s.SendError(protocol.ErrNotInRoom, "玩家不存在", reqID)
		return
	}
	if p.VoteDissolve {
		p.VoteDissolve = false
		logger.Info("房间 %s 玩家 %s 撤销解散投票", r.ID, s.Openid)
	}
	// 若已无人投票则停止倒计时
	if !anyHumanVotedLocked(r) {
		r.CancelVoteTimer()
	}
	r.BroadcastState()
	r.Unlock()
}

// startVoteTimerLocked 启动一次 60 秒倒计时；若已有计时器在运行则保持不变
// 调用前应已持有 r.mu
func startVoteTimerLocked(r *room.Room) {
	// 这里通过一个不可见的小技巧：Room.SetVoteTimer 在 voteTimer 不为空时会先 Stop 再覆盖
	// 我们仅在“当前没有人投同意之前的快照里没有定时器”时启动；为简化并发，直接重置定时器
	roomID := r.ID
	timer := time.AfterFunc(voteDissolveTimeout, func() {
		onVoteTimeout(roomID)
	})
	r.SetVoteTimer(timer)
}

// onVoteTimeout 投票 60 秒到期：清空所有 VoteDissolve，广播一条提示
func onVoteTimeout(roomID string) {
	r := room.GetRoom(roomID)
	if r == nil {
		return
	}
	r.Lock()
	if r.IsDestroyed() {
		r.Unlock()
		return
	}
	if r.Phase != room.PhasePlaying && r.Phase != room.PhaseComparing {
		r.Unlock()
		return
	}
	cleared := false
	for _, p := range r.Players {
		if p.VoteDissolve {
			p.VoteDissolve = false
			cleared = true
		}
	}
	r.CancelVoteTimer()
	if cleared {
		r.BroadcastState()
		r.Broadcast(protocol.MsgVoteDissolveTimeout, map[string]any{}, "")
		logger.Info("房间 %s 解散投票 60 秒超时，已重置", r.ID)
	}
	r.Unlock()
}

// allHumansVotedLocked 房间内所有在线真人是否都已投票同意
// 调用前应已持有 r.mu
func allHumansVotedLocked(r *room.Room) bool {
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

// anyHumanVotedLocked 是否仍有任意真人维持已投状态
// 调用前应已持有 r.mu
func anyHumanVotedLocked(r *room.Room) bool {
	for _, p := range r.Players {
		if p.IsBot {
			continue
		}
		if p.VoteDissolve {
			return true
		}
	}
	return false
}

// triggerEarlyMatchEnd 触发提前结算：
//   - playing 阶段：跳过本局结算，直接进入 match_end，按当前 Score 排序广播
//   - comparing 阶段：保留本局结算结果（已发 SETTLE_RESULT），随后进入 match_end
func triggerEarlyMatchEnd(r *room.Room) {
	r.Lock()
	if r.IsDestroyed() {
		r.Unlock()
		return
	}
	if r.Phase != room.PhasePlaying && r.Phase != room.PhaseComparing {
		r.Unlock()
		return
	}
	r.Phase = room.PhaseMatchEnd
	r.CancelVoteTimer()
	// 同时取消 bot 的待执行动作，避免提前结算后还触发开牌/确认
	r.CancelBotTimers()
	r.Touch()
	// 按累计 Score 排序构造排行榜
	idx := make([]int, len(r.Players))
	for i := range idx {
		idx[i] = i
	}
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && r.Players[idx[j-1]].Score < r.Players[idx[j]].Score; j-- {
			idx[j-1], idx[j] = idx[j], idx[j-1]
		}
	}
	ranks := make([]map[string]any, len(r.Players))
	for i, k := range idx {
		p := r.Players[k]
		ranks[i] = map[string]any{
			"openid":   p.Openid,
			"nickname": p.Nickname,
			"score":    p.Score,
		}
	}
	roomID := r.ID
	r.Broadcast(protocol.MsgMatchEnd, map[string]any{"ranks": ranks, "earlyEnd": true}, "")
	r.BroadcastState()
	r.Unlock()
	logger.Info("房间 %s 投票解散达成，提前进入整场结束", roomID)
}
