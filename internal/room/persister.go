// Package room 房间持久化（节流批量落库）
// persister.go: 维护 dirty 标记 + 1 秒节流的后台 goroutine，将房间快照写入 storage 层
package room

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"card_ssd/internal/logger"
	"card_ssd/internal/storage"
)

// PersistInterval 持久化节流间隔
const PersistInterval = time.Second

// MarkDirty 将房间标记为脏（调用前应已持有 r.mu）
// 业务关键写入操作（CreateRoom 后、JoinRoom、LeaveRoom、Settle、Touch、阶段切换、AllOfflineSince 变化等）都应调用一次
func (r *Room) MarkDirty() {
	r.dirty = true
}

// IsDirty 调用前应已持有 r.mu
func (r *Room) IsDirty() bool { return r.dirty }

// clearDirty 清除脏标记（调用前应已持有 r.mu）
func (r *Room) clearDirty() { r.dirty = false }

// toSnapshot 把 Room 转成 storage 的 DTO（调用前应已持有 r.mu）
func (r *Room) toSnapshot() (storage.RoomDTO, []storage.PlayerDTO) {
	rd := storage.RoomDTO{
		RoomID:          r.ID,
		HostOpenid:      r.HostID,
		Phase:           string(r.Phase),
		CurrentRound:    r.CurrentRound,
		TotalRounds:     r.Rule.TotalRounds,
		MaxPlayers:      r.Rule.MaxPlayers,
		WithMa:          r.Rule.WithMa,
		LastActiveAt:    r.LastActiveAt,
		AllOfflineSince: r.AllOfflineSince,
		Destroyed:       r.destroyed,
	}
	players := make([]storage.PlayerDTO, 0, len(r.Players))
	for i, p := range r.Players {
		handJSON := ""
		if len(p.Hand) > 0 {
			if buf, err := json.Marshal(p.Hand); err == nil {
				handJSON = string(buf)
			}
		}
		lanesJSON := ""
		if p.Lanes != nil {
			if buf, err := json.Marshal(p.Lanes); err == nil {
				lanesJSON = string(buf)
			}
		}
		players = append(players, storage.PlayerDTO{
			RoomID:         r.ID,
			Openid:         p.Openid,
			Seat:           i,
			Nickname:       p.Nickname,
			AvatarUrl:      p.AvatarUrl,
			Score:          p.Score,
			IsBot:          p.IsBot,
			Offline:        p.Offline,
			OfflineSince:   p.OfflineSince,
			HandJSON:       handJSON,
			LanesJSON:      lanesJSON,
			Submitted:      p.Submitted,
			RoundConfirmed: p.RoundConfirmed,
			VoteDissolve:   p.VoteDissolve,
		})
	}
	return rd, players
}

// persisterRunning 守护单实例
var (
	persisterMu      sync.Mutex
	persisterStarted bool
)

// StartPersister 启动后台节流持久化协程
// 进程仅启动一次；ctx Done 时退出
func StartPersister(ctx context.Context) {
	persisterMu.Lock()
	if persisterStarted {
		persisterMu.Unlock()
		return
	}
	persisterStarted = true
	persisterMu.Unlock()
	go func() {
		ticker := time.NewTicker(PersistInterval)
		defer ticker.Stop()
		logger.Info("房间持久化节流任务已启动 interval=%s", PersistInterval)
		for {
			select {
			case <-ctx.Done():
				logger.Info("房间持久化节流任务退出")
				FlushAll()
				return
			case <-ticker.C:
				flushDirtyRooms()
			}
		}
	}()
}

// flushDirtyRooms 单次扫描：把所有 dirty=true 的房间快照落库
func flushDirtyRooms() {
	if !storage.Enabled() {
		return
	}
	rmu.Lock()
	candidates := make([]*Room, 0, len(rooms))
	for _, r := range rooms {
		candidates = append(candidates, r)
	}
	rmu.Unlock()
	for _, r := range candidates {
		r.Lock()
		if !r.dirty {
			r.Unlock()
			continue
		}
		rd, ps := r.toSnapshot()
		r.clearDirty()
		r.Unlock()
		storage.SaveRoomSnapshot(rd, ps)
	}
}

// FlushAll 立即同步刷盘（用于进程退出 / 关键节点）
func FlushAll() {
	if !storage.Enabled() {
		return
	}
	flushDirtyRooms()
}

// SaveNow 立即同步保存单个房间（用于销毁前等关键节点）
// 调用方不应持有 r.mu
func SaveNow(r *Room) {
	if r == nil || !storage.Enabled() {
		return
	}
	r.Lock()
	rd, ps := r.toSnapshot()
	r.clearDirty()
	r.Unlock()
	storage.SaveRoomSnapshot(rd, ps)
}
