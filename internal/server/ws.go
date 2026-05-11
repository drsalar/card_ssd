// Package server WebSocket 升级与消息分发
// ws.go: 升级处理 + 路由分发
package server

import (
	"net/http"

	"card_ssd/internal/handler"
	"card_ssd/internal/logger"
	"card_ssd/internal/protocol"
	"card_ssd/internal/room"
	"card_ssd/internal/session"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// 允许任意来源（小游戏环境无 Origin 校验）
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleWS Gin 路由 GET /ws 的处理函数
func handleWS(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Warn("WebSocket 升级失败: %v", err)
		return
	}
	s := session.NewSession(ws)
	session.Add(s)
	logger.Info("新连接 connID=%d remote=%s", s.ConnID, c.ClientIP())

	// 可选：通过 ?token=xxx 预鉴权
	if token := c.Query("token"); token != "" {
		if info, ok := session.LookupByToken(token); ok {
			s.Token = token
			s.Nickname = info.Nickname
			s.AvatarUrl = info.AvatarUrl
			s.LoggedIn = true
			session.BindOpenid(s, info.Openid)
		}
	}

	// 启动收发协程
	s.Run(dispatch, onClose)
}

// dispatch 消息分发
func dispatch(s *session.Session, env *protocol.Envelope) {
	// 未登录前仅允许 LOGIN
	if !s.LoggedIn && env.Type != protocol.MsgLogin {
		s.SendError(protocol.ErrNotLoggedIn, "未登录", env.ReqID)
		return
	}
	switch env.Type {
	case protocol.MsgLogin:
		handler.HandleLogin(s, env.Data, env.ReqID)
	case protocol.MsgCreateRoom:
		handler.HandleCreateRoom(s, env.Data, env.ReqID)
	case protocol.MsgJoinRoom:
		handler.HandleJoinRoom(s, env.Data, env.ReqID)
	case protocol.MsgLeaveRoom:
		handler.HandleLeaveRoom(s, env.Data, env.ReqID)
	case protocol.MsgReady:
		handler.HandleReady(s, env.Data, env.ReqID)
	case protocol.MsgUnready:
		handler.HandleUnready(s, env.Data, env.ReqID)
	case protocol.MsgSubmitLanes:
		handler.HandleSubmitLanes(s, env.Data, env.ReqID)
	case protocol.MsgRoundConfirm:
		handler.HandleRoundConfirm(s, env.Data, env.ReqID)
	case protocol.MsgRoomAddBot:
		handler.HandleAddBot(s, env.Data, env.ReqID)
	case protocol.MsgRoomKickBot:
		handler.HandleKickBot(s, env.Data, env.ReqID)
	default:
		s.SendError(protocol.ErrBadRequest, "未知消息类型: "+env.Type, env.ReqID)
	}
}

// onClose 连接关闭回调
func onClose(s *session.Session) {
	logger.Info("连接关闭 connID=%d openid=%s", s.ConnID, s.Openid)
	room.HandleDisconnect(s)
	session.Remove(s)
}

// Shutdown 优雅关闭：关闭所有 WS 会话
func Shutdown() {
	session.CloseAll()
}
