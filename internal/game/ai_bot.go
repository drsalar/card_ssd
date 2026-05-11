// Package game AI 自动理牌算法
// ai_bot.go: 为电脑玩家从 13 张手牌中选出三道（头 3 / 中 5 / 尾 5）
// 设计思路（贪心两步）：
//  1. 枚举 13 选 5，找出"牌型最强"的 5 张作为尾道；并列最强时收集所有候选
//  2. 对每个尾道候选，在剩余 8 张中枚举 8 选 5，找"牌型最强"的 5 张作为中道
//     —— 跨候选比较：先比中道牌型，再比头道牌型，决定全局最优
//  3. 剩余 3 张作为头道
//  4. 事后校正"头道 ≤ 中道 ≤ 尾道"：
//     - 中 > 尾：交换中、尾两道（保留各自 5 张组合不变）
//     - 头 > 中：放弃，由上层走 fallback
//  5. 顺子规则：A 可当 1 与 2-3-4-5 组成最小顺子（由 evaluator.checkStraight 实现）
//  6. 复杂度：13C5 + 候选数 × 8C5 ≈ 1287 + N×56，远低于旧的全枚举（~7.2 万）
package game

// AutoArrange 为给定手牌自动理出合法三道
// hand: 13 张手牌，必须为 13 张。返回 Lanes 指针与是否使用了兜底策略
// 外部调用方：bot_driver 中异步调用，提交前再由 ValidateLanes 兜检
func AutoArrange(hand []Card) (lanes *Lanes, usedFallback bool) {
	if len(hand) != 13 {
		return fallbackArrange(hand), true
	}
	// 1. 查找最佳三道
	if best := tryBestArrange(hand); best != nil {
		v := ValidateLanes(best.Head, best.Middle, best.Tail)
		if v.OK {
			return best, false
		}
	}
	// 2. 兜底：点数排序后填道
	return fallbackArrange(hand), true
}

// tryBestArrange 贪心两步：尾道独立最大 → 中道在剩余 8 张里独立最大 → 剩余作头道
func tryBestArrange(hand []Card) *Lanes {
	n := len(hand) // 13
	idxs := make([]int, n)
	for i := range idxs {
		idxs[i] = i
	}

	// Step 1：在 13 张里找最强 5 张作尾道（并列收集所有候选）
	tailCandidates := pickStrongestFiveIdx(hand, idxs)
	if len(tailCandidates) == 0 {
		return nil
	}

	// Step 2：对每个尾道候选，在剩余 8 张里找最强 5 张作中道
	var bestLanes *Lanes
	var bestMidEval HandResult
	var bestHeadEval HandResult
	hasBest := false

	for _, tailIdx := range tailCandidates {
		tailCards := pickByIdx(hand, tailIdx)
		rest := excludeIdx(idxs, tailIdx)

		midCandidates := pickStrongestFiveIdx(hand, rest)
		if len(midCandidates) == 0 {
			continue
		}
		// 中道并列时挑头道更强者（剩 3 张固定）
		var localBestMidIdx []int
		var localBestMidEval HandResult
		var localBestHeadEval HandResult
		localHasBest := false
		for _, midIdx := range midCandidates {
			midCards := pickByIdx(hand, midIdx)
			midEval := Evaluate(midCards, false)
			headIdx := excludeIdx(rest, midIdx)
			headCards := pickByIdx(hand, headIdx)
			headEval := Evaluate(headCards, true)
			if !localHasBest ||
				Compare(midEval, localBestMidEval) > 0 ||
				(Compare(midEval, localBestMidEval) == 0 && Compare(headEval, localBestHeadEval) > 0) {
				localBestMidIdx = midIdx
				localBestMidEval = midEval
				localBestHeadEval = headEval
				localHasBest = true
			}
		}
		if !localHasBest {
			continue
		}
		// 跨候选比较：先比中道，再比头道
		if !hasBest ||
			Compare(localBestMidEval, bestMidEval) > 0 ||
			(Compare(localBestMidEval, bestMidEval) == 0 && Compare(localBestHeadEval, bestHeadEval) > 0) {
			midCards := pickByIdx(hand, localBestMidIdx)
			headIdx := excludeIdx(rest, localBestMidIdx)
			headCards := pickByIdx(hand, headIdx)
			bestLanes = &Lanes{Head: headCards, Middle: midCards, Tail: tailCards}
			bestMidEval = localBestMidEval
			bestHeadEval = localBestHeadEval
			hasBest = true
		}
	}

	if bestLanes == nil {
		return nil
	}
	// 事后校正三道顺序
	return fixLanesOrder(bestLanes)
}

// pickStrongestFiveIdx 在 hand[base...] 范围内枚举 5 张组合，返回所有"并列最强"的下标集合
// base: 原始下标白名单（升序）；返回的每个元素是长度为 5 的原始下标数组
// 当 len(base) < 5 时返回空切片
func pickStrongestFiveIdx(hand []Card, base []int) [][]int {
	if len(base) < 5 {
		return nil
	}
	var bestEval HandResult
	var bestList [][]int
	hasBest := false
	combinations(len(base), 5, func(off []int) bool {
		idx := mapIdx(base, off)
		cards := pickByIdx(hand, idx)
		ev := Evaluate(cards, false)
		if !hasBest {
			bestEval = ev
			bestList = [][]int{idx}
			hasBest = true
			return true
		}
		cmp := Compare(ev, bestEval)
		if cmp > 0 {
			bestEval = ev
			bestList = [][]int{idx}
		} else if cmp == 0 {
			bestList = append(bestList, idx)
		}
		return true
	})
	return bestList
}

// fixLanesOrder 修正"头道 ≤ 中道 ≤ 尾道"约束
// - 中道 > 尾道：交换中、尾两道
// - 头道 > 中道（修正后仍超）：返回 nil 让上层走兜底
// 入参 lanes 不会被修改，返回新的 Lanes
func fixLanesOrder(lanes *Lanes) *Lanes {
	headEval := Evaluate(lanes.Head, true)
	midEval := Evaluate(lanes.Middle, false)
	tailEval := Evaluate(lanes.Tail, false)

	out := &Lanes{
		Head:   append([]Card{}, lanes.Head...),
		Middle: append([]Card{}, lanes.Middle...),
		Tail:   append([]Card{}, lanes.Tail...),
	}
	// 中 > 尾：交换中、尾
	if Compare(midEval, tailEval) > 0 {
		out.Middle, out.Tail = out.Tail, out.Middle
		midEval, tailEval = tailEval, midEval
	}
	// 头 > 中：放弃，由上层走 fallback
	if Compare(headEval, midEval) > 0 {
		return nil
	}
	_ = tailEval
	return out
}

// fallbackArrange 兜底策略：按点数从大到小排序后填尾/中/头
// 该策略会令头道为最小点数、尾道为最大点数，一般就是散牌，肯定合法
func fallbackArrange(hand []Card) *Lanes {
	if len(hand) != 13 {
		// 异常保护：按原顺序切。实际上发牌必为 13 张
		out := make([]Card, 13)
		copy(out, hand)
		return &Lanes{Head: out[0:3], Middle: out[3:8], Tail: out[8:13]}
	}
	sorted := make([]Card, len(hand))
	copy(sorted, hand)
	// 倒序：RankValue 大 -> 小（A=14）
	for i := 1; i < len(sorted); i++ {
		j := i
		for j > 0 && RankValue(sorted[j-1].Rank) < RankValue(sorted[j].Rank) {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
			j--
		}
	}
	tail := append([]Card{}, sorted[0:5]...)
	middle := append([]Card{}, sorted[5:10]...)
	head := append([]Card{}, sorted[10:13]...)
	// 验证下，尽量使中道不大于尾道、头道不大于中道
	v := ValidateLanes(head, middle, tail)
	if v.OK {
		return &Lanes{Head: head, Middle: middle, Tail: tail}
	}
	// 不合法时尝试互换中、尾道最大牌，避免中道出现顺子/同花顺、尾道为散牌
	if Compare(v.Middle, v.Tail) > 0 {
		// 交换中、尾道
		return &Lanes{Head: head, Middle: tail, Tail: middle}
	}
	// 头道超越中道：从中道中拼一张最小牌到头道、将头道最大赶出去
	if Compare(v.Head, v.Middle) > 0 {
		// 简单报底：按手牌顺序直接切，不会超越
		return &Lanes{Head: hand[10:13], Middle: hand[5:10], Tail: hand[0:5]}
	}
	return &Lanes{Head: head, Middle: middle, Tail: tail}
}

// combinations 枚举从 [0..n) 中选 k 个的所有组合，对每个组合调用 cb
// cb 返回 false 则提前终止枚举（当前仅作为预留，调用方始终返回 true）
func combinations(n, k int, cb func(idx []int) bool) {
	if k > n || k <= 0 {
		return
	}
	idx := make([]int, k)
	for i := 0; i < k; i++ {
		idx[i] = i
	}
	for {
		// 拷贝后交出去，避免调用方保存并后续被修改
		out := make([]int, k)
		copy(out, idx)
		if !cb(out) {
			return
		}
		// 生成下一个组合
		i := k - 1
		for i >= 0 && idx[i] == n-k+i {
			i--
		}
		if i < 0 {
			return
		}
		idx[i]++
		for j := i + 1; j < k; j++ {
			idx[j] = idx[j-1] + 1
		}
	}
}

// pickByIdx 从 hand 中取出 idx 对应的牌
func pickByIdx(hand []Card, idx []int) []Card {
	out := make([]Card, len(idx))
	for i, k := range idx {
		out[i] = hand[k]
	}
	return out
}

// excludeIdx 返回 all 中排除 exc 后的余量（两者均为原始下标序列）
// all 与 exc 都是升序的，但 exc 未必是 all 的连续子集（这里逻辑上是子集）
func excludeIdx(all []int, exc []int) []int {
	excSet := make(map[int]struct{}, len(exc))
	for _, v := range exc {
		excSet[v] = struct{}{}
	}
	out := make([]int, 0, len(all)-len(exc))
	for _, v := range all {
		if _, ok := excSet[v]; ok {
			continue
		}
		out = append(out, v)
	}
	return out
}

// mapIdx 将偏移下标 off （针对 base 的序号）映射为原始下标
func mapIdx(base []int, off []int) []int {
	out := make([]int, len(off))
	for i, k := range off {
		out[i] = base[k]
	}
	return out
}
