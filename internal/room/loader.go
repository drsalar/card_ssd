// Package room 启动时从持久化层恢复未结束房间
// loader.go: 进程启动后由 main 调用一次，把 destroyed=0 的房间及玩家全部还原到内存
package room

import (
	"encoding/json"
	"time"

	"card_ssd/internal/game"
	"card_ssd/internal/logger"
	"card_ssd/internal/storage"
)

// LoadFromStorage 从持久化层恢复所有未销毁房间。
// 仅加载 destroyed=0 的行；所有玩家初始化为 Offline=true、Conn 为空，等待客户端重连。
// 调用前应保证 storage.Init 已执行；DB 未启用时直接返回。
func LoadFromStorage() {
	if !storage.Enabled() {
		return
	}
	roomList, playerMap := storage.LoadAliveRooms()
	if len(roomList) == 0 {
		return
	}
	now := time.Now().UnixMilli()
	rmu.Lock()
	defer rmu.Unlock()
	loaded := 0
	for _, rd := range roomList {
		if rd.Destroyed {
			continue
		}
		if _, exists := rooms[rd.RoomID]; exists {
			continue
		}
		r := NewRoom(rd.RoomID, Rule{
			WithMa:      rd.WithMa,
			TotalRounds: rd.TotalRounds,
			MaxPlayers:  rd.MaxPlayers,
		}, rd.HostOpenid)
		r.Phase = Phase(rd.Phase)
		r.CurrentRound = rd.CurrentRound
		r.LastActiveAt = rd.LastActiveAt
		r.AllOfflineSince = rd.AllOfflineSince
		// 还原玩家
		for _, pd := range playerMap[rd.RoomID] {
			p := &Player{
				Openid:         pd.Openid,
				Nickname:       pd.Nickname,
				AvatarUrl:      pd.AvatarUrl,
				Score:          pd.Score,
				IsBot:          pd.IsBot,
				Offline:        true, // 进程刚重启，统一视为离线，等真人重连
				OfflineSince:   pd.OfflineSince,
				Submitted:      pd.Submitted,
				RoundConfirmed: pd.RoundConfirmed,
				VoteDissolve:   pd.VoteDissolve,
			}
			if !p.IsBot && p.OfflineSince == 0 {
				p.OfflineSince = now
			}
			if pd.HandJSON != "" {
				var hand []game.Card
				if err := json.Unmarshal([]byte(pd.HandJSON), &hand); err == nil {
					p.Hand = hand
				}
			}
			if pd.LanesJSON != "" {
				var lanes game.Lanes
				if err := json.Unmarshal([]byte(pd.LanesJSON), &lanes); err == nil {
					p.Lanes = &lanes
				}
			}
			r.Players = append(r.Players, p)
		}
		// 进程刚重启，全员真人都不在线，AllOfflineSince 兜底为当前时间（若数据库未存）
		// 所有阶段（waiting/playing/comparing/match_end）统一纳入 24h 保活
		if r.AllOfflineSince == 0 {
			r.AllOfflineSince = now
		}
		rooms[rd.RoomID] = r
		loaded++
	}
	logger.Info("从持久化层恢复房间 count=%d", loaded)
}
