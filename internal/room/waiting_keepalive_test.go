// Package room waiting 阶段断线保活相关单元测试
package room

import (
	"testing"
	"time"
)

// TestWaitingDisconnectKeepsRoom waiting 阶段唯一真人断线后房间不销毁，AllOfflineSince 已登记
func TestWaitingDisconnectKeepsRoom(t *testing.T) {
	const openid = "waiting_keepalive_user1"
	const roomID = "9301"
	r := NewRoom(roomID, Rule{TotalRounds: 5, MaxPlayers: 4}, openid)
	// 仅有一名真人玩家
	r.Players = append(r.Players, &Player{Openid: openid, Nickname: "P1"})
	// 注册到全局表，模拟正常房间
	rmu.Lock()
	rooms[r.ID] = r
	rmu.Unlock()
	defer cleanupRoom(roomID)

	// 模拟 HandleDisconnect 中对 waiting 阶段玩家的处理：
	// 标记 Offline + 登记 AllOfflineSince，不再 RemovePlayer
	r.Lock()
	p := r.GetPlayer(openid)
	p.Offline = true
	p.OfflineSince = time.Now().UnixMilli()
	markAllOfflineIfNeeded(r)
	r.Unlock()

	// 房间仍存在
	if got := GetRoom(roomID); got == nil {
		t.Fatalf("waiting 阶段断线后房间应仍存在，实际已被销毁")
	}
	// 玩家仍在 Players 列表，未被移除
	r.Lock()
	if r.GetPlayer(openid) == nil {
		t.Errorf("waiting 阶段断线不应移除玩家")
	}
	if r.AllOfflineSince <= 0 {
		t.Errorf("AllOfflineSince 应已登记，实际=%d", r.AllOfflineSince)
	}
	r.Unlock()
}

// TestWaitingReconnectClearsAllOffline waiting 阶段断线后再 JoinRoom 重连：
// Offline 复位为 false，AllOfflineSince 清零
func TestWaitingReconnectClearsAllOffline(t *testing.T) {
	const openid = "waiting_keepalive_user2"
	const roomID = "9302"
	r := NewRoom(roomID, Rule{TotalRounds: 5, MaxPlayers: 4}, openid)
	r.Players = append(r.Players, &Player{
		Openid:       openid,
		Nickname:     "P2",
		Offline:      true,
		OfflineSince: time.Now().UnixMilli(),
	})
	r.AllOfflineSince = time.Now().UnixMilli()
	rmu.Lock()
	rooms[r.ID] = r
	rmu.Unlock()
	defer cleanupRoom(roomID)

	// 直接复用 ReconnectPlayer 内部逻辑（避免依赖 session.Session 重构造）
	r.Lock()
	p := r.GetPlayer(openid)
	p.Offline = false
	p.OfflineSince = 0
	markAllOfflineIfNeeded(r) // 有在线真人 → 清零
	r.Unlock()

	r.Lock()
	defer r.Unlock()
	if r.GetPlayer(openid).Offline {
		t.Errorf("重连后 Offline 应为 false")
	}
	if r.AllOfflineSince != 0 {
		t.Errorf("有在线真人时 AllOfflineSince 应清零，实际=%d", r.AllOfflineSince)
	}
}

// TestWaitingAllOfflineSweep waiting 阶段所有真人断线超过 24h 后被 sweeper 销毁
func TestWaitingAllOfflineSweep(t *testing.T) {
	const roomID = "9303"
	r := NewRoom(roomID, Rule{TotalRounds: 5, MaxPlayers: 4}, "ownerW")
	r.Players = append(r.Players,
		&Player{Openid: "userW1", Nickname: "W1", Offline: true},
		&Player{Openid: "userW2", Nickname: "W2", Offline: true},
	)
	// AllOfflineSince 设为 25 小时前，应被巡检销毁
	r.AllOfflineSince = time.Now().Add(-25 * time.Hour).UnixMilli()
	rmu.Lock()
	rooms[r.ID] = r
	rmu.Unlock()
	defer cleanupRoom(roomID)

	sweepIdleRooms(24 * time.Hour)

	if got := GetRoom(roomID); got != nil {
		t.Errorf("waiting 阶段全员真人离线 24h 后房间应被销毁")
	}
}

// TestWaitingActiveLeaveRemovesPlayer waiting 阶段主动 LeaveRoom（对应客户端「退出」按钮）
// 仍走 RemovePlayer / 无真人则销毁的原路径
func TestWaitingActiveLeaveRemovesPlayer(t *testing.T) {
	const openid = "waiting_keepalive_user3"
	const roomID = "9304"
	r := NewRoom(roomID, Rule{TotalRounds: 5, MaxPlayers: 4}, openid)
	r.Players = append(r.Players, &Player{Openid: openid, Nickname: "P3"})
	rmu.Lock()
	rooms[r.ID] = r
	rmu.Unlock()
	defer cleanupRoom(roomID)

	// 模拟 LeaveRoom 在 waiting 阶段的核心步骤
	r.Lock()
	r.RemovePlayer(openid)
	humanLeft := r.HumanCount()
	r.Unlock()

	if humanLeft != 0 {
		t.Fatalf("LeaveRoom 后应无真人，实际=%d", humanLeft)
	}
	// HumanCount=0 → 由调用方 DestroyRoom（这里直接显式触发）
	DestroyRoom(roomID)
	if got := GetRoom(roomID); got != nil {
		t.Errorf("waiting 主动离开且无真人时房间应被销毁")
	}
}

// TestWaitingHasBotKeepsRoom waiting 阶段所有真人 Offline、但还有 bot：
// 房间不立即销毁，沿用 AllOfflineSince 24h 销毁
func TestWaitingHasBotKeepsRoom(t *testing.T) {
	const roomID = "9305"
	r := NewRoom(roomID, Rule{TotalRounds: 5, MaxPlayers: 4}, "ownerB")
	r.Players = append(r.Players,
		&Player{Openid: "userB1", Nickname: "B1", Offline: true},
		&Player{Openid: "bot_x", Nickname: "电脑1", IsBot: true},
	)
	rmu.Lock()
	rooms[r.ID] = r
	rmu.Unlock()
	defer cleanupRoom(roomID)

	// markAllOfflineIfNeeded 应在无在线真人时登记 AllOfflineSince
	r.Lock()
	markAllOfflineIfNeeded(r)
	since := r.AllOfflineSince
	r.Unlock()

	if since <= 0 {
		t.Errorf("waiting 房间无在线真人但有 bot 时应登记 AllOfflineSince")
	}
	if got := GetRoom(roomID); got == nil {
		t.Errorf("waiting 房间无在线真人但有 bot 时不应被立即销毁")
	}
	// 仅在远未达阈值时才不销毁；做一次小阈值巡检确认行为
	sweepIdleRooms(24 * time.Hour)
	if got := GetRoom(roomID); got == nil {
		t.Errorf("waiting 房间刚断线就被巡检销毁是错误行为")
	}
}

// cleanupRoom 测试结束后清理房间表，避免污染其他测试
func cleanupRoom(id string) {
	rmu.Lock()
	delete(rooms, id)
	rmu.Unlock()
}
