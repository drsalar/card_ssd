// Package handler WebSocket 大厅相关 handler
// lobby.go: 处理登录、创建/加入/离开房间、准备、管理电脑玩家等大厅阶段消息
package handler

import (
	"encoding/json"

	"card_ssd/internal/game"
	"card_ssd/internal/logger"
	"card_ssd/internal/protocol"
	"card_ssd/internal/room"
	"card_ssd/internal/session"
)

// LoginReq 登录消息载荷
type LoginReq struct {
	Openid    string `json:"openid"`
	Nickname  string `json:"nickname"`
	AvatarUrl string `json:"avatarUrl"`
}

// CreateRoomReq 创建房间载荷
type CreateRoomReq struct {
	WithMa      bool `json:"withMa"`
	TotalRounds int  `json:"totalRounds"`
	MaxPlayers  int  `json:"maxPlayers"`
}

// JoinRoomReq 加入房间载荷
type JoinRoomReq struct {
	RoomID string `json:"roomId"`
}

// HandleLogin 处理登录
func HandleLogin(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	var req LoginReq
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	if req.Openid == "" {
		s.SendError(protocol.ErrBadRequest, "缺少 openid", reqID)
		return
	}
	if req.Nickname == "" {
		req.Nickname = "玩家" + tail4(req.Openid)
	}
	s.Nickname = req.Nickname
	s.AvatarUrl = req.AvatarUrl
	s.LoggedIn = true
	session.BindOpenid(s, req.Openid)
	s.Send(protocol.MsgLoginOK, map[string]string{
		"openid":   req.Openid,
		"nickname": s.Nickname,
	}, reqID)
}

// HandleCreateRoom 处理创建房间
func HandleCreateRoom(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	if s.RoomID != "" {
		s.SendError(protocol.ErrAlreadyInRoom, "已在房间中", reqID)
		return
	}
	var req CreateRoomReq
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	if req.TotalRounds <= 0 {
		req.TotalRounds = 5
	}
	if req.MaxPlayers <= 0 {
		req.MaxPlayers = 4
	}
	rule := room.Rule{
		WithMa:      req.WithMa,
		TotalRounds: req.TotalRounds,
		MaxPlayers:  req.MaxPlayers,
	}
	r := room.CreateRoom(rule, s.Openid)
	res := room.JoinRoom(r.ID, s)
	if res.Err != "" {
		s.SendError(protocol.ErrBadRequest, "创建房间失败", reqID)
		return
	}
	s.Send(protocol.MsgCreateRoomOK, map[string]string{"roomId": r.ID}, reqID)
	r.Lock()
	r.BroadcastState()
	r.Unlock()
}

// HandleJoinRoom 处理加入房间
func HandleJoinRoom(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	if s.RoomID != "" {
		s.SendError(protocol.ErrAlreadyInRoom, "已在房间中", reqID)
		return
	}
	var req JoinRoomReq
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	if req.RoomID == "" {
		s.SendError(protocol.ErrBadRequest, "缺少 roomId", reqID)
		return
	}
	res := room.JoinRoom(req.RoomID, s)
	if res.Err != "" {
		switch res.Err {
		case room.JoinErrNotFound:
			s.SendError(protocol.ErrRoomNotFound, "房间不存在", reqID)
		case room.JoinErrFull:
			s.SendError(protocol.ErrRoomFull, "房间已满", reqID)
		case room.JoinErrPlaying:
			s.SendError(protocol.ErrRoomPlaying, "对局已开始", reqID)
		default:
			s.SendError(protocol.ErrBadRequest, "加入失败", reqID)
		}
		return
	}
	s.Send(protocol.MsgJoinRoomOK, map[string]any{
		"roomId":    req.RoomID,
		"reconnect": res.Reconnect,
	}, reqID)
	res.Room.Lock()
	res.Room.BroadcastState()
	needSettle := res.Room.Phase == room.PhasePlaying && res.Room.AllSubmitted()
	res.Room.Unlock()
	if needSettle {
		DoSettle(res.Room)
	}
	// 重连补发手牌、三道与阶段快照
	if res.Reconnect && res.Player != nil {
		sendReconnectMessages(s, res.Room, res.Player)
	}
}

// reconnectSnapshot 断线重连快照载荷
type reconnectSnapshot struct {
	Phase        room.Phase         `json:"phase"`
	Hand         []game.Card        `json:"hand"`
	Lanes        *game.Lanes        `json:"lanes"`
	Submitted    bool               `json:"submitted"`
	LastSettle   *game.SettleResult `json:"lastSettle"`
	CurrentRound int                `json:"currentRound"`
	TotalRounds  int                `json:"totalRounds"`
}

// sendReconnectMessages 单播断线重连消息，恢复前端子阶段
func sendReconnectMessages(s *session.Session, r *room.Room, p *room.Player) {
	if s == nil || r == nil || p == nil {
		return
	}
	r.Lock()
	hand := append([]game.Card{}, p.Hand...)
	lanes := cloneLanes(p.Lanes)
	submitted := p.Submitted
	payload := reconnectSnapshot{
		Phase:        r.Phase,
		Hand:         hand,
		Lanes:        lanes,
		Submitted:    submitted,
		LastSettle:   r.LastSettle,
		CurrentRound: r.CurrentRound,
		TotalRounds:  r.Rule.TotalRounds,
	}
	r.Unlock()

	if len(hand) > 0 {
		s.Send(protocol.MsgDealCards, map[string]any{"hand": hand}, nil)
	}
	if lanes != nil && submitted {
		s.Send(protocol.MsgSubmitLanesOK, map[string]any{"lanes": lanes}, nil)
	}
	s.Send(protocol.MsgReconnectSnapshot, payload, nil)
}

// cloneLanes 复制三道切片，避免快照发送时引用房间内可变切片
func cloneLanes(src *game.Lanes) *game.Lanes {
	if src == nil {
		return nil
	}
	return &game.Lanes{
		Head:   append([]game.Card{}, src.Head...),
		Middle: append([]game.Card{}, src.Middle...),
		Tail:   append([]game.Card{}, src.Tail...),
	}
}

// HandleLeaveRoom 处理离开房间
func HandleLeaveRoom(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	if s.RoomID == "" {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	room.LeaveRoom(s)
	s.Send(protocol.MsgLeaveRoomOK, struct{}{}, reqID)
}

// HandleReady 处理准备
func HandleReady(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	r.Lock()
	if r.Phase != room.PhaseWaiting {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "当前阶段不允许准备", reqID)
		return
	}
	if p := r.GetPlayer(s.Openid); p != nil {
		p.Ready = true
	}
	r.BroadcastState()
	allReady := r.AllReady()
	r.Unlock()
	if allReady {
		StartRound(r)
	}
}

// HandleUnready 处理取消准备
func HandleUnready(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	r.Lock()
	defer r.Unlock()
	if r.Phase != room.PhaseWaiting {
		s.SendError(protocol.ErrBadRequest, "当前阶段不允许取消准备", reqID)
		return
	}
	if p := r.GetPlayer(s.Openid); p != nil {
		p.Ready = false
	}
	r.BroadcastState()
}

// AddBotReq 添加电脑玩家载荷（暂无字段，预留）
type AddBotReq struct{}

// KickBotReq 踢出电脑玩家载荷
type KickBotReq struct {
	Openid string `json:"openid"`
}

// HandleAddBot 处理「添加电脑玩家」
// 仅房主、仅 PhaseWaiting、人数未满才允许
func HandleAddBot(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	r.Lock()
	if r.HostID != s.Openid {
		r.Unlock()
		s.SendError(protocol.ErrNotHost, "仅房主可操作", reqID)
		return
	}
	if r.Phase != room.PhaseWaiting {
		r.Unlock()
		s.SendError(protocol.ErrRoomNotWaiting, "对局已开始，无法添加电脑玩家", reqID)
		return
	}
	if r.IsFull() {
		r.Unlock()
		s.SendError(protocol.ErrRoomFull, "房间人数已满", reqID)
		return
	}
	bot := r.AddBot()
	logger.Info("房间 %s 房主 %s 添加电脑玩家 %s(%s)", r.ID, s.Openid, bot.Nickname, bot.Openid)
	r.BroadcastState()
	r.Unlock()
	s.Send(protocol.MsgRoomAddBotOK, map[string]any{
		"openid":   bot.Openid,
		"nickname": bot.Nickname,
	}, reqID)
	// 1 秒后自动准备
	room.ScheduleBotReady(r, bot.Openid)
}

// HandleKickBot 处理「踢出电脑玩家」
// 仅房主、仅 PhaseWaiting、且目标必须是 bot
func HandleKickBot(s *session.Session, raw json.RawMessage, reqID json.RawMessage) {
	r := room.GetRoom(s.RoomID)
	if r == nil {
		s.SendError(protocol.ErrNotInRoom, "未在房间", reqID)
		return
	}
	var req KickBotReq
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	if req.Openid == "" {
		s.SendError(protocol.ErrBadRequest, "缺少 openid", reqID)
		return
	}
	r.Lock()
	if r.HostID != s.Openid {
		r.Unlock()
		s.SendError(protocol.ErrNotHost, "仅房主可操作", reqID)
		return
	}
	if r.Phase != room.PhaseWaiting {
		r.Unlock()
		s.SendError(protocol.ErrRoomNotWaiting, "对局进行中，无法踢出", reqID)
		return
	}
	target := r.GetPlayer(req.Openid)
	if target == nil || !target.IsBot {
		r.Unlock()
		s.SendError(protocol.ErrBadRequest, "目标不是电脑玩家", reqID)
		return
	}
	r.RemoveBot(req.Openid)
	logger.Info("房间 %s 房主 %s 踢出电脑玩家 %s", r.ID, s.Openid, req.Openid)
	r.BroadcastState()
	r.Unlock()
	s.Send(protocol.MsgRoomKickBotOK, map[string]any{"openid": req.Openid}, reqID)
}

// tail4 取字符串末尾 4 位（用于默认昵称）
func tail4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
