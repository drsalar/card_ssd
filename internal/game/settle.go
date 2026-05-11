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
	laneScoresPerPlayer := make([]LaneScores, n)
	pairs := make([]PairResult, 0, n*(n-1)/2)

	// 两两比较
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			ei, ej := evals[i], evals[j]
			if ei == nil || ej == nil {
				continue
			}
			result := comparePair(i, j, ei, ej)
			baseScores[i] += result.ScoreI
			baseScores[j] += result.ScoreJ
			laneScoresPerPlayer[i].Head += result.Head.ScoreI
			laneScoresPerPlayer[i].Middle += result.Middle.ScoreI
			laneScoresPerPlayer[i].Tail += result.Tail.ScoreI
			laneScoresPerPlayer[i].Extra += result.Extra.ScoreI
			laneScoresPerPlayer[j].Head += result.Head.ScoreJ
			laneScoresPerPlayer[j].Middle += result.Middle.ScoreJ
			laneScoresPerPlayer[j].Tail += result.Tail.ScoreJ
			laneScoresPerPlayer[j].Extra += result.Extra.ScoreJ
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

	// 重新计算 final scores 应用本垒打倍率：与 i 相关的所有 pair 再×2
	finalScores := make([]int, n)
	for _, p := range pairs {
		mul := 1
		if homerunIdx[p.I] || homerunIdx[p.J] {
			mul = 2
		}
		finalScores[p.I] += p.ScoreI * mul
		finalScores[p.J] += p.ScoreJ * mul
	}

	// 马牌倍率：拥有红桃 5 的玩家最终分数 ×2
	if withMa {
		for i := 0; i < n; i++ {
			if evals[i] != nil && evals[i].hasMa {
				finalScores[i] *= 2
			}
		}
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
func comparePair(i, j int, ei, ej *playerEval) PairResult {
	headCmp := Compare(ei.head, ej.head)
	midCmp := Compare(ei.middle, ej.middle)
	tailCmp := Compare(ei.tail, ej.tail)
	head := laneScore(headCmp)
	middle := laneScore(midCmp)
	tail := laneScore(tailCmp)

	// 特殊加分（输方支付）
	extraI, extraJ := 0, 0
	// 冲三：头道三条
	if ei.head.Type == TypeThree && headCmp >= 0 {
		extraI += HeadThreeBonus
	}
	if ej.head.Type == TypeThree && headCmp <= 0 {
		extraJ += HeadThreeBonus
	}
	// 中道
	if v, ok := midBonus[ei.middle.Type]; ok && midCmp >= 0 {
		extraI += v
	}
	if v, ok := midBonus[ej.middle.Type]; ok && midCmp <= 0 {
		extraJ += v
	}
	// 尾道
	if v, ok := tailBonus[ei.tail.Type]; ok && tailCmp >= 0 {
		extraI += v
	}
	if v, ok := tailBonus[ej.tail.Type]; ok && tailCmp <= 0 {
		extraJ += v
	}

	// 打枪：i 三道全胜 j → i 整体加倍
	gunI := headCmp > 0 && midCmp > 0 && tailCmp > 0
	gunJ := headCmp < 0 && midCmp < 0 && tailCmp < 0

	scoreI := head.ScoreI + middle.ScoreI + tail.ScoreI + extraI - extraJ
	scoreJ := head.ScoreJ + middle.ScoreJ + tail.ScoreJ + extraJ - extraI
	if gunI || gunJ {
		scoreI *= 2
		scoreJ *= 2
	}
	return PairResult{
		I:      i,
		J:      j,
		Head:   LaneCmp{Cmp: headCmp, ScoreI: head.ScoreI, ScoreJ: head.ScoreJ},
		Middle: LaneCmp{Cmp: midCmp, ScoreI: middle.ScoreI, ScoreJ: middle.ScoreJ},
		Tail:   LaneCmp{Cmp: tailCmp, ScoreI: tail.ScoreI, ScoreJ: tail.ScoreJ},
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
		return LaneCmp{ScoreI: 1, ScoreJ: -1}
	}
	if cmp < 0 {
		return LaneCmp{ScoreI: -1, ScoreJ: 1}
	}
	return LaneCmp{}
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
