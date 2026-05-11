// Package game 三道合法性校验
// validator.go: 三道大小校验（头道 ≤ 中道 ≤ 尾道）
package game

// LaneValidateResult 三道合法性结果
type LaneValidateResult struct {
	OK     bool               `json:"ok"`
	Errors LaneValidateErrors `json:"errors"`
	Head   HandResult         `json:"head"`
	Middle HandResult         `json:"middle"`
	Tail   HandResult         `json:"tail"`
}

// LaneValidateErrors 各道是否合法（true=合法）
type LaneValidateErrors struct {
	Head   bool `json:"head"`
	Middle bool `json:"middle"`
	Tail   bool `json:"tail"`
}

// ValidateLanes 校验三道总张数与「头道 ≤ 中道 ≤ 尾道」
func ValidateLanes(headCards, middleCards, tailCards []Card) LaneValidateResult {
	errors := LaneValidateErrors{Head: true, Middle: true, Tail: true}
	if len(headCards) != 3 {
		errors.Head = false
	}
	if len(middleCards) != 5 {
		errors.Middle = false
	}
	if len(tailCards) != 5 {
		errors.Tail = false
	}
	if !errors.Head || !errors.Middle || !errors.Tail {
		return LaneValidateResult{OK: false, Errors: errors}
	}
	h := Evaluate(headCards, true)
	m := Evaluate(middleCards, false)
	t := Evaluate(tailCards, false)
	// 头道 > 中道 → 非法
	if Compare(h, m) > 0 {
		errors.Head = false
		errors.Middle = false
	}
	// 中道 > 尾道 → 非法
	if Compare(m, t) > 0 {
		errors.Middle = false
		errors.Tail = false
	}
	ok := errors.Head && errors.Middle && errors.Tail
	return LaneValidateResult{OK: ok, Errors: errors, Head: h, Middle: m, Tail: t}
}
