// Package storage 单元测试：未配置 MYSQL_PWD 时所有 Save*/Load* 应保持空操作（兜底）
package storage

import (
	"os"
	"testing"
	"time"
)

// TestInitWithoutMysqlPwdReturnsDisabled 未配置 MYSQL_PWD 时 Init 不应报错且 Enabled=false
func TestInitWithoutMysqlPwdReturnsDisabled(t *testing.T) {
	old := os.Getenv("MYSQL_PWD")
	_ = os.Unsetenv("MYSQL_PWD")
	defer os.Setenv("MYSQL_PWD", old)

	if err := Init(); err != nil {
		t.Fatalf("Init 应返回 nil，实际：%v", err)
	}
	if Enabled() {
		t.Fatalf("未配置 MYSQL_PWD 时 Enabled 应为 false")
	}
}

// TestSaveAndLoadAreNoopWhenDisabled 未启用持久化时所有写读 API 应为空操作
func TestSaveAndLoadAreNoopWhenDisabled(t *testing.T) {
	old := os.Getenv("MYSQL_PWD")
	_ = os.Unsetenv("MYSQL_PWD")
	defer os.Setenv("MYSQL_PWD", old)

	_ = Init()

	// users
	UpsertUser("test_openid", "nick", "avatar")
	if _, _, ok := GetUser("test_openid"); ok {
		t.Fatalf("DB 未启用时 GetUser 应返回 false")
	}

	// tokens
	SaveToken("tok", "oid", "n", "a", time.Now().Add(time.Hour))
	if _, ok := LoadToken("tok"); ok {
		t.Fatalf("DB 未启用时 LoadToken 应返回 false")
	}

	// rooms / players
	SaveRoomSnapshot(RoomDTO{RoomID: "0001"}, []PlayerDTO{{RoomID: "0001", Openid: "x"}})
	MarkRoomDestroyed("0001")
	if rs, _ := LoadAliveRooms(); rs != nil {
		t.Fatalf("DB 未启用时 LoadAliveRooms 应返回 nil")
	}

	// match results
	SaveMatchResult("0001", 1, true, 5, []byte(`{"a":1}`))
}

// TestCloseIsIdempotent Close 在未初始化时也应安全
func TestCloseIsIdempotent(t *testing.T) {
	if err := Close(); err != nil {
		t.Fatalf("Close 在未初始化时应返回 nil，实际：%v", err)
	}
}
