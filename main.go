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
)

func main() {
	// 读取端口环境变量，默认 80
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	addr := ":" + port

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 关闭所有 WebSocket
	server.Shutdown()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP 关闭异常: %v", err)
	}
	logger.Info("服务端已退出")
}
