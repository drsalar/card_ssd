// Package main 是 card_ssd Go 服务端的入口
// 启动 HTTP（Gin）+ WebSocket（gorilla/websocket）双协议服务
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"card_ssd/internal/logger"
	"card_ssd/internal/room"
	"card_ssd/internal/server"
	"card_ssd/internal/storage"
)

func main() {
	// 读取端口环境变量，默认 80
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	addr := ":" + port

	// 初始化持久化层（未配置 MYSQL_PWD 时降级为内存模式，不阻塞启动）
	_ = storage.Init()

	// 从持久化层恢复未销毁的房间（DB 未启用时为空操作）
	room.LoadFromStorage()

	// 构建 Gin 路由（同时挂载 HTTP API 与 /ws WebSocket）
	engine := server.NewEngine()

	httpServer := &http.Server{
		Addr:    addr,
		Handler: engine,
	}

	// 启动空闲房间巡检（每小时一次，全员真人离线超 24 小时即销毁）
	sweeperCtx, cancelSweeper := context.WithCancel(context.Background())
	defer cancelSweeper()
	room.StartIdleSweeper(sweeperCtx, room.SweeperDefaultInterval, room.SweeperDefaultThreshold)

	// 启动房间持久化节流任务（dirty 标记每秒批量落库）
	persisterCtx, cancelPersister := context.WithCancel(context.Background())
	defer cancelPersister()
	room.StartPersister(persisterCtx)

	// 异步启动 HTTP 服务
	go func() {
		logger.Info("服务端启动监听 %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP 监听失败: %v", err)
			os.Exit(1)
		}
	}()

	// 监听退出信号，优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("收到退出信号，开始优雅关闭...")

	// 取消巡检任务
	cancelSweeper()
	cancelPersister()
	// 在关闭 HTTP / 连接前刷一次最后的脏数据
	room.FlushAll()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 关闭所有 WebSocket
	server.Shutdown()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP 关闭异常: %v", err)
	}
	// 关闭 MySQL 连接池
	_ = storage.Close()
	logger.Info("服务端已退出")
}
