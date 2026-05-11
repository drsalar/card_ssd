// Package session 维护 WebSocket 会话与连接管理
// session.go: 单个会话的发送/接收/关闭
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"card_ssd/internal/logger"
	"card_ssd/internal/protocol"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 64 * 1024
)

// MessageHandler 消息处理回调
type MessageHandler func(s *Session, env *protocol.Envelope)

// CloseHandler 连接关闭回调
type CloseHandler func(s *Session)

var nextConnID int64

// Session 单个 WebSocket 会话
type Session struct {
	ConnID    int64
	Openid    string
	Nickname  string
	AvatarUrl string
	Token     string
	RoomID    string
	LoggedIn  bool

	ws     *websocket.Conn
	sendCh chan []byte
	closed atomic.Bool
	mu     sync.Mutex
}

// NewSession 创建新的 Session
func NewSession(ws *websocket.Conn) *Session {
	return &Session{
		ConnID: atomic.AddInt64(&nextConnID, 1),
		ws:     ws,
		sendCh: make(chan []byte, 64),
	}
}

// Send 发送消息（自动包裹 JSON）
// reqID 为客户端原始 reqId（可为数字或字符串），传 nil 则不带 reqId 字段
func (s *Session) Send(msgType string, data any, reqID json.RawMessage) {
	if s.closed.Load() {
		return
	}
	payload := map[string]any{"type": msgType}
	if data != nil {
		payload["data"] = data
	} else {
		payload["data"] = struct{}{}
	}
	if len(reqID) > 0 {
		payload["reqId"] = reqID
	}
	bs, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("会话 %d 序列化失败: %v", s.ConnID, err)
		return
	}
	select {
	case s.sendCh <- bs:
	default:
		// 队列满，主动断开
		logger.Warn("会话 %d 发送队列满，关闭连接", s.ConnID)
		s.Close()
	}
}

// SendError 发送错误消息
func (s *Session) SendError(code int, msg string, reqID json.RawMessage) {
	if s.closed.Load() {
		return
	}
	payload := map[string]any{
		"type": protocol.MsgError,
		"data": struct{}{},
		"code": code,
		"msg":  msg,
	}
	if len(reqID) > 0 {
		payload["reqId"] = reqID
	}
	bs, err := json.Marshal(payload)
	if err != nil {
		return
	}
	select {
	case s.sendCh <- bs:
	default:
		s.Close()
	}
}

// Close 关闭会话
func (s *Session) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.sendCh)
	_ = s.ws.Close()
}

// IsClosed 是否已关闭
func (s *Session) IsClosed() bool {
	return s.closed.Load()
}

// Run 启动读写协程，阻塞直到连接关闭
func (s *Session) Run(onMessage MessageHandler, onClose CloseHandler) {
	go s.writePump()
	s.readPump(onMessage)
	if onClose != nil {
		onClose(s)
	}
}

// readPump 读循环
func (s *Session) readPump(onMessage MessageHandler) {
	defer s.Close()
	s.ws.SetReadLimit(maxMessageSize)
	_ = s.ws.SetReadDeadline(time.Now().Add(pongWait))
	s.ws.SetPongHandler(func(string) error {
		_ = s.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, msg, err := s.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Warn("会话 %d 读异常: %v", s.ConnID, err)
			}
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(msg, &env); err != nil {
			s.SendError(protocol.ErrBadRequest, "无效的 JSON", nil)
			continue
		}
		// 业务处理放入新协程，避免单条消息阻塞读循环；handler 内部需自保护
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("会话 %d 处理消息 panic: %v", s.ConnID, r)
					s.SendError(protocol.ErrInternal, "服务器内部错误", env.ReqID)
				}
			}()
			if onMessage != nil {
				onMessage(s, &env)
			}
		}()
	}
}

// writePump 写循环，串行化发送，定时 ping 保活
func (s *Session) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-s.sendCh:
			if !ok {
				return
			}
			s.mu.Lock()
			_ = s.ws.SetWriteDeadline(time.Now().Add(writeWait))
			err := s.ws.WriteMessage(websocket.TextMessage, msg)
			s.mu.Unlock()
			if err != nil {
				logger.Warn("会话 %d 写失败: %v", s.ConnID, err)
				return
			}
		case <-ticker.C:
			s.mu.Lock()
			_ = s.ws.SetWriteDeadline(time.Now().Add(writeWait))
			err := s.ws.WriteMessage(websocket.PingMessage, nil)
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// GenToken 生成随机 token（HTTP 登录后返回，WS 升级时携带）
func GenToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
