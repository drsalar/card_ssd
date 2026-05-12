// Package game evaluator_straight_test.go
// 验证 A-2-3-4-5 顺子的特殊大小关系：
//   - 仅小于 10-J-Q-K-A
//   - 大于其他所有顺子（包括 9-10-J-Q-K）
package game

import "testing"

// makeStraight 构造 5 张顺子牌（指定每张点数，全部用黑桃，避免被识别为同花顺；A 用 1 表示）
func makeStraight(ranks ...int) []Card {
	out := make([]Card, 0, len(ranks))
	// 用不同花色避免触发同花/同花顺
	suits := []string{"S", "H", "D", "C", "S"}
	for i, r := range ranks {
		out = append(out, Card{Suit: suits[i], Rank: r})
	}
	return out
}

func TestStraight_A2345_vs_Others(t *testing.T) {
	a2345 := Evaluate(makeStraight(1, 2, 3, 4, 5), false)
	if a2345.Type != TypeStraight {
		t.Fatalf("A2345 应识别为顺子，实际 type=%d", a2345.Type)
	}

	cases := []struct {
		name   string
		ranks  []int
		expect int // 期望 A2345 与之比较的结果（1: A2345 > x；-1: A2345 < x）
	}{
		{"23456", []int{2, 3, 4, 5, 6}, 1},
		{"34567", []int{3, 4, 5, 6, 7}, 1},
		{"56789", []int{5, 6, 7, 8, 9}, 1},
		{"9-10-J-Q-K", []int{9, 10, 11, 12, 13}, 1}, // 同 top=13，A2345 仍应大
		{"10-J-Q-K-A", []int{1, 10, 11, 12, 13}, -1},
	}
	for _, c := range cases {
		other := Evaluate(makeStraight(c.ranks...), false)
		if other.Type != TypeStraight {
			t.Fatalf("%s 未识别为顺子", c.name)
		}
		got := Compare(a2345, other)
		if got != c.expect {
			t.Fatalf("A2345 vs %s 期望 %d, 实际 %d (a.ranks=%v b.ranks=%v)",
				c.name, c.expect, got, a2345.Ranks, other.Ranks)
		}
	}
}

func TestStraight_A2345_Equal_Self(t *testing.T) {
	a := Evaluate(makeStraight(1, 2, 3, 4, 5), false)
	b := Evaluate(makeStraight(1, 2, 3, 4, 5), false)
	if Compare(a, b) != 0 {
		t.Fatalf("A2345 自身比较应相等")
	}
}
