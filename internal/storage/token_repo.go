// Package storage token 仓储：对应 auth_tokens 表
package storage

import (
	"database/sql"
	"errors"
	"time"
)

// TokenInfo token 对应的身份信息（与 session.TokenInfo 一致；放在 storage 包内独立声明避免循环依赖）
type TokenInfo struct {
	Openid    string
	Nickname  string
	AvatarUrl string
}

// SaveToken 保存 token → 身份映射
func SaveToken(token, openid, nickname, avatarUrl string, expiresAt time.Time) {
	conn := DB()
	if conn == nil || token == "" || openid == "" {
		return
	}
	const sqlStr = `INSERT INTO auth_tokens (token, openid, nickname, avatar_url, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			openid     = VALUES(openid),
			nickname   = VALUES(nickname),
			avatar_url = VALUES(avatar_url),
			expires_at = VALUES(expires_at)`
	if _, err := conn.Exec(sqlStr, token, openid, nickname, avatarUrl, expiresAt); err != nil {
		logWarn("auth_tokens", token, err)
	}
}

// LoadToken 读取 token；过期或不存在均返回 ok=false
func LoadToken(token string) (TokenInfo, bool) {
	conn := DB()
	if conn == nil || token == "" {
		return TokenInfo{}, false
	}
	var (
		info      TokenInfo
		expiresAt time.Time
	)
	row := conn.QueryRow(`SELECT openid, nickname, avatar_url, expires_at FROM auth_tokens WHERE token=?`, token)
	if err := row.Scan(&info.Openid, &info.Nickname, &info.AvatarUrl, &expiresAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logWarn("auth_tokens", token, err)
		}
		return TokenInfo{}, false
	}
	if expiresAt.Before(time.Now()) {
		return TokenInfo{}, false
	}
	return info, true
}
