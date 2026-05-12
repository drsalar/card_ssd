// Package room AllReady 单元测试：验证开局门槛 ≥2 人就绪
package room

import "testing"

// TestAllReadyBelowTwo 1 人 Ready 时应返回 false（最少 2 人才能开局）
func TestAllReadyBelowTwo(t *testing.T) {
	r := NewRoom("8801", Rule{TotalRounds: 5, MaxPlayers: 6}, "u1")
	r.Players = append(r.Players, &Player{Openid: "u1", Ready: true})
	if r.AllReady() {
		t.Errorf("仅 1 人 Ready 不应满足 AllReady")
	}
}

// TestAllReadyTwoToSix 2/3/5/6 人就绪应满足 AllReady
func TestAllReadyTwoToSix(t *testing.T) {
	cases := []int{2, 3, 5, 6}
	for _, n := range cases {
		r := NewRoom("8802", Rule{TotalRounds: 5, MaxPlayers: 6}, "u1")
		for i := 0; i < n; i++ {
			r.Players = append(r.Players, &Player{Openid: itoaSimple(i + 1), Ready: true})
		}
		if !r.AllReady() {
			t.Errorf("%d 人就绪应满足 AllReady", n)
		}
	}
}

// TestAllReadyOneNotReady 任意一人未 Ready（且非 Offline）→ AllReady=false
func TestAllReadyOneNotReady(t *testing.T) {
	r := NewRoom("8803", Rule{TotalRounds: 5, MaxPlayers: 6}, "u1")
	r.Players = append(r.Players,
		&Player{Openid: "u1", Ready: true},
		&Player{Openid: "u2", Ready: true},
		&Player{Openid: "u3", Ready: false}, // 未准备
	)
	if r.AllReady() {
		t.Errorf("有玩家未 Ready 时不应满足 AllReady")
	}
}

// TestAllReadyOfflineSkipped Offline 玩家不参与 Ready 校验
func TestAllReadyOfflineSkipped(t *testing.T) {
	r := NewRoom("8804", Rule{TotalRounds: 5, MaxPlayers: 6}, "u1")
	r.Players = append(r.Players,
		&Player{Openid: "u1", Ready: true},
		&Player{Openid: "u2", Ready: true},
		&Player{Openid: "u3", Ready: false, Offline: true}, // 离线 → 不阻塞开局
	)
	if !r.AllReady() {
		t.Errorf("Offline 玩家不应阻塞 AllReady")
	}
}
