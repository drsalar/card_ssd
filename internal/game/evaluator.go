// Package game 牌型识别与比较
// evaluator.go: 牌型识别与比较（评估、排序、加色同花带对规则）
package game

// 牌型枚举（数字越大牌型越大）
const (
	TypeHigh          = 1  // 散牌（德州中的高牌：仅有单张，未组成任何其他牌型）
	TypePair          = 2  // 对子
	TypeTwoPair       = 3  // 两对
	TypeThree         = 4  // 三条
	TypeStraight      = 5  // 顺子
	TypeFlush         = 6  // 同花
	TypeFull          = 7  // 葫芦
	TypeFour          = 8  // 炸弹
	TypeStraightFlush = 9  // 同花顺
	TypeFive          = 10 // 五龙
)

// TypeName 牌型名称
var TypeName = map[int]string{
	1:  "散牌",
	2:  "对子",
	3:  "两对",
	4:  "三条",
	5:  "顺子",
	6:  "同花",
	7:  "葫芦",
	8:  "炸弹",
	9:  "同花顺",
	10: "五龙",
}

// HandResult 牌型评估结果
// Ranks: 用于比牌的关键点数数组（从大到小）
// ExtraPairs: 同花上对子组数（加色规则用）
type HandResult struct {
	Type       int    `json:"type"`
	Name       string `json:"name"`
	Ranks      []int  `json:"ranks"`
	ExtraPairs int    `json:"-"`
}

// Evaluate 评估一组牌的牌型
// isHead: 是否头道（仅 3 张，仅可能为散牌/对子/三条）
func Evaluate(cards []Card, isHead bool) HandResult {
	if isHead {
		return evaluateHead(cards)
	}
	if len(cards) != 5 {
		return HandResult{Type: TypeHigh, Name: TypeName[TypeHigh], Ranks: []int{}}
	}
	groups := groupByRank(cards)
	counts := groupCountsDesc(groups)

	// 五龙
	if counts[0] == 5 {
		var r int
		for k := range groups {
			r = k
		}
		return HandResult{Type: TypeFive, Name: TypeName[TypeFive], Ranks: []int{RankValue(r)}}
	}

	flush := isFlush(cards)
	straightTop, hasStraight := checkStraight(cards)

	// 同花顺
	if flush && hasStraight {
		return HandResult{Type: TypeStraightFlush, Name: TypeName[TypeStraightFlush], Ranks: []int{straightTop}}
	}
	// 炸弹（4 张同点 + 单张 kicker）
	if counts[0] == 4 {
		var four, kicker int
		for r, c := range groups {
			if c == 4 {
				four = r
			} else {
				kicker = r
			}
		}
		return HandResult{
			Type:  TypeFour,
			Name:  TypeName[TypeFour],
			Ranks: []int{RankValue(four), RankValue(kicker)},
		}
	}
	// 葫芦（3+2）
	if counts[0] == 3 && len(counts) > 1 && counts[1] == 2 {
		var three, pair int
		for r, c := range groups {
			if c == 3 {
				three = r
			} else if c == 2 {
				pair = r
			}
		}
		return HandResult{
			Type:  TypeFull,
			Name:  TypeName[TypeFull],
			Ranks: []int{RankValue(three), RankValue(pair)},
		}
	}
	// 同花（含加色规则的对子计数）
	if flush {
		ranks := make([]int, 0, 5)
		for _, c := range cards {
			ranks = append(ranks, RankValue(c.Rank))
		}
		sortIntsDesc(ranks)
		return HandResult{
			Type:       TypeFlush,
			Name:       TypeName[TypeFlush],
			Ranks:      ranks,
			ExtraPairs: countPairs(groups),
		}
	}
	// 顺子
	if hasStraight {
		return HandResult{Type: TypeStraight, Name: TypeName[TypeStraight], Ranks: []int{straightTop}}
	}
	// 三条（中尾道）
	if counts[0] == 3 {
		var three int
		kickers := make([]int, 0, 2)
		for r, c := range groups {
			if c == 3 {
				three = r
			} else {
				kickers = append(kickers, r)
			}
		}
		sortByRankDesc(kickers)
		ranks := []int{RankValue(three)}
		for _, k := range kickers {
			ranks = append(ranks, RankValue(k))
		}
		return HandResult{Type: TypeThree, Name: TypeName[TypeThree], Ranks: ranks}
	}
	// 两对
	if counts[0] == 2 && len(counts) > 1 && counts[1] == 2 {
		pairs := make([]int, 0, 2)
		var kicker int
		for r, c := range groups {
			if c == 2 {
				pairs = append(pairs, r)
			} else {
				kicker = r
			}
		}
		sortByRankDesc(pairs)
		return HandResult{
			Type:  TypeTwoPair,
			Name:  TypeName[TypeTwoPair],
			Ranks: []int{RankValue(pairs[0]), RankValue(pairs[1]), RankValue(kicker)},
		}
	}
	// 对子
	if counts[0] == 2 {
		var pair int
		kickers := make([]int, 0, 3)
		for r, c := range groups {
			if c == 2 {
				pair = r
			} else {
				kickers = append(kickers, r)
			}
		}
		sortByRankDesc(kickers)
		ranks := []int{RankValue(pair)}
		for _, k := range kickers {
			ranks = append(ranks, RankValue(k))
		}
		return HandResult{Type: TypePair, Name: TypeName[TypePair], Ranks: ranks}
	}
	// 高牌
	ranks := make([]int, 0, 5)
	for _, c := range cards {
		ranks = append(ranks, RankValue(c.Rank))
	}
	sortIntsDesc(ranks)
	return HandResult{Type: TypeHigh, Name: TypeName[TypeHigh], Ranks: ranks}
}

// evaluateHead 头道（3 张）评估
func evaluateHead(cards []Card) HandResult {
	if len(cards) != 3 {
		return HandResult{Type: TypeHigh, Name: TypeName[TypeHigh], Ranks: []int{}}
	}
	groups := groupByRank(cards)
	counts := groupCountsDesc(groups)
	if counts[0] == 3 {
		var r int
		for k := range groups {
			r = k
		}
		return HandResult{Type: TypeThree, Name: TypeName[TypeThree], Ranks: []int{RankValue(r)}}
	}
	if counts[0] == 2 {
		var pair, kicker int
		for r, c := range groups {
			if c == 2 {
				pair = r
			} else {
				kicker = r
			}
		}
		return HandResult{
			Type:  TypePair,
			Name:  TypeName[TypePair],
			Ranks: []int{RankValue(pair), RankValue(kicker)},
		}
	}
	ranks := make([]int, 0, 3)
	for _, c := range cards {
		ranks = append(ranks, RankValue(c.Rank))
	}
	sortIntsDesc(ranks)
	return HandResult{Type: TypeHigh, Name: TypeName[TypeHigh], Ranks: ranks}
}

// groupByRank 按 rank 分组 → rank → 张数
func groupByRank(cards []Card) map[int]int {
	m := make(map[int]int, 5)
	for _, c := range cards {
		m[c.Rank]++
	}
	return m
}

// groupCountsDesc 各 rank 分组张数从大到小排序
func groupCountsDesc(m map[int]int) []int {
	out := make([]int, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sortIntsDesc(out)
	return out
}

// isFlush 5 张是否同花
func isFlush(cards []Card) bool {
	if len(cards) != 5 {
		return false
	}
	s0 := NormalSuit(cards[0])
	for _, c := range cards[1:] {
		if NormalSuit(c) != s0 {
			return false
		}
	}
	return true
}

// checkStraight 是否顺子
// 返回 (top, ok)：top 为顶点
// 1-2-3-4-5 → top=5；10-J-Q-K-A → top=14；普通 → top=最大点
func checkStraight(cards []Card) (int, bool) {
	if len(cards) != 5 {
		return 0, false
	}
	ranks := make([]int, 5)
	for i, c := range cards {
		ranks[i] = c.Rank
	}
	sortIntsAsc(ranks)
	for i := 1; i < 5; i++ {
		if ranks[i] == ranks[i-1] {
			return 0, false
		}
	}
	// 普通顺子：差值正好为 4
	if ranks[4]-ranks[0] == 4 {
		if ranks[4] == 1 {
			return 14, true
		}
		return ranks[4], true
	}
	// 10-J-Q-K-A: 排序后 = [1,10,11,12,13]
	if ranks[0] == 1 && ranks[1] == 10 && ranks[2] == 11 && ranks[3] == 12 && ranks[4] == 13 {
		return 14, true
	}
	// 1-2-3-4-5
	if ranks[0] == 1 && ranks[1] == 2 && ranks[2] == 3 && ranks[3] == 4 && ranks[4] == 5 {
		return 5, true
	}
	return 0, false
}

// countPairs 数对子组数（同花带对加色规则）
func countPairs(groups map[int]int) int {
	n := 0
	for _, c := range groups {
		if c >= 2 {
			n++
		}
	}
	return n
}

// Compare 比较两个评估结果
// 返回 1: a > b ; -1: a < b ; 0: 相等
func Compare(a, b HandResult) int {
	if a.Type != b.Type {
		if a.Type > b.Type {
			return 1
		}
		return -1
	}
	// 同花特殊：带对同花 > 普通同花；2 对同花 > 1 对同花
	if a.Type == TypeFlush {
		if a.ExtraPairs != b.ExtraPairs {
			if a.ExtraPairs > b.ExtraPairs {
				return 1
			}
			return -1
		}
	}
	la, lb := a.Ranks, b.Ranks
	n := len(la)
	if len(lb) > n {
		n = len(lb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(la) {
			va = la[i]
		}
		if i < len(lb) {
			vb = lb[i]
		}
		if va != vb {
			if va > vb {
				return 1
			}
			return -1
		}
	}
	return 0
}

// sortIntsDesc 整数从大到小（插入排序，规模小）
func sortIntsDesc(s []int) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1] < s[j] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}

// sortIntsAsc 整数从小到大
func sortIntsAsc(s []int) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1] > s[j] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}

// sortByRankDesc 按 RankValue（A=14）从大到小排序
func sortByRankDesc(s []int) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && RankValue(s[j-1]) < RankValue(s[j]) {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}
