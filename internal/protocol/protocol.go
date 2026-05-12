// Package protocol 定义客户端与服务端通信协议常量
package protocol

import "encoding/json"

// 消息类型常量
const (
	MsgLogin             = "LOGIN"
	MsgLoginOK           = "LOGIN_OK"
	MsgCreateRoom        = "CREATE_ROOM"
	MsgCreateRoomOK      = "CREATE_ROOM_OK"
	MsgJoinRoom          = "JOIN_ROOM"
	MsgJoinRoomOK        = "JOIN_ROOM_OK"
	MsgLeaveRoom         = "LEAVE_ROOM"
	MsgLeaveRoomOK       = "LEAVE_ROOM_OK"
	MsgRoomState         = "ROOM_STATE"
	MsgReady             = "READY"
	MsgUnready           = "UNREADY"
	MsgDealCards         = "DEAL_CARDS"
	MsgSubmitLanes       = "SUBMIT_LANES"
	MsgSubmitLanesOK     = "SUBMIT_LANES_OK"
	MsgSettleResult      = "SETTLE_RESULT"
	MsgRoundConfirm      = "ROUND_CONFIRM"
	MsgMatchEnd          = "MATCH_END"
	MsgReconnectSnapshot = "RECONNECT_SNAPSHOT"
	MsgError             = "ERROR"

	// 电脑玩家相关协议
	MsgRoomAddBot    = "ROOM_ADD_BOT"
	MsgRoomAddBotOK  = "ROOM_ADD_BOT_OK"
	MsgRoomKickBot   = "ROOM_KICK_BOT"
	MsgRoomKickBotOK = "ROOM_KICK_BOT_OK"

	// 投票解散对局相关协议
	MsgVoteDissolve        = "VOTE_DISSOLVE"         // 客户端 → 服务端：发起/同意解散
	MsgVoteDissolveCancel  = "VOTE_DISSOLVE_CANCEL"  // 客户端 → 服务端：撤销同意
	MsgVoteDissolveTimeout = "VOTE_DISSOLVE_TIMEOUT" // 服务端 → 客户端：投票 60 秒超时通知
)

// 错误码常量
const (
	ErrRoomNotFound   = 1001
	ErrRoomFull       = 1002
	ErrRoomPlaying    = 1003
	ErrNotInRoom      = 1004
	ErrInvalidLanes   = 1005
	ErrNotLoggedIn    = 1006
	ErrAlreadyInRoom  = 1007
	ErrBadRequest     = 1008
	ErrNotHost        = 1009 // 仅房主可操作
	ErrRoomNotWaiting = 1010 // 房间不处于准备阶段
	ErrInternal       = 500
)

// Envelope 是 WebSocket JSON 消息的统一信封
// 与 Node.js 端 { type, data, reqId, code, msg } 字段保持一致
// ReqID 用 RawMessage 兼容客户端发送的数字与字符串两种形式
type Envelope struct {
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data,omitempty"`
	ReqID json.RawMessage `json:"reqId,omitempty"`
	Code  int             `json:"code,omitempty"`
	Msg   string          `json:"msg,omitempty"`
}
