// Package room \u7535\u8111\u73a9\u5bb6\u81ea\u52a8\u52a8\u4f5c\u8c03\u5ea6
// bot_driver.go: \u4e3a\u7535\u8111\u73a9\u5bb6\u63d0\u4f9b\u81ea\u52a8\u51c6\u5907 / \u81ea\u52a8\u5f00\u724c / \u81ea\u52a8\u786e\u8ba4 \u4e09\u4e2a\u8c03\u5ea6\u5165\u53e3\u3002
//
// \u8bbe\u8ba1\u8981\u70b9\uff1a
//  1. \u5168\u90e8\u52a8\u4f5c\u53ea\u5728\u5b9a\u65f6\u5668\u56de\u8c03\u4e2d\u8d77\u4e1a\u52a1\u6548\u679c\uff0c\u5e76\u5728\u8fdb\u5165\u4e1a\u52a1\u903b\u8f91\u524d\u4f1a\u68c0\u67e5\u623f\u95f4 destroyed \u6807\u8bb0
//  2. \u6240\u6709\u5b9a\u65f6\u5668\u90fd\u4f1a\u767b\u8bb0\u5230 room.botTimers\uff0c\u623f\u95f4\u9500\u6bc1\u65f6\u53ef\u88ab CancelBotTimers \u7edf\u4e00\u53d6\u6d88
//  3. \u4e0a\u5c42\u4e1a\u52a1\u903b\u8f91\uff08\u5168\u5458\u51c6\u5907 \u2192 \u53d1\u724c\u3001\u5168\u5458\u5f00\u724c \u2192 \u7ed3\u7b97 \u7b49\uff09\u4ee5\u201d\u94a9\u5b50\u201c\u65b9\u5f0f\u6ce8\u5165\uff0c\u907f\u514d room \u5305\u53cd\u5411\u4f9d\u8d56 handler
package room

import (
	"time"

	"card_ssd/internal/game"
	"card_ssd/internal/logger"
)

// botActionDelay \u7535\u8111\u73a9\u5bb6\u81ea\u52a8\u52a8\u4f5c\u7684\u9ed8\u8ba4\u5ef6\u8fdf\uff081 \u79d2\uff09
// \u9002\u5f53\u5ef6\u8fdf\u907f\u514d\u201d\u624b\u8d77\u624b\u843d\u201c\u8fc7\u4e8e\u5495\u54dd\uff0c\u4e5f\u7ed9\u524d\u7aef\u72b6\u6001\u540c\u6b65\u7559\u51fa\u91cf
const botActionDelay = 1 * time.Second

// BotStartRoundHook \u5168\u5458\u51c6\u5907\u540e\u89e6\u53d1\u53d1\u724c\u7684\u94a9\u5b50\uff08\u5e94\u7531 handler \u6ce8\u5165\u4e3a StartRound\uff09
// \u8c03\u7528\u65f6\u4f20\u5165\u201d\u672a\u52a0\u9501\u201c\u7684\u623f\u95f4\u3002\u94a9\u5b50\u5185\u90e8\u9700\u81ea\u884c\u5904\u7406\u9501\u3002
type BotStartRoundHook func(r *Room)

// BotSettleHook \u5168\u5458\u5f00\u724c\u540e\u89e6\u53d1\u7ed3\u7b97\u7684\u94a9\u5b50\uff08\u5e94\u7531 handler \u6ce8\u5165\u4e3a DoSettle\uff09
type BotSettleHook func(r *Room)

// BotAdvanceAfterConfirmHook \u5168\u5458\u786e\u8ba4\u540e\u89e6\u53d1\u8fdb\u5165\u4e0b\u4e00\u5c40\u6216\u6574\u573a\u7ed3\u675f\u7684\u94a9\u5b50
type BotAdvanceAfterConfirmHook func(r *Room)

var (
	botStartRoundHook BotStartRoundHook
	botSettleHook     BotSettleHook
	botAdvanceHook    BotAdvanceAfterConfirmHook
)

// SetBotStartRoundHook \u6ce8\u5165\u53d1\u724c\u94a9\u5b50
func SetBotStartRoundHook(h BotStartRoundHook) { botStartRoundHook = h }

// SetBotSettleHook \u6ce8\u5165\u7ed3\u7b97\u94a9\u5b50
func SetBotSettleHook(h BotSettleHook) { botSettleHook = h }

// SetBotAdvanceAfterConfirmHook \u6ce8\u5165\u8fdb\u5165\u4e0b\u4e00\u5c40\u94a9\u5b50
func SetBotAdvanceAfterConfirmHook(h BotAdvanceAfterConfirmHook) { botAdvanceHook = h }

// ScheduleBotReady \u5b89\u6392\u4e00\u4e2a bot \u81ea\u52a8\u51c6\u5907
// \u8c03\u7528\u65f6\u53ef\u80fd\u5904\u4e8e\u5df2\u52a0\u9501\u72b6\u6001\u4e2d\uff08\u4f8b\u5982 lobby AddBot \u540e\uff09\u4e5f\u53ef\u80fd\u672a\u52a0\u9501
// \u672c\u51fd\u6570\u4e0d\u4f1a\u52a0\u9501 r.mu\uff0c\u5b9a\u65f6\u5668\u56de\u8c03\u91cc\u624d\u4f1a\u91cd\u65b0\u52a0\u9501\u3002
func ScheduleBotReady(r *Room, botOpenid string) {
	if r == nil {
		return
	}
	roomID := r.ID
	t := time.AfterFunc(botActionDelay, func() {
		botDoReady(roomID, botOpenid)
	})
	r.AddBotTimer(t)
}

// botDoReady \u5b9a\u65f6\u5668\u56de\u8c03\uff1a\u8bbe\u7f6e bot ready=true\uff0c\u5e76\u5728\u5168\u5458\u5c31\u7eea\u65f6\u89e6\u53d1\u53d1\u724c
func botDoReady(roomID, botOpenid string) {
	r := GetRoom(roomID)
	if r == nil {
		return
	}
	r.Lock()
	if r.IsDestroyed() {
		r.Unlock()
		return
	}
	if r.Phase != PhaseWaiting {
		r.Unlock()
		return
	}
	p := r.GetPlayer(botOpenid)
	if p == nil || !p.IsBot {
		r.Unlock()
		return
	}
	if p.Ready {
		r.Unlock()
		return
	}
	p.Ready = true
	logger.Info("\u623f\u95f4 %s \u7535\u8111\u73a9\u5bb6 %s(%s) \u81ea\u52a8\u51c6\u5907", r.ID, p.Nickname, p.Openid)
	r.BroadcastState()
	allReady := r.AllReady()
	r.Unlock()
	if allReady && botStartRoundHook != nil {
		botStartRoundHook(r)
	}
}

// ScheduleBotLock \u5b89\u6392\u4e00\u4e2a bot \u5728\u53d1\u724c\u540e\u81ea\u52a8\u7406\u724c\u5e76\u63d0\u4ea4\u5f00\u724c
// \u9700\u8981\u5728 handler \u53d1\u724c\u5b8c\u6210\uff08\u8fdb\u5165 PhasePlaying\uff09\u540e\u8c03\u7528
func ScheduleBotLock(r *Room, botOpenid string) {
	if r == nil {
		return
	}
	roomID := r.ID
	t := time.AfterFunc(botActionDelay, func() {
		botDoLock(roomID, botOpenid)
	})
	r.AddBotTimer(t)
}

// botDoLock \u5b9a\u65f6\u5668\u56de\u8c03\uff1a\u8c03\u7528 game.AutoArrange \u7406\u724c\uff0c\u8bbe\u7f6e Lanes \u4e0e Submitted\uff0c\u5e76\u5728\u5168\u5458\u63d0\u4ea4\u65f6\u89e6\u53d1\u7ed3\u7b97
func botDoLock(roomID, botOpenid string) {
	r := GetRoom(roomID)
	if r == nil {
		return
	}
	r.Lock()
	if r.IsDestroyed() || r.Phase != PhasePlaying {
		r.Unlock()
		return
	}
	p := r.GetPlayer(botOpenid)
	if p == nil || !p.IsBot {
		r.Unlock()
		return
	}
	if p.Submitted {
		r.Unlock()
		return
	}
	if len(p.Hand) != 13 {
		r.Unlock()
		return
	}
	hand := append([]game.Card{}, p.Hand...)
	r.Unlock()

	// AI \u7406\u724c\u8fd0\u7b97\u53ef\u80fd\u6709\u4e00\u70b9\u8017\u65f6\uff08~7 \u4e07\u6b21\u8bc4\u4f30\uff09\uff0c\u5728\u9501\u5916\u6267\u884c\u4ee5\u907f\u514d\u963b\u585e\u5176\u4ed6\u73a9\u5bb6
	lanes, fb := game.AutoArrange(hand)
	if fb {
		logger.Warn("\u623f\u95f4 %s \u7535\u8111\u73a9\u5bb6 %s \u4f7f\u7528\u5151\u5e95\u7406\u724c\u7b56\u7565", roomID, botOpenid)
	}

	r.Lock()
	if r.IsDestroyed() || r.Phase != PhasePlaying {
		r.Unlock()
		return
	}
	p = r.GetPlayer(botOpenid)
	if p == nil || !p.IsBot || p.Submitted {
		r.Unlock()
		return
	}
	// \u518d\u6821\u9a8c\u4e00\u4e0b\uff08\u5176\u95f4\u53ef\u80fd\u6709\u72b6\u6001\u53d8\u5316\uff09
	if !game.SameCardSet(p.Hand, append(append(append([]game.Card{}, lanes.Head...), lanes.Middle...), lanes.Tail...)) {
		r.Unlock()
		logger.Warn("\u623f\u95f4 %s \u7535\u8111\u73a9\u5bb6 %s \u624b\u724c\u4e0e\u7406\u724c\u4e0d\u4e00\u81f4\uff0c\u8df3\u8fc7", roomID, botOpenid)
		return
	}
	v := game.ValidateLanes(lanes.Head, lanes.Middle, lanes.Tail)
	if !v.OK {
		// \u4e0a\u5c42\u9080\u8bf7\u8fc7\u7684\u5151\u5e95\u51fd\u6570\u8fd8\u4e0d\u80fd\u4fdd\u8bc1\uff0c\u518d\u624b\u52a8\u8d70\u5934 3/\u4e2d 5/\u5c3e 5\uff08\u4e4c\u9f99\u4e00\u5b9a\u5408\u6cd5\u5982\u679c\u987a\u5e8f\u4e0d\u9519\uff0c\u8fd9\u91cc\u53cc\u4fdd\u9669\uff09
		lanes = &game.Lanes{Head: hand[0:3], Middle: hand[3:8], Tail: hand[8:13]}
	}
	p.Lanes = lanes
	p.Submitted = true
	logger.Info("\u623f\u95f4 %s \u7535\u8111\u73a9\u5bb6 %s \u81ea\u52a8\u5f00\u724c", r.ID, botOpenid)
	r.BroadcastState()
	allSubmitted := r.AllSubmitted()
	r.Unlock()
	if allSubmitted && botSettleHook != nil {
		botSettleHook(r)
	}
}

// ScheduleBotConfirm \u5b89\u6392 bot \u81ea\u52a8\u786e\u8ba4\u672c\u5c40\u7ed3\u7b97 / \u603b\u573a\u7ed3\u675f
// \u9002\u7528\u4e8e Phase=PhaseComparing \u6216 PhaseMatchEnd \u7684\u573a\u666f
func ScheduleBotConfirm(r *Room, botOpenid string) {
	if r == nil {
		return
	}
	roomID := r.ID
	t := time.AfterFunc(botActionDelay, func() {
		botDoConfirm(roomID, botOpenid)
	})
	r.AddBotTimer(t)
}

// botDoConfirm \u5b9a\u65f6\u5668\u56de\u8c03\uff1a\u8bbe\u7f6e RoundConfirmed=true\uff0c\u5168\u5458\u786e\u8ba4\u540e\u89e6\u53d1 Advance \u94a9\u5b50
func botDoConfirm(roomID, botOpenid string) {
	r := GetRoom(roomID)
	if r == nil {
		return
	}
	r.Lock()
	if r.IsDestroyed() {
		r.Unlock()
		return
	}
	if r.Phase != PhaseComparing && r.Phase != PhaseMatchEnd {
		r.Unlock()
		return
	}
	p := r.GetPlayer(botOpenid)
	if p == nil || !p.IsBot {
		r.Unlock()
		return
	}
	if p.RoundConfirmed {
		r.Unlock()
		return
	}
	p.RoundConfirmed = true
	logger.Info("\u623f\u95f4 %s \u7535\u8111\u73a9\u5bb6 %s \u81ea\u52a8\u786e\u8ba4\u7ed3\u7b97", r.ID, botOpenid)
	r.BroadcastState()
	allConfirmed := r.AllRoundConfirmed()
	phase := r.Phase
	r.Unlock()
	if allConfirmed && phase == PhaseComparing && botAdvanceHook != nil {
		botAdvanceHook(r)
	}
}

// ScheduleAllBotsReady \u5c06\u623f\u95f4\u5185\u6240\u6709\u7535\u8111\u73a9\u5bb6\u91cd\u65b0\u8c03\u5ea6\u4e3a\u81ea\u52a8\u51c6\u5907
// \u9002\u7528\u4e8e\u4e00\u5c40\u7ed3\u7b97\u540e\u8fdb\u5165\u4e0b\u4e00\u5c40\u51c6\u5907\u9636\u6bb5\u3002\u8c03\u7528\u524d\u5e94\u4f7f\u4f17 bot \u7684 ready=false\u3002
func ScheduleAllBotsReady(r *Room) {
	if r == nil {
		return
	}
	for _, p := range r.BotPlayers() {
		ScheduleBotReady(r, p.Openid)
	}
}

// ScheduleAllBotsLock \u4e3a\u623f\u95f4\u5185\u6240\u6709\u7535\u8111\u73a9\u5bb6\u8c03\u5ea6\u81ea\u52a8\u5f00\u724c
func ScheduleAllBotsLock(r *Room) {
	if r == nil {
		return
	}
	for _, p := range r.BotPlayers() {
		ScheduleBotLock(r, p.Openid)
	}
}

// ScheduleAllBotsConfirm \u4e3a\u623f\u95f4\u5185\u6240\u6709\u7535\u8111\u73a9\u5bb6\u8c03\u5ea6\u81ea\u52a8\u786e\u8ba4\u7ed3\u7b97
func ScheduleAllBotsConfirm(r *Room) {
	if r == nil {
		return
	}
	for _, p := range r.BotPlayers() {
		ScheduleBotConfirm(r, p.Openid)
	}
}
