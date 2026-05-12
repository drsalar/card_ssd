// Package session 会话管理：openid 索引、token 索引、顶号、批量关闭
package session

import (
	"sync"
	"time"

	"card_ssd/internal/storage"
)

// TokenInfo HTTP 登录后存储的身份信息
type TokenInfo struct {
	Openid    string
	Nickname  string
	AvatarUrl string
}

// TokenTTL token 默认有效期（7 天）
const TokenTTL = 7 * 24 * time.Hour

var (
	mu sync.RWMutex
	// connID -> *Session
	byConnID = make(map[int64]*Session)
	// openid -> *Session（用于断线重连查找与顶号）
	byOpenid = make(map[string]*Session)
	// token -> TokenInfo（HTTP 登录的临时凭证）
	tokens = make(map[string]TokenInfo)
)

// Add 将新创建的 Session 加入连接表
func Add(s *Session) {
	mu.Lock()
	defer mu.Unlock()
	byConnID[s.ConnID] = s
}

// Remove 从连接表移除会话
func Remove(s *Session) {
	if s == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	delete(byConnID, s.ConnID)
	if s.Openid != "" && byOpenid[s.Openid] == s {
		delete(byOpenid, s.Openid)
	}
}

// BindOpenid 绑定 openid（登录时调用），如有同 openid 旧连接则关闭
func BindOpenid(s *Session, openid string) {
	mu.Lock()
	prev := byOpenid[openid]
	if prev != nil && prev != s {
		// 关闭旧连接
		delete(byConnID, prev.ConnID)
	}
	s.Openid = openid
	byOpenid[openid] = s
	mu.Unlock()
	if prev != nil && prev != s {
		prev.Close()
	}
}

// GetByOpenid 通过 openid 查找会话
func GetByOpenid(openid string) *Session {
	mu.RLock()
	defer mu.RUnlock()
	return byOpenid[openid]
}

// SaveToken 保存 token → 身份映射
// 同时写入持久化层（DB 未启用时为空操作）
func SaveToken(token string, info TokenInfo) {
	mu.Lock()
	tokens[token] = info
	mu.Unlock()
	storage.SaveToken(token, info.Openid, info.Nickname, info.AvatarUrl, time.Now().Add(TokenTTL))
}

// LookupByToken 通过 token 查找身份
// 内存命中即返回；未命中时回查 DB 并补回内存（DB 未启用或不存在 / 已过期返回 false）
func LookupByToken(token string) (TokenInfo, bool) {
	mu.RLock()
	info, ok := tokens[token]
	mu.RUnlock()
	if ok {
		return info, true
	}
	dbInfo, found := storage.LoadToken(token)
	if !found {
		return TokenInfo{}, false
	}
	info = TokenInfo{
		Openid:    dbInfo.Openid,
		Nickname:  dbInfo.Nickname,
		AvatarUrl: dbInfo.AvatarUrl,
	}
	mu.Lock()
	tokens[token] = info
	mu.Unlock()
	return info, true
}

// CountConns 当前连接数
func CountConns() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(byConnID)
}

// CloseAll 关闭所有会话（优雅退出时调用）
func CloseAll() {
	mu.Lock()
	conns := make([]*Session, 0, len(byConnID))
	for _, s := range byConnID {
		conns = append(conns, s)
	}
	byConnID = make(map[int64]*Session)
	byOpenid = make(map[string]*Session)
	mu.Unlock()
	for _, s := range conns {
		s.Close()
	}
}
