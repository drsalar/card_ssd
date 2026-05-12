// Package server HTTP 路由（Gin）
// http.go: /api/health, /api/login, /api/lobby/active-room, /api/room/:id 与 CORS
package server

import (
	"net/http"
	"time"

	"card_ssd/internal/handler"
	"card_ssd/internal/logger"
	"card_ssd/internal/room"
	"card_ssd/internal/session"
	"card_ssd/internal/storage"
	"card_ssd/internal/wxauth"

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

// apiLogin HTTP 登录：方案 B —— 用 wx.login 取的 code 换真实 openid
//  1. body.Code 非空 → 调用 wxauth.Code2Session（WX_APPID/WX_SECRET 未配置或失败时进入步骤 2）
//  2. 请求头 X-WX-OPENID 非空 → 直接用云托管自动注入的真实 openid
//  3. 兼容旧链路：body.Openid 非空 → 直接使用（仅用于本地调试 / 老客户端兼容）
//  4. 三步均失败 → 返回 400
type loginBody struct {
	Code      string `json:"code"`
	Openid    string `json:"openid"` // 兼容老客户端 / 本地调试
	Nickname  string `json:"nickname"`
	AvatarUrl string `json:"avatarUrl"`
}

func apiLogin(c *gin.Context) {
	var body loginBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "无效请求"})
		return
	}
	openid := resolveOpenid(c, &body)
	if openid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "登录失败"})
		return
	}
	// 用户档案：写入或回填
	if body.Nickname != "" || body.AvatarUrl != "" {
		storage.UpsertUser(openid, body.Nickname, body.AvatarUrl)
	}
	if body.Nickname == "" || body.AvatarUrl == "" {
		// 客户端未传 → 从 DB 回填，避免「玩家xxxx」
		if dbNick, dbAvatar, ok := storage.GetUser(openid); ok {
			if body.Nickname == "" {
				body.Nickname = dbNick
			}
			if body.AvatarUrl == "" {
				body.AvatarUrl = dbAvatar
			}
		}
	}
	if body.Nickname == "" {
		body.Nickname = "玩家" + tail4(openid)
	}
	token := session.GenToken()
	session.SaveToken(token, session.TokenInfo{
		Openid:    openid,
		Nickname:  body.Nickname,
		AvatarUrl: body.AvatarUrl,
	})
	// 仅查询，不修改任何 Session/Player 在线状态
	active := room.FindActiveRoomByOpenid(openid)
	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"openid":     openid,
		"nickname":   body.Nickname,
		"avatarUrl":  body.AvatarUrl,
		"activeRoom": active,
	})
}

// resolveOpenid 按优先级解析 openid：code → X-WX-OPENID → body.Openid
func resolveOpenid(c *gin.Context, body *loginBody) string {
	if body.Code != "" {
		if oid, err := wxauth.Code2Session(body.Code); err == nil {
			return oid
		} else {
			logger.Warn("[login] code2session 失败: %v", err)
		}
	}
	if oid := c.GetHeader("X-WX-OPENID"); oid != "" {
		return oid
	}
	if body.Openid != "" {
		return body.Openid
	}
	return ""
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
