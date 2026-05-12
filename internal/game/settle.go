// Package game 比牌结算引擎
// settle.go: 比牌结算（特殊加分、打枪、本垒打、马牌倍率）
// 输入：每位玩家的三道，输出每位玩家详细积分明细 + 比牌过程信息
package game

// 中道特殊加分（输方支付）
var midBonus = map[int]int{
	TypeFull:          1, // 葫芦 +1
	TypeFour:          3, // 炸弹 +3
	TypeStraightFlush: 4, // 同花顺 +4
	TypeFive:          9, // 五龙 +9
}

// 尾道特殊加分（输方支付）
var tailBonus = map[int]int{
	TypeFour:          3,
	TypeStraightFlush: 4,
	TypeFive:          9,
}

// HeadThreeBonus 冲三（头道三条）加分
const HeadThreeBonus = 2

// SettleInput 结算输入：每位玩家的三道及参与状态
type SettleInput struct {
	Openid string
	Hand   []Card
	Lanes  *Lanes
}

// Lanes 三道
type Lanes struct {
	Head   []Card `json:"head"`
	Middle []Card `json:"middle"`
	Tail   []Card `json:"tail"`
}

// LaneCmp 单道对比结果
type LaneCmp struct {
	Cmp    int `json:"cmp"`
	ScoreI int `json:"scoreI"`
	ScoreJ int `json:"scoreJ"`
}

// ExtraScore 特殊加分对比结果
type ExtraScore struct {
	ScoreI int `json:"scoreI"`
	ScoreJ int `json:"scoreJ"`
	BonusI int `json:"bonusI"`
	BonusJ int `json:"bonusJ"`
}

// PairResult 两位玩家比对结果
type PairResult struct {
	I      int        `json:"i"`
	J      int        `json:"j"`
	Head   LaneCmp    `json:"head"`
	Middle LaneCmp    `json:"middle"`
	Tail   LaneCmp    `json:"tail"`
	Extra  ExtraScore `json:"extra"`
	GunI   bool       `json:"gunI"`
	GunJ   bool       `json:"gunJ"`
	ScoreI int        `json:"scoreI"`
	ScoreJ int        `json:"scoreJ"`
}

// HandTypes 三道牌型
type HandTypes struct {
	Head   HandResult `json:"head"`
	Middle HandResult `json:"middle"`
	Tail   HandResult `json:"tail"`
}

// LaneScores 单玩家逐道得分汇总
// 各道得分已包含：本道牌型特殊加分（冲三 / 中道 / 尾道）、打枪 ×2、本垒打 ×2、马牌 ×2 等所有倍率。
// 满足 Head + Middle + Tail == FinalScore，便于前端面板分项展示与总分对账。
// Extra 字段保留为 0 以兼容旧客户端，不再单独累计。
type LaneScores struct {
	Head   int `json:"head"`
	Middle int `json:"middle"`
	Tail   int `json:"tail"`
	Extra  int `json:"extra"`
}

// PlayerSettleResult 单玩家结算结果
type PlayerSettleResult struct {
	Openid     string     `json:"openid"`
	Lanes      *Lanes     `json:"lanes"`
	HandTypes  *HandTypes `json:"handTypes"`
	HasMa      bool       `json:"hasMa"`
	BaseScore  int        `json:"baseScore"`
	FinalScore int        `json:"finalScore"`
	LaneScores LaneScores `json:"laneScores"`
}

// SettleResult 整局结算结果
type SettleResult struct {
	Players  []PlayerSettleResult `json:"players"`
	Pairs    []PairResult         `json:"pairs"`
	Homeruns []string             `json:"homeruns"`
}

// playerEval 玩家三道评估缓存
type playerEval struct {
	openid string
	head   HandResult
	middle HandResult
	tail   HandResult
	hasMa  bool
}

// Settle 结算一局
// withMa: 是否启用马牌（红桃 5 双倍）
func Settle(players []SettleInput, withMa bool) SettleResult {
	n := len(players)
	evals := make([]*playerEval, n)
	for i, p := range players {
		if p.Lanes == nil {
			evals[i] = nil
			continue
		}
		evals[i] = &playerEval{
			openid: p.Openid,
			head:   Evaluate(p.Lanes.Head, true),
			middle: Evaluate(p.Lanes.Middle, false),
			tail:   Evaluate(p.Lanes.Tail, false),
			hasMa:  withMa && hasMa(p),
		}
	}

	baseScores := make([]int, n)
	pairs := make([]PairResult, 0, n*(n-1)/2)

	// 两两比较：先收集全部 pair，但不累计 LaneScores（待本垒打/马牌倍率确定后再统一累计）
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			ei, ej := evals[i], evals[j]
			if ei == nil || ej == nil {
				continue
			}
			result := comparePair(i, j, ei, ej)
			baseScores[i] += result.ScoreI
			baseScores[j] += result.ScoreJ
			pairs = append(pairs, result)
		}
	}

	// 本垒打：n>=3，且某玩家对所有对手都打枪
	homerunOpenids := make([]string, 0)
	homerunIdx := make(map[int]bool)
	if n >= 3 {
		for i := 0; i < n; i++ {
			if evals[i] == nil {
				continue
			}
			cnt := 0
			allGun := true
			for _, p := range pairs {
				if p.I != i && p.J != i {
					continue
				}
				cnt++
				if p.I == i {
					if !p.GunI {
						allGun = false
						break
					}
				} else {
					if !p.GunJ {
						allGun = false
						break
					}
				}
			}
			if allGun && cnt == n-1 {
				homerunOpenids = append(homerunOpenids, evals[i].openid)
				homerunIdx[i] = true
			}
		}
	}

	// 统一累计：按本垒打 ×2、马牌 ×2 累乘后再累加到 LaneScores 与 finalScores
	// 关键约束：head + middle + tail == finalScore，便于结算面板分项展示与总分自洽
	laneScoresPerPlayer := make([]LaneScores, n)
	finalScores := make([]int, n)
	for _, p := range pairs {
		mul := 1
		if homerunIdx[p.I] || homerunIdx[p.J] {
			mul = 2
		}
		// 马牌倍率：拥有红桃 5 的玩家在每道每一项上额外 ×2（与打枪 / 本垒打可累乘）
		mulI := mul
		mulJ := mul
		if withMa {
			if evals[p.I] != nil && evals[p.I].hasMa {
				mulI *= 2
			}
			if evals[p.J] != nil && evals[p.J].hasMa {
				mulJ *= 2
			}
		}
		hI := p.Head.ScoreI * mulI
		mI := p.Middle.ScoreI * mulI
		tI := p.Tail.ScoreI * mulI
		hJ := p.Head.ScoreJ * mulJ
		mJ := p.Middle.ScoreJ * mulJ
		tJ := p.Tail.ScoreJ * mulJ
		laneScoresPerPlayer[p.I].Head += hI
		laneScoresPerPlayer[p.I].Middle += mI
		laneScoresPerPlayer[p.I].Tail += tI
		laneScoresPerPlayer[p.J].Head += hJ
		laneScoresPerPlayer[p.J].Middle += mJ
		laneScoresPerPlayer[p.J].Tail += tJ
		finalScores[p.I] += hI + mI + tI
		finalScores[p.J] += hJ + mJ + tJ
	}

	// 组装结果
	out := SettleResult{
		Players:  make([]PlayerSettleResult, n),
		Pairs:    pairs,
		Homeruns: homerunOpenids,
	}
	for i, p := range players {
		var ht *HandTypes
		hasMaFlag := false
		if evals[i] != nil {
			ht = &HandTypes{
				Head:   evals[i].head,
				Middle: evals[i].middle,
				Tail:   evals[i].tail,
			}
			hasMaFlag = evals[i].hasMa
		}
		out.Players[i] = PlayerSettleResult{
			Openid:     p.Openid,
			Lanes:      p.Lanes,
			HandTypes:  ht,
			HasMa:      hasMaFlag,
			BaseScore:  baseScores[i],
			FinalScore: finalScores[i],
			LaneScores: laneScoresPerPlayer[i],
		}
	}
	return out
}

// comparePair 比较两位玩家的三道
// 关键约束：返回的 Head/Middle/Tail 的 ScoreI/ScoreJ 已经包含本道牌型特殊加分（冲三 / 中道 / 尾道）
// 与对方在同一道上的特殊加分扣减；如发生打枪，三道分别 ×2。
// 这样 head+middle+tail == ScoreI（pair 总分），便于上层按道累计后与总分自洽。
func comparePair(i, j int, ei, ej *playerEval) PairResult {
	headCmp := Compare(ei.head, ej.head)
	midCmp := Compare(ei.middle, ej.middle)
	tailCmp := Compare(ei.tail, ej.tail)
	head := laneScore(headCmp)
	middle := laneScore(midCmp)
	tail := laneScore(tailCmp)

	// 各道牌型特殊加分（输方支付）
	// 头道：冲三
	headBonusI, headBonusJ := 0, 0
	if ei.head.Type == TypeThree && headCmp >= 0 {
		headBonusI += HeadThreeBonus
	}
	if ej.head.Type == TypeThree && headCmp <= 0 {
		headBonusJ += HeadThreeBonus
	}
	// 中道：葫芦/炸弹/同花顺/五龙
	midBonusI, midBonusJ := 0, 0
	if v, ok := midBonus[ei.middle.Type]; ok && midCmp >= 0 {
		midBonusI += v
	}
	if v, ok := midBonus[ej.middle.Type]; ok && midCmp <= 0 {
		midBonusJ += v
	}
	// 尾道：炸弹/同花顺/五龙
	tailBonusI, tailBonusJ := 0, 0
	if v, ok := tailBonus[ei.tail.Type]; ok && tailCmp >= 0 {
		tailBonusI += v
	}
	if v, ok := tailBonus[ej.tail.Type]; ok && tailCmp <= 0 {
		tailBonusJ += v
	}

	// 将本道 bonus 合并进各道 ScoreI/ScoreJ：一道净分 = 胜负分 + 我方该道 bonus - 对方该道 bonus
	head.ScoreI += headBonusI - headBonusJ
	head.ScoreJ += headBonusJ - headBonusI
	middle.ScoreI += midBonusI - midBonusJ
	middle.ScoreJ += midBonusJ - midBonusI
	tail.ScoreI += tailBonusI - tailBonusJ
	tail.ScoreJ += tailBonusJ - tailBonusI

	// 打枪：i 三道全胜 j → 整体加倍（每道独立 ×2，保证逐道之和仍等于 pair 总分）
	gunI := headCmp > 0 && midCmp > 0 && tailCmp > 0
	gunJ := headCmp < 0 && midCmp < 0 && tailCmp < 0
	if gunI || gunJ {
		head.ScoreI *= 2
		head.ScoreJ *= 2
		middle.ScoreI *= 2
		middle.ScoreJ *= 2
		tail.ScoreI *= 2
		tail.ScoreJ *= 2
	}

	scoreI := head.ScoreI + middle.ScoreI + tail.ScoreI
	scoreJ := head.ScoreJ + middle.ScoreJ + tail.ScoreJ

	extraI := headBonusI + midBonusI + tailBonusI
	extraJ := headBonusJ + midBonusJ + tailBonusJ
	return PairResult{
		I:      i,
		J:      j,
		Head:   head,
		Middle: middle,
		Tail:   tail,
		Extra: ExtraScore{
			ScoreI: extraI - extraJ,
			ScoreJ: extraJ - extraI,
			BonusI: extraI,
			BonusJ: extraJ,
		},
		GunI:   gunI,
		GunJ:   gunJ,
		ScoreI: scoreI,
		ScoreJ: scoreJ,
	}
}

func laneScore(cmp int) LaneCmp {
	if cmp > 0 {
		return LaneCmp{Cmp: cmp, ScoreI: 1, ScoreJ: -1}
	}
	if cmp < 0 {
		return LaneCmp{Cmp: cmp, ScoreI: -1, ScoreJ: 1}
	}
	return LaneCmp{Cmp: cmp}
}

// hasMa 玩家是否持有红桃 5
func hasMa(p SettleInput) bool {
	if len(p.Hand) > 0 {
		for _, c := range p.Hand {
			if IsMaCard(c) {
				return true
			}
		}
	}
	if p.Lanes != nil {
		for _, c := range p.Lanes.Head {
			if IsMaCard(c) {
				return true
			}
		}
		for _, c := range p.Lanes.Middle {
			if IsMaCard(c) {
				return true
			}
		}
		for _, c := range p.Lanes.Tail {
			if IsMaCard(c) {
				return true
			}
		}
	}
	return false
}
