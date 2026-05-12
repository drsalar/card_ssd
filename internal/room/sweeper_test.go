// Package room sweeper 单元测试
package room

import (
	"context"
	"testing"
	"time"
)

// TestSweepIdleRooms 直接验证 sweepIdleRooms 的销毁路径
func TestSweepIdleRooms(t *testing.T) {
	// 准备一个空闲已超阈值的房间
	r1 := NewRoom("9001", Rule{TotalRounds: 5, MaxPlayers: 4}, "userA")
	r1.Phase = PhasePlaying
	r1.AllOfflineSince = time.Now().Add(-25 * time.Hour).UnixMilli()
	// 准备一个未达到阈值的房间
	r2 := NewRoom("9002", Rule{TotalRounds: 5, MaxPlayers: 4}, "userB")
	r2.Phase = PhasePlaying
	r2.AllOfflineSince = time.Now().Add(-time.Minute).UnixMilli()
	// 准备一个未登记 AllOfflineSince 的房间
	r3 := NewRoom("9003", Rule{TotalRounds: 5, MaxPlayers: 4}, "userC")
	r3.Phase = PhasePlaying

	rmu.Lock()
	rooms[r1.ID] = r1
	rooms[r2.ID] = r2
	rooms[r3.ID] = r3
	rmu.Unlock()
	defer func() {
		// 清理房间表，避免污染其他测试
		rmu.Lock()
		delete(rooms, "9001")
		delete(rooms, "9002")
		delete(rooms, "9003")
		rmu.Unlock()
	}()

	sweepIdleRooms(24 * time.Hour)

	if got := GetRoom("9001"); got != nil {
		t.Errorf("房间 9001 应被销毁，实际仍存在")
	}
	if got := GetRoom("9002"); got == nil {
		t.Errorf("房间 9002 不应被销毁，实际已销毁")
	}
	if got := GetRoom("9003"); got == nil {
		t.Errorf("房间 9003 不应被销毁，实际已销毁")
	}
}

// TestStartIdleSweeperLifecycle 验证巡检协程能按 ticker 触发并响应 ctx.Done()
func TestStartIdleSweeperLifecycle(t *testing.T) {
	r := NewRoom("9101", Rule{TotalRounds: 5, MaxPlayers: 4}, "userA")
	r.Phase = PhasePlaying
	r.AllOfflineSince = time.Now().Add(-time.Hour).UnixMilli()
	rmu.Lock()
	rooms[r.ID] = r
	rmu.Unlock()
	defer func() {
		rmu.Lock()
		delete(rooms, "9101")
		rmu.Unlock()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartIdleSweeper(ctx, 10*time.Millisecond, 20*time.Millisecond)
	// 等待巡检命中
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if GetRoom("9101") == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("房间 9101 在巡检阈值满足后仍未被销毁")
}

// TestFindActiveRoomLatestFirst 验证多房间命中时返回最新一条
func TestFindActiveRoomLatestFirst(t *testing.T) {
	openid := "test_user_latest"
	rA := NewRoom("9201", Rule{TotalRounds: 5, MaxPlayers: 4}, openid)
	rA.Players = append(rA.Players, &Player{Openid: openid, Nickname: "A"})
	rA.LastActiveAt = time.Now().Add(-time.Minute).UnixMilli()
	rB := NewRoom("9202", Rule{TotalRounds: 5, MaxPlayers: 4}, openid)
	rB.Players = append(rB.Players, &Player{Openid: openid, Nickname: "B"})
	rB.LastActiveAt = time.Now().UnixMilli()

	rmu.Lock()
	rooms[rA.ID] = rA
	rooms[rB.ID] = rB
	rmu.Unlock()
	defer func() {
		rmu.Lock()
		delete(rooms, "9201")
		delete(rooms, "9202")
		rmu.Unlock()
	}()

	got := FindActiveRoomByOpenid(openid)
	if got == nil {
		t.Fatalf("应找到活跃房间，实际为空")
	}
	if got.RoomID != "9202" {
		t.Errorf("应返回最新房间 9202，实际返回 %s", got.RoomID)
	}
}
