// Package server HTTP 路由（Gin）
// http.go: /api/health, /api/login, /api/lobby/active-room, /api/room/:id 与 CORS
package server

import (
	"net/http"
	"time"

	"card_ssd/internal/handler"
	"card_ssd/internal/room"
	"card_ssd/internal/session"

	"github.com/gin-gonic/gin"
)

// NewEngine 构建 Gin 引擎，注册 HTTP API 与 /ws WebSocket
func NewEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	// 启动游戏 handler 钩子
	handler.InitGameHandler()
	handler.InitVoteHandler()

	// WebSocket 路由
	r.GET("/ws", handleWS)

	api := r.Group("/api")
	{
		api.GET("/health", apiHealth)
		api.POST("/login", apiLogin)
		api.GET("/lobby/active-room", apiActiveRoom)
		api.GET("/room/:id", apiGetRoom)
	}
	return r
}

// corsMiddleware 跨域中间件：允许任意来源
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Token")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// apiHealth 健康检查
func apiHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ok":    true,
		"time":  time.Now().UnixMilli(),
		"rooms": room.CountRooms(),
		"conns": session.CountConns(),
	})
}

// apiLogin HTTP 登录：生成 token 与身份
type loginBody struct {
	Openid    string `json:"openid"`
	Nickname  string `json:"nickname"`
	AvatarUrl string `json:"avatarUrl"`
}

func apiLogin(c *gin.Context) {
	var body loginBody
	if err := c.ShouldBindJSON(&body); err != nil || body.Openid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "缺少 openid"})
		return
	}
	if body.Nickname == "" {
		body.Nickname = "玩家" + tail4(body.Openid)
	}
	token := session.GenToken()
	session.SaveToken(token, session.TokenInfo{
		Openid:    body.Openid,
		Nickname:  body.Nickname,
		AvatarUrl: body.AvatarUrl,
	})
	// 仅查询，不修改任何 Session/Player 在线状态
	active := room.FindActiveRoomByOpenid(body.Openid)
	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"openid":     body.Openid,
		"nickname":   body.Nickname,
		"activeRoom": active,
	})
}

// apiActiveRoom 仅查询某 openid 当前所在的未结束房间摘要
// GET /api/lobby/active-room?openid=xxx
func apiActiveRoom(c *gin.Context) {
	openid := c.Query("openid")
	if openid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "缺少 openid"})
		return
	}
	active := room.FindActiveRoomByOpenid(openid)
	c.JSON(http.StatusOK, gin.H{
		"activeRoom": active,
	})
}

// apiGetRoom 查询房间概要
func apiGetRoom(c *gin.Context) {
	id := c.Param("id")
	r := room.GetRoom(id)
	if r == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "房间不存在"})
		return
	}
	r.Lock()
	state := r.ToState()
	r.Unlock()
	c.JSON(http.StatusOK, state)
}

func tail4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}
