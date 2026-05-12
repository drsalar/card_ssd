// Package storage 房间快照仓储：rooms / room_players 表
package storage

import (
	"database/sql"
)

// RoomDTO 房间快照（与业务层解耦的纯数据结构）
type RoomDTO struct {
	RoomID          string
	HostOpenid      string
	Phase           string
	CurrentRound    int
	TotalRounds     int
	MaxPlayers      int
	WithMa          bool
	LastActiveAt    int64
	AllOfflineSince int64
	Destroyed       bool
}

// PlayerDTO 房间内玩家快照
type PlayerDTO struct {
	RoomID         string
	Openid         string
	Seat           int
	Nickname       string
	AvatarUrl      string
	Score          int
	IsBot          bool
	Offline        bool
	OfflineSince   int64
	HandJSON       string // []game.Card 的 JSON 序列化
	LanesJSON      string // *game.Lanes 的 JSON 序列化（可为空）
	Submitted      bool
	RoundConfirmed bool
	VoteDissolve   bool
}

// SaveRoomSnapshot 保存房间 + 玩家快照（事务内：UPSERT rooms，覆盖 room_players）
func SaveRoomSnapshot(room RoomDTO, players []PlayerDTO) {
	conn := DB()
	if conn == nil || room.RoomID == "" {
		return
	}
	tx, err := conn.Begin()
	if err != nil {
		logWarn("rooms", room.RoomID, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	const upsertRoom = `INSERT INTO rooms
		(room_id, host_openid, phase, current_round, total_rounds, max_players, with_ma,
		 last_active_at, all_offline_since, destroyed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			host_openid       = VALUES(host_openid),
			phase             = VALUES(phase),
			current_round     = VALUES(current_round),
			total_rounds      = VALUES(total_rounds),
			max_players       = VALUES(max_players),
			with_ma           = VALUES(with_ma),
			last_active_at    = VALUES(last_active_at),
			all_offline_since = VALUES(all_offline_since),
			destroyed         = VALUES(destroyed)`
	if _, err := tx.Exec(upsertRoom,
		room.RoomID, room.HostOpenid, room.Phase, room.CurrentRound, room.TotalRounds,
		room.MaxPlayers, boolToInt(room.WithMa), room.LastActiveAt, room.AllOfflineSince,
		boolToInt(room.Destroyed)); err != nil {
		logWarn("rooms", room.RoomID, err)
		return
	}
	if _, err := tx.Exec(`DELETE FROM room_players WHERE room_id=?`, room.RoomID); err != nil {
		logWarn("room_players", room.RoomID, err)
		return
	}
	const insertPlayer = `INSERT INTO room_players
		(room_id, openid, seat, nickname, avatar_url, score, is_bot, offline, offline_since,
		 hand, lanes, submitted, round_confirmed, vote_dissolve)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for _, p := range players {
		if _, err := tx.Exec(insertPlayer,
			room.RoomID, p.Openid, p.Seat, p.Nickname, p.AvatarUrl, p.Score,
			boolToInt(p.IsBot), boolToInt(p.Offline), p.OfflineSince,
			p.HandJSON, p.LanesJSON, boolToInt(p.Submitted),
			boolToInt(p.RoundConfirmed), boolToInt(p.VoteDissolve)); err != nil {
			logWarn("room_players", room.RoomID+"/"+p.Openid, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		logWarn("rooms", room.RoomID, err)
	}
}

// MarkRoomDestroyed 房间销毁：rooms.destroyed=1，并清空对应 room_players
func MarkRoomDestroyed(roomID string) {
	conn := DB()
	if conn == nil || roomID == "" {
		return
	}
	if _, err := conn.Exec(`UPDATE rooms SET destroyed=1 WHERE room_id=?`, roomID); err != nil {
		logWarn("rooms", roomID, err)
	}
	if _, err := conn.Exec(`DELETE FROM room_players WHERE room_id=?`, roomID); err != nil {
		logWarn("room_players", roomID, err)
	}
}

// LoadAliveRooms 读取所有 destroyed=0 的房间及其玩家
func LoadAliveRooms() ([]RoomDTO, map[string][]PlayerDTO) {
	conn := DB()
	if conn == nil {
		return nil, nil
	}
	rows, err := conn.Query(`SELECT room_id, host_openid, phase, current_round, total_rounds,
		max_players, with_ma, last_active_at, all_offline_since, destroyed
		FROM rooms WHERE destroyed=0`)
	if err != nil {
		logWarn("rooms", "load", err)
		return nil, nil
	}
	defer rows.Close()
	var roomList []RoomDTO
	for rows.Next() {
		var r RoomDTO
		var withMa, destroyed int
		if err := rows.Scan(&r.RoomID, &r.HostOpenid, &r.Phase, &r.CurrentRound,
			&r.TotalRounds, &r.MaxPlayers, &withMa, &r.LastActiveAt, &r.AllOfflineSince,
			&destroyed); err != nil {
			logWarn("rooms", "scan", err)
			continue
		}
		r.WithMa = withMa != 0
		r.Destroyed = destroyed != 0
		roomList = append(roomList, r)
	}
	if len(roomList) == 0 {
		return nil, nil
	}
	playerMap := make(map[string][]PlayerDTO, len(roomList))
	prows, err := conn.Query(`SELECT room_id, openid, seat, nickname, avatar_url, score,
		is_bot, offline, offline_since, hand, lanes, submitted, round_confirmed, vote_dissolve
		FROM room_players`)
	if err != nil {
		logWarn("room_players", "load", err)
		return roomList, playerMap
	}
	defer prows.Close()
	for prows.Next() {
		var p PlayerDTO
		var isBot, offline, submitted, confirmed, voted int
		var hand, lanes sql.NullString
		if err := prows.Scan(&p.RoomID, &p.Openid, &p.Seat, &p.Nickname, &p.AvatarUrl,
			&p.Score, &isBot, &offline, &p.OfflineSince, &hand, &lanes,
			&submitted, &confirmed, &voted); err != nil {
			logWarn("room_players", "scan", err)
			continue
		}
		p.IsBot = isBot != 0
		p.Offline = offline != 0
		p.Submitted = submitted != 0
		p.RoundConfirmed = confirmed != 0
		p.VoteDissolve = voted != 0
		if hand.Valid {
			p.HandJSON = hand.String
		}
		if lanes.Valid {
			p.LanesJSON = lanes.String
		}
		playerMap[p.RoomID] = append(playerMap[p.RoomID], p)
	}
	return roomList, playerMap
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
