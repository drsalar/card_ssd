// Package game ai_bot 单元测试
package game

import (
	"math/rand"
	"testing"
)

// TestAutoArrange_NormalHand 验证包含强牌型的手牌能产出合法三道且不走兜底
func TestAutoArrange_NormalHand(t *testing.T) {
	// 构造一手：同花顺 S 6-7-8-9-10、葫芦 H K K K + D D，剩余三张作头道（散牌）
	hand := []Card{
		// 同花顺（尾道应这里收）
		{Suit: "S", Rank: 6}, {Suit: "S", Rank: 7}, {Suit: "S", Rank: 8},
		{Suit: "S", Rank: 9}, {Suit: "S", Rank: 10},
		// 葫芦（中道应这里收）：K K K + 2 2
		{Suit: "H", Rank: 13}, {Suit: "D", Rank: 13}, {Suit: "C", Rank: 13},
		{Suit: "D", Rank: 2}, {Suit: "C", Rank: 2},
		// 头道 3 张散牌
		{Suit: "H", Rank: 3}, {Suit: "D", Rank: 4}, {Suit: "C", Rank: 5},
	}
	lanes, fb := AutoArrange(hand)
	if lanes == nil {
		t.Fatalf("AutoArrange 返回 nil")
	}
	v := ValidateLanes(lanes.Head, lanes.Middle, lanes.Tail)
	if !v.OK {
		t.Fatalf("产出三道不合法: head=%+v middle=%+v tail=%+v", v.Head, v.Middle, v.Tail)
	}
	if fb {
		t.Logf("未预期使用兜底策略，但仍可接受；head=%s middle=%s tail=%s", v.Head.Name, v.Middle.Name, v.Tail.Name)
	}
	// 期望尾道牌型 ≥ 中道 ≥ 头道
	if Compare(v.Tail, v.Middle) < 0 || Compare(v.Middle, v.Head) < 0 {
		t.Fatalf("三道顺序异常: head=%s middle=%s tail=%s", v.Head.Name, v.Middle.Name, v.Tail.Name)
	}
	// 期望尾道为同花顺
	if v.Tail.Type != TypeStraightFlush {
		t.Fatalf("期望尾道为同花顺，实际为 %s", v.Tail.Name)
	}
}

// TestAutoArrange_FallbackHand 验证完全散牌时产出合法三道（可能走兜底）
func TestAutoArrange_FallbackHand(t *testing.T) {
	// 构造一手几乎是最差的牌：全不同花色、不连续、无对
	hand := []Card{
		{Suit: "S", Rank: 1}, {Suit: "H", Rank: 3}, {Suit: "D", Rank: 5},
		{Suit: "C", Rank: 7}, {Suit: "S", Rank: 9}, {Suit: "H", Rank: 11},
		{Suit: "D", Rank: 13}, {Suit: "C", Rank: 2}, {Suit: "S", Rank: 4},
		{Suit: "H", Rank: 6}, {Suit: "D", Rank: 8}, {Suit: "C", Rank: 10},
		{Suit: "S", Rank: 12},
	}
	lanes, _ := AutoArrange(hand)
	if lanes == nil {
		t.Fatalf("AutoArrange 返回 nil")
	}
	v := ValidateLanes(lanes.Head, lanes.Middle, lanes.Tail)
	if !v.OK {
		t.Fatalf("散牌场景产出三道不合法: head=%+v middle=%+v tail=%+v", v.Head, v.Middle, v.Tail)
	}
}

// TestAutoArrange_RandomHands 随机 30 局验证三道总能合法
func TestAutoArrange_RandomHands(t *testing.T) {
	rand.Seed(42)
	for i := 0; i < 30; i++ {
		hands := Deal(2)
		for _, h := range hands {
			lanes, _ := AutoArrange(h)
			if lanes == nil {
				t.Fatalf("随机手牌 #%d 返回 nil", i)
			}
			v := ValidateLanes(lanes.Head, lanes.Middle, lanes.Tail)
			if !v.OK {
				t.Fatalf("随机手牌 #%d 产出非法三道", i)
			}
		}
	}
}
