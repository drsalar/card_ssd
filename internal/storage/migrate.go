// Package storage DDL 迁移：服务启动时自动 CREATE TABLE IF NOT EXISTS
package storage

import (
	"database/sql"
	"time"

	"card_ssd/internal/logger"
)

// ddlStatements 5 张表 DDL（按依赖顺序执行）
var ddlStatements = []string{
	// 用户档案：openid 主键
	`CREATE TABLE IF NOT EXISTS users (
		openid      VARCHAR(64)  NOT NULL,
		nickname    VARCHAR(64)  NOT NULL DEFAULT '',
		avatar_url  VARCHAR(512) NOT NULL DEFAULT '',
		created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (openid)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	// 登录 token
	`CREATE TABLE IF NOT EXISTS auth_tokens (
		token       VARCHAR(64)  NOT NULL,
		openid      VARCHAR(64)  NOT NULL,
		nickname    VARCHAR(64)  NOT NULL DEFAULT '',
		avatar_url  VARCHAR(512) NOT NULL DEFAULT '',
		expires_at  DATETIME     NOT NULL,
		created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (token),
		KEY idx_openid (openid),
		KEY idx_expires (expires_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	// 房间
	`CREATE TABLE IF NOT EXISTS rooms (
		room_id            VARCHAR(8)  NOT NULL,
		host_openid        VARCHAR(64) NOT NULL DEFAULT '',
		phase              VARCHAR(16) NOT NULL DEFAULT 'waiting',
		current_round      INT         NOT NULL DEFAULT 0,
		total_rounds       INT         NOT NULL DEFAULT 5,
		max_players        INT         NOT NULL DEFAULT 4,
		with_ma            TINYINT     NOT NULL DEFAULT 0,
		last_active_at     BIGINT      NOT NULL DEFAULT 0,
		all_offline_since  BIGINT      NOT NULL DEFAULT 0,
		destroyed          TINYINT     NOT NULL DEFAULT 0,
		updated_at         DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (room_id),
		KEY idx_destroyed (destroyed),
		KEY idx_last_active (last_active_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	// 房间内玩家：复合主键 (room_id, openid)
	`CREATE TABLE IF NOT EXISTS room_players (
		room_id          VARCHAR(8)  NOT NULL,
		openid           VARCHAR(64) NOT NULL,
		seat             INT         NOT NULL DEFAULT 0,
		nickname         VARCHAR(64) NOT NULL DEFAULT '',
		avatar_url       VARCHAR(512) NOT NULL DEFAULT '',
		score            INT         NOT NULL DEFAULT 0,
		is_bot           TINYINT     NOT NULL DEFAULT 0,
		offline          TINYINT     NOT NULL DEFAULT 1,
		offline_since    BIGINT      NOT NULL DEFAULT 0,
		hand             TEXT        NULL,
		lanes            TEXT        NULL,
		submitted        TINYINT     NOT NULL DEFAULT 0,
		round_confirmed  TINYINT     NOT NULL DEFAULT 0,
		vote_dissolve    TINYINT     NOT NULL DEFAULT 0,
		PRIMARY KEY (room_id, openid),
		KEY idx_openid (openid)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	// 对局结果（per-round）
	`CREATE TABLE IF NOT EXISTS match_results (
		id            BIGINT       NOT NULL AUTO_INCREMENT,
		room_id       VARCHAR(8)   NOT NULL,
		round         INT          NOT NULL DEFAULT 0,
		with_ma       TINYINT      NOT NULL DEFAULT 0,
		total_rounds  INT          NOT NULL DEFAULT 0,
		payload       JSON         NULL,
		created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_room_round (room_id, round),
		KEY idx_created (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
}

// migrate 顺序执行所有 DDL
func migrate(conn *sql.DB) error {
	for i, stmt := range ddlStatements {
		if _, err := conn.Exec(stmt); err != nil {
			return wrapMigrate(i, err)
		}
	}
	return nil
}

// wrapMigrate 简单包一层错误
func wrapMigrate(idx int, err error) error {
	logger.Error("[storage] ddl[%d] 失败: %v", idx, err)
	return err
}

// cleanupExpiredTokens 清理过期 token，启动时执行一次
func cleanupExpiredTokens() {
	conn := DB()
	if conn == nil {
		return
	}
	res, err := conn.Exec("DELETE FROM auth_tokens WHERE expires_at < ?", time.Now())
	if err != nil {
		logger.Warn("[storage] 清理过期 token 失败: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		logger.Info("[storage] 启动清理过期 token=%d", n)
	}
}
