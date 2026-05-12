// Package game 结算结果对账单元测试
// 关键不变量：每位玩家 LaneScores.Head + Middle + Tail 必须等于 FinalScore，
// 且全场 FinalScore 之和为 0（零和博弈）
package game

import (
	"math/rand"
	"testing"
)

// assertSettleConsistent 校验单次 Settle 的零和性与逐道对账
func assertSettleConsistent(t *testing.T, tag string, res SettleResult) {
	t.Helper()
	total := 0
	for i, p := range res.Players {
		laneSum := p.LaneScores.Head + p.LaneScores.Middle + p.LaneScores.Tail
		if laneSum != p.FinalScore {
			t.Fatalf("%s: 玩家[%d]=%s 三道汇总=%d 与 finalScore=%d 不一致 (head=%d middle=%d tail=%d)",
				tag, i, p.Openid, laneSum, p.FinalScore,
				p.LaneScores.Head, p.LaneScores.Middle, p.LaneScores.Tail)
		}
		total += p.FinalScore
	}
	if total != 0 {
		t.Fatalf("%s: 全场 finalScore 之和 %d != 0（应零和）", tag, total)
	}
}

// 构造 4 人局：每人 13 张固定手牌 + 三道
func makeInputsFixed4P() []SettleInput {
	mk := func(openid string, head, middle, tail []Card) SettleInput {
		hand := append(append(append([]Card{}, head...), middle...), tail...)
		return SettleInput{
			Openid: openid,
			Hand:   hand,
			Lanes:  &Lanes{Head: head, Middle: middle, Tail: tail},
		}
	}
	// P1：头道对子(QJJ)、中道对子(AA1084)、尾道葫芦(KKK55)
	p1 := mk("p1",
		[]Card{{Suit: "S", Rank: 12}, {Suit: "C", Rank: 11}, {Suit: "H", Rank: 11}},
		[]Card{{Suit: "S", Rank: 1}, {Suit: "H", Rank: 1}, {Suit: "C", Rank: 10}, {Suit: "S", Rank: 8}, {Suit: "C", Rank: 4}},
		[]Card{{Suit: "S", Rank: 13}, {Suit: "H", Rank: 13}, {Suit: "C", Rank: 13}, {Suit: "S", Rank: 5}, {Suit: "C", Rank: 5}},
	)
	// P2：头道散牌、中道两对、尾道同花
	p2 := mk("p2",
		[]Card{{Suit: "C", Rank: 11}, {Suit: "S", Rank: 8}, {Suit: "S", Rank: 2}},
		[]Card{{Suit: "S", Rank: 1}, {Suit: "S", Rank: 7}, {Suit: "H", Rank: 7}, {Suit: "S", Rank: 6}, {Suit: "H", Rank: 6}},
		[]Card{{Suit: "D", Rank: 9}, {Suit: "D", Rank: 8}, {Suit: "D", Rank: 5}, {Suit: "D", Rank: 4}, {Suit: "D", Rank: 2}},
	)
	// P3：头道散牌(763)、中道对子(AKJ99)、尾道葫芦(QQ101010)
	p3 := mk("p3",
		[]Card{{Suit: "H", Rank: 7}, {Suit: "C", Rank: 6}, {Suit: "S", Rank: 3}},
		[]Card{{Suit: "H", Rank: 1}, {Suit: "S", Rank: 13}, {Suit: "S", Rank: 11}, {Suit: "C", Rank: 9}, {Suit: "D", Rank: 9}},
		[]Card{{Suit: "H", Rank: 12}, {Suit: "D", Rank: 12}, {Suit: "S", Rank: 10}, {Suit: "C", Rank: 10}, {Suit: "D", Rank: 10}},
	)
	// P4：含红桃 5（马牌），头道对子(Q22)、中道顺子(98765)、尾道葫芦(44333)
	p4 := mk("p4",
		[]Card{{Suit: "S", Rank: 12}, {Suit: "C", Rank: 2}, {Suit: "H", Rank: 2}},
		[]Card{{Suit: "C", Rank: 9}, {Suit: "C", Rank: 8}, {Suit: "C", Rank: 7}, {Suit: "C", Rank: 6}, {Suit: "H", Rank: 5}},
		[]Card{{Suit: "H", Rank: 4}, {Suit: "C", Rank: 4}, {Suit: "S", Rank: 3}, {Suit: "H", Rank: 3}, {Suit: "C", Rank: 3}},
	)
	return []SettleInput{p1, p2, p3, p4}
}

// TestSettle_LaneConsistency_4Players 4 人局含马牌：lane 之和必须等于 finalScore，全场零和
func TestSettle_LaneConsistency_4Players(t *testing.T) {
	inputs := makeInputsFixed4P()
	res := Settle(inputs, true)
	assertSettleConsistent(t, "4 人局含马牌", res)
}

// TestSettle_LaneConsistency_NoMa 同样的牌但关闭马牌
func TestSettle_LaneConsistency_NoMa(t *testing.T) {
	inputs := makeInputsFixed4P()
	res := Settle(inputs, false)
	assertSettleConsistent(t, "4 人局关闭马牌", res)
}

// TestSettle_LaneConsistency_Random 随机 50 次 4 人局，验证逐道对账与零和
func TestSettle_LaneConsistency_Random(t *testing.T) {
	rand.Seed(1)
	for k := 0; k < 50; k++ {
		hands := Deal(4)
		inputs := make([]SettleInput, 4)
		for i, h := range hands {
			lanes, _ := AutoArrange(h)
			if lanes == nil {
				t.Fatalf("随机第 %d 局玩家 %d AutoArrange 返回 nil", k, i)
			}
			inputs[i] = SettleInput{
				Openid: string(rune('A' + i)),
				Hand:   h,
				Lanes:  lanes,
			}
		}
		res := Settle(inputs, k%2 == 0)
		assertSettleConsistent(t, "随机 4 人局", res)
	}
}

// TestSettle_LaneConsistency_RandomMulti 随机 2~5 人局
func TestSettle_LaneConsistency_RandomMulti(t *testing.T) {
	rand.Seed(7)
	for n := 2; n <= 5; n++ {
		for k := 0; k < 20; k++ {
			hands := Deal(n)
			inputs := make([]SettleInput, n)
			for i, h := range hands {
				lanes, _ := AutoArrange(h)
				if lanes == nil {
					t.Fatalf("n=%d 第 %d 局玩家 %d AutoArrange 返回 nil", n, k, i)
				}
				inputs[i] = SettleInput{
					Openid: string(rune('A' + i)),
					Hand:   h,
					Lanes:  lanes,
				}
			}
			res := Settle(inputs, true)
			assertSettleConsistent(t, "随机多人局", res)
		}
	}
}
