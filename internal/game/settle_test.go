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

// TestSettle_MaPair_WidensMultiplier 验证「马牌按 pair 整体加倍」：
// pair 中只要任意一方持有马牌（红桃 5），双方在该 pair 的所有得分都同步 ×2，
// 与打枪 / 本垒打可累乘。这是图中 g25i 总分应为 +20 的核心规则。
//
// 构造场景（2 人局，pB 三道全胜 → 同时构成打枪与本垒打）：
//
//	pA：头道 S2 S3 S4；中道散牌；尾道同花
//	pB（带马牌 H5）：头道 SK SQ SJ；中道同花含 H5；尾道同花
//	pB 三道全胜 → 打枪 ×2 + 本垒打 ×2 + 马牌 ×2 = 总倍率 ×8。
//
// 三道净分（不含特殊 bonus）= 1+1+1 = 3，乘以 8 = 24。
// 期望：pB = +24，pA = -24。
func TestSettle_MaPair_WidensMultiplier(t *testing.T) {
	mk := func(openid string, head, middle, tail []Card) SettleInput {
		hand := append(append(append([]Card{}, head...), middle...), tail...)
		return SettleInput{
			Openid: openid,
			Hand:   hand,
			Lanes:  &Lanes{Head: head, Middle: middle, Tail: tail},
		}
	}
	pA := mk("A",
		[]Card{{Suit: "S", Rank: 2}, {Suit: "S", Rank: 3}, {Suit: "S", Rank: 4}},
		[]Card{{Suit: "C", Rank: 2}, {Suit: "C", Rank: 3}, {Suit: "C", Rank: 4}, {Suit: "S", Rank: 7}, {Suit: "S", Rank: 8}},
		[]Card{{Suit: "D", Rank: 2}, {Suit: "D", Rank: 3}, {Suit: "D", Rank: 6}, {Suit: "D", Rank: 7}, {Suit: "D", Rank: 9}}, // 同花
	)
	pB := mk("B",
		[]Card{{Suit: "S", Rank: 13}, {Suit: "S", Rank: 12}, {Suit: "S", Rank: 11}},
		[]Card{{Suit: "H", Rank: 13}, {Suit: "H", Rank: 12}, {Suit: "H", Rank: 11}, {Suit: "H", Rank: 10}, {Suit: "H", Rank: 5}}, // 含马牌 H5，且为同花
		[]Card{{Suit: "D", Rank: 13}, {Suit: "D", Rank: 12}, {Suit: "D", Rank: 11}, {Suit: "D", Rank: 10}, {Suit: "D", Rank: 8}}, // 同花
	)

	res := Settle([]SettleInput{pA, pB}, true)
	assertSettleConsistent(t, "马牌 pair 整体加倍", res)

	// pB 三道全胜 + 持马 → 2 人局打枪 ×2、本垒打 ×2、马牌 ×2，总倍率 ×8
	// 三道净分（不含特殊 bonus）= 1+1+1 = 3，乘以 8 = 24
	if res.Players[1].FinalScore != 24 || res.Players[0].FinalScore != -24 {
		t.Fatalf("马牌 pair 整体加倍：期望 pB=+24 / pA=-24，实际 pB=%d / pA=%d",
			res.Players[1].FinalScore, res.Players[0].FinalScore)
	}
	// 同步校验本垒打字段
	if len(res.Homeruns) != 1 || res.Homeruns[0] != "B" {
		t.Fatalf("马牌 pair 整体加倍：期望 Homeruns=[B]，实际 %v", res.Homeruns)
	}
}

// mk2P 2 人局测试构造工具
func mk2P(openid string, head, middle, tail []Card) SettleInput {
	hand := append(append(append([]Card{}, head...), middle...), tail...)
	return SettleInput{
		Openid: openid,
		Hand:   hand,
		Lanes:  &Lanes{Head: head, Middle: middle, Tail: tail},
	}
}

// TestSettle_2P_Homerun_Triggered 用例 A：2 人对局 pA 三道全胜 → 触发本垒打
// 构造：均为散牌（无任何特殊 bonus），pA 三道严格大于 pB
//
//	pA：头 SA SK SQ；中 CA CK CQ CJ S9；尾 DA DK DQ DJ D8
//	pB：头 H5 H4 H3；中 D5 D4 D3 D2 H6；尾 C7 C6 C5 C4 H2
//
// 关闭马牌：仅打枪 ×2 + 本垒打 ×2 = 总倍率 ×4
// 三道净分 = 1+1+1 = 3 → 总分 = 3×4 = 12
func TestSettle_2P_Homerun_Triggered(t *testing.T) {
	pA := mk2P("A",
		[]Card{{Suit: "S", Rank: 1}, {Suit: "S", Rank: 13}, {Suit: "S", Rank: 12}},
		[]Card{{Suit: "C", Rank: 1}, {Suit: "C", Rank: 13}, {Suit: "C", Rank: 12}, {Suit: "C", Rank: 11}, {Suit: "S", Rank: 9}},
		[]Card{{Suit: "D", Rank: 1}, {Suit: "D", Rank: 13}, {Suit: "D", Rank: 12}, {Suit: "D", Rank: 11}, {Suit: "D", Rank: 8}}, // 同花
	)
	pB := mk2P("B",
		[]Card{{Suit: "H", Rank: 5}, {Suit: "H", Rank: 4}, {Suit: "H", Rank: 3}},
		[]Card{{Suit: "D", Rank: 5}, {Suit: "D", Rank: 4}, {Suit: "D", Rank: 3}, {Suit: "D", Rank: 2}, {Suit: "H", Rank: 6}},
		[]Card{{Suit: "C", Rank: 7}, {Suit: "C", Rank: 6}, {Suit: "C", Rank: 5}, {Suit: "C", Rank: 4}, {Suit: "H", Rank: 2}}, // 同花
	)

	res := Settle([]SettleInput{pA, pB}, false)
	assertSettleConsistent(t, "2 人本垒打触发", res)

	if len(res.Homeruns) != 1 || res.Homeruns[0] != "A" {
		t.Fatalf("2 人本垒打触发：期望 Homeruns=[A]，实际 %v", res.Homeruns)
	}
	// pA 同花×2 道、pB 同花×1 道 → 尾道双方同花，按比大小（A 大于 7）pA 胜
	// 中道：pA 一对 A vs pB 一对 5 → pA 胜（无 bonus，因均非中道特殊牌型）
	// 头道：pA 散牌 AKQ vs pB 散牌 543 → pA 胜
	// 三道净分 1+1+1=3，乘以 4（打枪 ×2 + 本垒打 ×2）= 12
	if res.Players[0].FinalScore != 12 || res.Players[1].FinalScore != -12 {
		t.Fatalf("2 人本垒打触发：期望 pA=+12 / pB=-12，实际 pA=%d / pB=%d",
			res.Players[0].FinalScore, res.Players[1].FinalScore)
	}
}

// TestSettle_2P_NoHomerun 用例 B：2 人对局未触发本垒打
// 构造：pA 头道 + 尾道胜，中道输 → 不构成全胜，无本垒打、无打枪
func TestSettle_2P_NoHomerun(t *testing.T) {
	pA := mk2P("A",
		[]Card{{Suit: "S", Rank: 1}, {Suit: "S", Rank: 13}, {Suit: "S", Rank: 12}},                                             // AKQ 散
		[]Card{{Suit: "C", Rank: 2}, {Suit: "D", Rank: 3}, {Suit: "S", Rank: 4}, {Suit: "C", Rank: 5}, {Suit: "S", Rank: 7}},   // 散
		[]Card{{Suit: "D", Rank: 1}, {Suit: "C", Rank: 13}, {Suit: "S", Rank: 11}, {Suit: "D", Rank: 9}, {Suit: "D", Rank: 8}}, // 散，A 高
	)
	pB := mk2P("B",
		[]Card{{Suit: "H", Rank: 11}, {Suit: "H", Rank: 10}, {Suit: "H", Rank: 9}},                                              // JT9 散，输给 AKQ
		[]Card{{Suit: "D", Rank: 6}, {Suit: "D", Rank: 8}, {Suit: "C", Rank: 10}, {Suit: "C", Rank: 11}, {Suit: "S", Rank: 13}}, // 散，K 高 → 胜中道
		[]Card{{Suit: "C", Rank: 6}, {Suit: "C", Rank: 4}, {Suit: "S", Rank: 5}, {Suit: "H", Rank: 7}, {Suit: "H", Rank: 2}},    // 散，7 高 → 输尾道
	)

	res := Settle([]SettleInput{pA, pB}, false)
	assertSettleConsistent(t, "2 人无本垒打", res)

	if len(res.Homeruns) != 0 {
		t.Fatalf("2 人无本垒打：期望 Homeruns=[]，实际 %v", res.Homeruns)
	}
	// 头道 +1、中道 -1、尾道 +1，无打枪/本垒打/马牌倍率 → pA 净分 +1
	if res.Players[0].FinalScore != 1 || res.Players[1].FinalScore != -1 {
		t.Fatalf("2 人无本垒打：期望 pA=+1 / pB=-1，实际 pA=%d / pB=%d",
			res.Players[0].FinalScore, res.Players[1].FinalScore)
	}
}

// TestSettle_2P_NilLanes 用例 C：2 人对局其中一方 Lanes==nil（弃局/未提交）
// 期望：不进入 pair 比较；Homeruns 为空；双方 FinalScore 均为 0
func TestSettle_2P_NilLanes(t *testing.T) {
	pA := mk2P("A",
		[]Card{{Suit: "S", Rank: 1}, {Suit: "S", Rank: 13}, {Suit: "S", Rank: 12}},
		[]Card{{Suit: "C", Rank: 1}, {Suit: "C", Rank: 13}, {Suit: "C", Rank: 12}, {Suit: "C", Rank: 11}, {Suit: "S", Rank: 9}},
		[]Card{{Suit: "D", Rank: 1}, {Suit: "D", Rank: 13}, {Suit: "D", Rank: 12}, {Suit: "D", Rank: 11}, {Suit: "D", Rank: 8}},
	)
	pB := SettleInput{Openid: "B", Lanes: nil}

	res := Settle([]SettleInput{pA, pB}, false)
	assertSettleConsistent(t, "2 人 Lanes==nil", res)

	if len(res.Homeruns) != 0 {
		t.Fatalf("2 人 Lanes==nil：期望 Homeruns=[]，实际 %v", res.Homeruns)
	}
	if res.Players[0].FinalScore != 0 || res.Players[1].FinalScore != 0 {
		t.Fatalf("2 人 Lanes==nil：期望双方 0/0，实际 pA=%d / pB=%d",
			res.Players[0].FinalScore, res.Players[1].FinalScore)
	}
}
