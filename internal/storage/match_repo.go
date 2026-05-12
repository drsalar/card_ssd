// Package storage 对局结果仓储：match_results 表
package storage

import (
	"encoding/json"
)

// SaveMatchResult 保存某一局结算结果
// payload 已在调用方完成 json.Marshal（避免本包反向依赖 game 包）
func SaveMatchResult(roomID string, round int, withMa bool, totalRounds int, payload []byte) {
	conn := DB()
	if conn == nil || roomID == "" {
		return
	}
	if !json.Valid(payload) {
		// 容错：非合法 JSON 时存空对象
		payload = []byte(`{}`)
	}
	const sqlStr = `INSERT INTO match_results
		(room_id, round, with_ma, total_rounds, payload)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := conn.Exec(sqlStr, roomID, round, boolToInt(withMa), totalRounds, string(payload)); err != nil {
		logWarn("match_results", roomID, err)
	}
}
