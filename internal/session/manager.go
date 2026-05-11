// Package session 会话管理：openid 索引、token 索引、顶号、批量关闭
package session

import (
	"sync"
)

// TokenInfo HTTP 登录后存储的身份信息
type TokenInfo struct {
	Openid    string
	Nickname  string
	AvatarUrl string
}

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
func SaveToken(token string, info TokenInfo) {
	mu.Lock()
	defer mu.Unlock()
	tokens[token] = info
}

// LookupByToken 通过 token 查找身份
func LookupByToken(token string) (TokenInfo, bool) {
	mu.RLock()
	defer mu.RUnlock()
	info, ok := tokens[token]
	return info, ok
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
