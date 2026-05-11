// Package game 实现十三张的核心规则
// card.go: 卡牌结构、牌堆构建、洗牌、加色花色归一化
package game

import (
	"math/rand"
	"strings"
)

// Card 表示一张扑克牌
// Suit: S 黑桃 / H 红桃 / D 方块 / C 草花 / D2 加色方块 / C2 加色草花
// Rank: 1..13（A=1, J=11, Q=12, K=13）
type Card struct {
	Suit string `json:"suit"`
	Rank int    `json:"rank"`
}

// SuitsBase 是 4 种基础花色
var SuitsBase = []string{"S", "H", "D", "C"}

// BuildDeck 根据玩家数量生成牌堆
// 4 人及以下：标准 52 张
// 5 人：加一组方块（+13 张，共 65）
// 6 人：再加一组草花（+13 张，共 78）
func BuildDeck(playerCount int) []Card {
	suits := append([]string{}, SuitsBase...)
	if playerCount >= 5 {
		suits = append(suits, "D2")
	}
	if playerCount >= 6 {
		suits = append(suits, "C2")
	}
	cards := make([]Card, 0, len(suits)*13)
	for _, s := range suits {
		for r := 1; r <= 13; r++ {
			cards = append(cards, Card{Suit: s, Rank: r})
		}
	}
	return cards
}

// Shuffle Fisher-Yates 洗牌（原地修改并返回切片）
func Shuffle(arr []Card) []Card {
	for i := len(arr) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		arr[i], arr[j] = arr[j], arr[i]
	}
	return arr
}

// Deal 发牌：每人 13 张，返回 hands[playerCount][13]
func Deal(playerCount int) [][]Card {
	deck := Shuffle(BuildDeck(playerCount))
	hands := make([][]Card, playerCount)
	for i := 0; i < playerCount; i++ {
		hands[i] = deck[i*13 : (i+1)*13]
	}
	return hands
}

// IsMaCard 是否为红桃 5（马牌）
func IsMaCard(c Card) bool {
	return c.Suit == "H" && c.Rank == 5
}

// SameSuit 是否同花色（不区分加色组：D 与 D2 都是方块）
func SameSuit(a, b Card) bool {
	return strings.ReplaceAll(a.Suit, "2", "") == strings.ReplaceAll(b.Suit, "2", "")
}

// NormalSuit 去掉加色后缀的视觉花色
func NormalSuit(c Card) string {
	return strings.ReplaceAll(c.Suit, "2", "")
}

// RankValue 比较两张牌点数：A(1) 默认最大（14）
func RankValue(r int) int {
	if r == 1 {
		return 14
	}
	return r
}

// SameCardSet 比较两组卡牌是否相同（不考虑顺序，按 Suit_Rank 字符串排序）
func SameCardSet(a, b []Card) bool {
	if len(a) != len(b) {
		return false
	}
	ka := make([]string, len(a))
	kb := make([]string, len(b))
	for i, c := range a {
		ka[i] = cardKey(c)
	}
	for i, c := range b {
		kb[i] = cardKey(c)
	}
	sortStrings(ka)
	sortStrings(kb)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}

func cardKey(c Card) string {
	return c.Suit + "_" + itoa(c.Rank)
}

// 简易的整数转字符串（避免引入 strconv 增加依赖层级）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [12]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// sortStrings 简单插入排序（牌组规模小，无需 sort 包）
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1] > s[j] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}
