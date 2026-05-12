// Package storage 持久化层：MySQL 连接池 + DDL 迁移 + 各业务仓储
// 设计要点：
//  1. 「可降级」：未配置 MYSQL_PWD 或连接失败时，DB=nil，所有 Save*/Load* 直接返回 nil，
//     让上层逻辑保持纯内存运行，不阻塞本地调试与生产启动。
//  2. 「最小入侵」：业务方按需调用 SaveXxx；错误一律 WARN 不向上抛 panic。
//  3. 启动时自动执行内置 DDL（CREATE TABLE IF NOT EXISTS），并清理过期 token。
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"card_ssd/internal/logger"

	// 注册 mysql 驱动
	_ "github.com/go-sql-driver/mysql"
)

// 包级 DB 句柄。未启用持久化时为 nil。
var db atomic.Pointer[sql.DB]

// Enabled 是否已建立可用的 MySQL 连接
func Enabled() bool {
	return db.Load() != nil
}

// DB 获取当前 DB 句柄（可能为 nil，调用方需自行 nil 检查）
func DB() *sql.DB {
	return db.Load()
}

// Init 初始化 MySQL 连接池并执行迁移。
// 未配置 MYSQL_PWD 时记录 WARN 后返回 nil（降级为内存模式）。
// 连接 / 迁移失败时记录 ERROR 后返回 nil（同样降级，不阻塞主流程）。
func Init() error {
	pwd := os.Getenv("MYSQL_PWD")
	if pwd == "" {
		logger.Warn("[storage] MYSQL_PWD 未配置，持久化层降级为内存模式")
		return nil
	}
	host := os.Getenv("MYSQL_HOST")
	if host == "" {
		host = "10.31.102.121:3306"
	}
	dbName := os.Getenv("MYSQL_DB")
	if dbName == "" {
		dbName = "card_ssd"
	}
	user := os.Getenv("MYSQL_USER")
	if user == "" {
		user = "root"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		user, pwd, host, dbName)
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		logger.Error("[storage] sql.Open 失败: %v", err)
		return nil
	}
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)
	conn.SetConnMaxLifetime(time.Hour)
	if err := conn.Ping(); err != nil {
		logger.Error("[storage] Ping 失败 host=%s db=%s: %v", host, dbName, err)
		_ = conn.Close()
		return nil
	}
	if err := migrate(conn); err != nil {
		logger.Error("[storage] DDL 迁移失败: %v", err)
		_ = conn.Close()
		return nil
	}
	db.Store(conn)
	logger.Info("[storage] MySQL 已就绪 host=%s db=%s", host, dbName)
	// 异步清理过期 token，不阻塞启动
	go cleanupExpiredTokens()
	return nil
}

// Close 关闭连接池
func Close() error {
	old := db.Swap(nil)
	if old == nil {
		return nil
	}
	return old.Close()
}

// logWarn 业务侧统一的写错处理：记录 WARN 不抛错
func logWarn(table, id string, err error) {
	if err == nil {
		return
	}
	logger.Warn("[storage] table=%s id=%s err=%v", table, id, err)
}
