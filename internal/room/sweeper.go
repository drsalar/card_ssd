// Package room 房间空闲巡检
// sweeper.go: 周期性遍历所有未销毁房间，销毁“全员真人离线超过阈值”的房间
package room

import (
	"context"
	"time"

	"card_ssd/internal/logger"
)

// SweeperDefaultInterval 默认巡检间隔（每小时一次）
const SweeperDefaultInterval = time.Hour

// SweeperDefaultThreshold 默认空闲销毁阈值（24 小时）
const SweeperDefaultThreshold = 24 * time.Hour

// StartIdleSweeper 启动一个后台巡检协程：每 interval 触发一次，
// 销毁满足 AllOfflineSince > 0 且 now-AllOfflineSince >= threshold 的房间
// 通过 ctx 取消，进程退出时调用即可
func StartIdleSweeper(ctx context.Context, interval, threshold time.Duration) {
	if interval <= 0 {
		interval = SweeperDefaultInterval
	}
	if threshold <= 0 {
		threshold = SweeperDefaultThreshold
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		logger.Info("空闲房间巡检任务已启动 interval=%s threshold=%s", interval, threshold)
		for {
			select {
			case <-ctx.Done():
				logger.Info("空闲房间巡检任务退出")
				return
			case <-ticker.C:
				sweepIdleRooms(threshold)
			}
		}
	}()
}

// sweepIdleRooms 单次巡检：遍历快照、销毁达阈值的房间
// 为避免长时间持有 rmu，先复制候选切片，再针对每个房间独立加锁
func sweepIdleRooms(threshold time.Duration) {
	rmu.Lock()
	candidates := make([]*Room, 0, len(rooms))
	for _, r := range rooms {
		candidates = append(candidates, r)
	}
	rmu.Unlock()
	now := time.Now().UnixMilli()
	thresholdMs := threshold.Milliseconds()
	toDestroy := make([]string, 0)
	for _, r := range candidates {
		r.Lock()
		if r.IsDestroyed() {
			r.Unlock()
			continue
		}
		if r.AllOfflineSince > 0 && now-r.AllOfflineSince >= thresholdMs {
			toDestroy = append(toDestroy, r.ID)
		}
		r.Unlock()
	}
	for _, id := range toDestroy {
		logger.Info("巡检命中：销毁房间 %s 原因=24h all-offline timeout", id)
		DestroyRoom(id)
	}
}
