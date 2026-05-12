// Package handler WebSocket 对局相关 handler
// game.go: 处理发牌、提交三道、本局结算确认等对局阶段消息
package handler

import (
	"encoding/json"

	"card_ssd/internal/game"
	"card_ssd/internal/logger"
	"card_ssd/internal/protocol"
	"card_ssd/internal/room"
	"card_ssd/internal/session"
)

// SubmitLanesReq 提交三道载荷
type SubmitLanesReq struct {
	Head   []game.Card `json:"head"`
	Middle []game.Card `json:"middle"`
	Tail   []game.Card `json:"tail"`
}

// InitGameHandler 注册自动结算钩子（由 server 启动时调用）
func InitGameHandler() {
	room.SetAutoSettleHook(DoSettle)
	// 电脑玩家调度钩子
	room.SetBotStartRoundHook(StartRound)
	room.SetBotSettleHook(DoSettle)
	room.SetBotAdvanceAfterConfirmHook(advanceAfterAllConfirmed)
}

// StartRound 开始一局：洗牌、发牌、改阶段
// 调用方：在房间 unlock 状态下调用（lobby.HandleReady 已 unlock）
func StartRound(r *room.Room) {
	r.Lock()
	r.Phase = room.PhasePlaying
	r.Touch()
	// 重置一局相关数据（保留 score）
	for _, p := range r.Players {
		p.Hand = nil
		p.Lanes = nil
		p.Submitted = false
		p.RoundConfirmed = false
	}
	hands := game.Deal(len(r.Players))
	// 私发手牌
	for i, p := range r.Players {
		p.Hand = hands[i]
	}
	r.BroadcastState()
	currentRound := r.CurrentRound + 1
	playersSnapshot := make([]struct {
		openid string
		hand   []game.Card
	}, len(r.Players))
	for i, p := range r.Players {
		playersSnapshot[i].openid = p.Openid
		playersSnapshot[i].hand = p.Hand
	}
	roomID := r.ID
	r.Unlock()
	// 私发 DEAL_CARDS
	for _, snap := range playersSnapshot {
		s := session.GetByOpenid(snap.openid)
		if s != nil {
			s.Send(protocol.MsgDealCards, map[string]any{"hand": snap.hand}, nil)
		}
	}
	logger.Info("房间 %s 开始第 %d 局", roomID, currentRound)
	// 为所有电脑玩家安排自动理牌 + 开牌
	room.ScheduleAllBotsLock(r)
}

// HandleSubmitLanes 玩家提交三道
func HandleSubmitLanes(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	var req SubmitLanesReq
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	r.Lock()
	if r.Phase != room.PhasePlaying {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "当前不允许开牌", reqID)
		return
	}
	p := r.GetPlayer(s.Openid)
	if p == nil {
		r.Unlock()
		s.SendError(protocol.ErrNotInRoom, "玩家不存在", reqID)
		return
	}
	if p.Submitted {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "已开牌", reqID)
		return
	}
	// 校验所提交 13 张与发牌一致
	all := make([]game.Card, 0, 13)
	all = append(all, req.Head...)
	all = append(all, req.Middle...)
	all = append(all, req.Tail...)
	if len(all) != 13 || !game.SameCardSet(p.Hand, all) {
		r.Unlock()
		s.SendError(protocol.ErrInvalidLanes, "提交的牌不合法", reqID)
		return
	}
	v := game.ValidateLanes(req.Head, req.Middle, req.Tail)
	if !v.OK {
		r.Unlock()
		s.SendError(protocol.ErrInvalidLanes, "三道大小不合法", reqID)
		return
	}
	p.Lanes = &game.Lanes{Head: req.Head, Middle: req.Middle, Tail: req.Tail}
	p.Submitted = true
	r.Touch()
	r.BroadcastState()
	allSubmitted := r.AllSubmitted()
	r.Unlock()
	s.Send(protocol.MsgSubmitLanesOK, map[string]any{"lanes": p.Lanes}, reqID)
	if allSubmitted {
		DoSettle(r)
	}
}

// DoSettle 触发结算
func DoSettle(r *room.Room) {
	r.Lock()
	if r.Phase != room.PhasePlaying {
		r.Unlock()
		return
	}
	r.Phase = room.PhaseComparing
	r.Touch()
	// 构造结算输入
	inputs := make([]game.SettleInput, len(r.Players))
	for i, p := range r.Players {
		inputs[i] = game.SettleInput{
			Openid: p.Openid,
			Hand:   p.Hand,
			Lanes:  p.Lanes,
		}
	}
	result := game.Settle(inputs, r.Rule.WithMa)
	// 累加积分
	for _, rp := range result.Players {
		if pp := r.GetPlayer(rp.Openid); pp != nil {
			pp.Score += rp.FinalScore
		}
	}
	r.LastSettle = &result
	r.CurrentRound++
	// 当前积分快照
	scores := make([]map[string]any, len(r.Players))
	for i, p := range r.Players {
		scores[i] = map[string]any{"openid": p.Openid, "score": p.Score}
	}
	payload := map[string]any{
		"round":       r.CurrentRound,
		"totalRounds": r.Rule.TotalRounds,
		"players":     result.Players,
		"homeruns":    result.Homeruns,
		"pairs":       result.Pairs,
		"scores":      scores,
	}
	roomID := r.ID
	currentRound := r.CurrentRound
	r.Broadcast(protocol.MsgSettleResult, payload, "")
	r.BroadcastState()
	r.Unlock()
	logger.Info("房间 %s 第 %d 局结算完成", roomID, currentRound)
	// 进入比牌确认阶段后，电脑玩家自动确认
	room.ScheduleAllBotsConfirm(r)
}

// HandleRoundConfirm 玩家确认本局结算
func HandleRoundConfirm(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	r.Lock()
	if r.Phase != room.PhaseComparing {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "当前阶段不允许确认", reqID)
		return
	}
	if p := r.GetPlayer(s.Openid); p != nil {
		p.RoundConfirmed = true
	}
	r.BroadcastState()
	allConfirmed := r.AllRoundConfirmed()
	// 兜底：若仍有 bot 未确认，主动重新调度一次（Schedule 内部 idempotent，已 confirmed 的会自行跳过）
	pendingBots := make([]string, 0)
	if !allConfirmed {
		for _, pp := range r.Players {
			if pp.IsBot && !pp.RoundConfirmed {
				pendingBots = append(pendingBots, pp.Openid)
			}
		}
	}
	r.Unlock()
	for _, oid := range pendingBots {
		room.ScheduleBotConfirm(r, oid)
	}
	if allConfirmed {
		advanceAfterAllConfirmed(r)
	}
}

// advanceAfterAllConfirmed 全员确认本局结算后推进：进入下一局或整场结束
// 抽离为独立函数，便于 bot 自动确认时复用
func advanceAfterAllConfirmed(r *room.Room) {
	r.Lock()
	defer r.Unlock()
	if r.Phase != room.PhaseComparing {
		return
	}
	r.Touch()
	if r.CurrentRound >= r.Rule.TotalRounds {
		r.Phase = room.PhaseMatchEnd
		ranks := make([]map[string]any, len(r.Players))
		idx := make([]int, len(r.Players))
		for i := range idx {
			idx[i] = i
		}
		for i := 1; i < len(idx); i++ {
			for j := i; j > 0 && r.Players[idx[j-1]].Score < r.Players[idx[j]].Score; j-- {
				idx[j-1], idx[j] = idx[j], idx[j-1]
			}
		}
		for i, k := range idx {
			p := r.Players[k]
			ranks[i] = map[string]any{
				"openid":   p.Openid,
				"nickname": p.Nickname,
				"score":    p.Score,
			}
		}
		r.Broadcast(protocol.MsgMatchEnd, map[string]any{"ranks": ranks}, "")
		r.BroadcastState()
		return
	}
	r.Phase = room.PhaseWaiting
	r.ResetRound()
	r.BroadcastState()
	// 进入下一局准备阶段：所有 bot 自动重新准备
	room.ScheduleAllBotsReady(r)
}
