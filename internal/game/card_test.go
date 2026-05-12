// Package game BuildDeck/Deal 单元测试：覆盖 2~6 人按实际人数动态加色与发牌
package game

import "testing"

// TestBuildDeckBelowFive 4 人及以下使用标准 52 张
func TestBuildDeckBelowFive(t *testing.T) {
	for _, n := range []int{2, 3, 4} {
		deck := BuildDeck(n)
		if len(deck) != 52 {
			t.Errorf("BuildDeck(%d) 期望 52 张，实际 %d", n, len(deck))
		}
		// 不应包含加色花色
		for _, c := range deck {
			if c.Suit == "D2" || c.Suit == "C2" {
				t.Errorf("BuildDeck(%d) 不应包含加色花色，发现 %s", n, c.Suit)
			}
		}
	}
}

// TestBuildDeckFive 5 人加一组方块（D2），共 65 张
func TestBuildDeckFive(t *testing.T) {
	deck := BuildDeck(5)
	if len(deck) != 65 {
		t.Errorf("BuildDeck(5) 期望 65 张，实际 %d", len(deck))
	}
	hasD2, hasC2 := false, false
	for _, c := range deck {
		if c.Suit == "D2" {
			hasD2 = true
		}
		if c.Suit == "C2" {
			hasC2 = true
		}
	}
	if !hasD2 {
		t.Errorf("BuildDeck(5) 应包含加色方块 D2")
	}
	if hasC2 {
		t.Errorf("BuildDeck(5) 不应包含加色草花 C2")
	}
}

// TestBuildDeckSix 6 人再加一组草花（C2），共 78 张
func TestBuildDeckSix(t *testing.T) {
	deck := BuildDeck(6)
	if len(deck) != 78 {
		t.Errorf("BuildDeck(6) 期望 78 张，实际 %d", len(deck))
	}
	hasD2, hasC2 := false, false
	for _, c := range deck {
		if c.Suit == "D2" {
			hasD2 = true
		}
		if c.Suit == "C2" {
			hasC2 = true
		}
	}
	if !hasD2 || !hasC2 {
		t.Errorf("BuildDeck(6) 应同时包含 D2 与 C2，hasD2=%v hasC2=%v", hasD2, hasC2)
	}
}

// TestDealAllPlayers 每人均 13 张
func TestDealAllPlayers(t *testing.T) {
	for _, n := range []int{2, 3, 4, 5, 6} {
		hands := Deal(n)
		if len(hands) != n {
			t.Fatalf("Deal(%d) 应返回 %d 个 hand，实际 %d", n, n, len(hands))
		}
		for i, h := range hands {
			if len(h) != 13 {
				t.Errorf("Deal(%d) 第 %d 个玩家手牌应为 13 张，实际 %d", n, i, len(h))
			}
		}
	}
}
