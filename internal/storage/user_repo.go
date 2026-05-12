// Package storage 用户档案仓储：对应 users 表
package storage

import (
	"database/sql"
	"errors"
)

// UpsertUser 写入或更新用户档案
// 仅当 nickname/avatar 非空时覆盖，避免空值清空已有资料。
func UpsertUser(openid, nickname, avatarUrl string) {
	conn := DB()
	if conn == nil || openid == "" {
		return
	}
	// MySQL 原生 ON DUPLICATE KEY 写法：
	// nickname=IF(VALUES(nickname)='', nickname, VALUES(nickname))
	const sqlStr = `INSERT INTO users (openid, nickname, avatar_url)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			nickname   = IF(VALUES(nickname)='',   nickname,   VALUES(nickname)),
			avatar_url = IF(VALUES(avatar_url)='', avatar_url, VALUES(avatar_url))`
	if _, err := conn.Exec(sqlStr, openid, nickname, avatarUrl); err != nil {
		logWarn("users", openid, err)
	}
}

// GetUser 读取用户档案；不存在返回 ok=false
func GetUser(openid string) (nickname, avatarUrl string, ok bool) {
	conn := DB()
	if conn == nil || openid == "" {
		return "", "", false
	}
	row := conn.QueryRow(`SELECT nickname, avatar_url FROM users WHERE openid=?`, openid)
	if err := row.Scan(&nickname, &avatarUrl); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logWarn("users", openid, err)
		}
		return "", "", false
	}
	return nickname, avatarUrl, true
}
