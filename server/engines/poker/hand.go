package poker

import "sort"

// Hand rankings (higher = better)
const (
	HighCard      = 1
	OnePair       = 2
	TwoPair       = 3
	ThreeOfAKind  = 4
	Straight      = 5
	Flush         = 6
	FullHouse     = 7
	FourOfAKind   = 8
	StraightFlush = 9
	RoyalFlush    = 10
)

var rankNames = map[int]string{
	HighCard:      "High Card",
	OnePair:       "One Pair",
	TwoPair:       "Two Pair",
	ThreeOfAKind:  "Three of a Kind",
	Straight:      "Straight",
	Flush:         "Flush",
	FullHouse:     "Full House",
	FourOfAKind:   "Four of a Kind",
	StraightFlush: "Straight Flush",
	RoyalFlush:    "Royal Flush",
}

func RankName(rank int) string {
	if n, ok := rankNames[rank]; ok {
		return n
	}
	return "Unknown"
}

// HandScore represents a comparable hand score.
// Compare by Rank first, then Kickers lexicographically.
type HandScore struct {
	Rank    int
	Kickers [5]int // values used for tiebreaking, highest first
}

func (a HandScore) Beats(b HandScore) int {
	if a.Rank != b.Rank {
		if a.Rank > b.Rank {
			return 1
		}
		return -1
	}
	for i := range 5 {
		if a.Kickers[i] != b.Kickers[i] {
			if a.Kickers[i] > b.Kickers[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func cardValue(c string) int {
	switch c[0] {
	case '2':
		return 2
	case '3':
		return 3
	case '4':
		return 4
	case '5':
		return 5
	case '6':
		return 6
	case '7':
		return 7
	case '8':
		return 8
	case '9':
		return 9
	case 'T':
		return 10
	case 'J':
		return 11
	case 'Q':
		return 12
	case 'K':
		return 13
	case 'A':
		return 14
	}
	return 0
}

func cardSuit(c string) byte {
	return c[1]
}

// BestHand evaluates the best 5-card hand from 7 cards (2 hole + 5 community).
func BestHand(cards []string) HandScore {
	best := HandScore{}
	n := len(cards)
	// Generate all C(n,5) combinations
	for i := 0; i < n-4; i++ {
		for j := i + 1; j < n-3; j++ {
			for k := j + 1; k < n-2; k++ {
				for l := k + 1; l < n-1; l++ {
					for m := l + 1; m < n; m++ {
						hand := [5]string{cards[i], cards[j], cards[k], cards[l], cards[m]}
						score := evaluateFive(hand)
						if score.Beats(best) > 0 {
							best = score
						}
					}
				}
			}
		}
	}
	return best
}

func evaluateFive(hand [5]string) HandScore {
	values := make([]int, 5)
	suits := make([]byte, 5)
	for i, c := range hand {
		values[i] = cardValue(c)
		suits[i] = cardSuit(c)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(values)))

	isFlush := suits[0] == suits[1] && suits[1] == suits[2] && suits[2] == suits[3] && suits[3] == suits[4]

	isStraight := false
	straightHigh := 0
	// Normal straight
	if values[0]-values[4] == 4 && allUnique(values) {
		isStraight = true
		straightHigh = values[0]
	}
	// Ace-low straight (A-2-3-4-5): values sorted desc = [14, 5, 4, 3, 2]
	if values[0] == 14 && values[1] == 5 && values[2] == 4 && values[3] == 3 && values[4] == 2 {
		isStraight = true
		straightHigh = 5 // 5-high straight
	}

	if isStraight && isFlush {
		if straightHigh == 14 {
			return HandScore{Rank: RoyalFlush, Kickers: [5]int{14, 13, 12, 11, 10}}
		}
		return HandScore{Rank: StraightFlush, Kickers: [5]int{straightHigh}}
	}

	// Count value frequencies
	freq := map[int]int{}
	for _, v := range values {
		freq[v]++
	}

	var quads, trips []int
	var pairs []int
	var singles []int
	for v, c := range freq {
		switch c {
		case 4:
			quads = append(quads, v)
		case 3:
			trips = append(trips, v)
		case 2:
			pairs = append(pairs, v)
		case 1:
			singles = append(singles, v)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(quads)))
	sort.Sort(sort.Reverse(sort.IntSlice(trips)))
	sort.Sort(sort.Reverse(sort.IntSlice(pairs)))
	sort.Sort(sort.Reverse(sort.IntSlice(singles)))

	if len(quads) == 1 {
		kicker := singles[0]
		if len(pairs) > 0 && pairs[0] > kicker {
			kicker = pairs[0]
		}
		if len(trips) > 0 && trips[0] > kicker {
			kicker = trips[0]
		}
		return HandScore{Rank: FourOfAKind, Kickers: [5]int{quads[0], kicker}}
	}

	if len(trips) == 1 && len(pairs) >= 1 {
		return HandScore{Rank: FullHouse, Kickers: [5]int{trips[0], pairs[0]}}
	}

	if isFlush {
		return HandScore{Rank: Flush, Kickers: [5]int{values[0], values[1], values[2], values[3], values[4]}}
	}

	if isStraight {
		return HandScore{Rank: Straight, Kickers: [5]int{straightHigh}}
	}

	if len(trips) == 1 {
		return HandScore{Rank: ThreeOfAKind, Kickers: [5]int{trips[0], singles[0], singles[1]}}
	}

	if len(pairs) == 2 {
		return HandScore{Rank: TwoPair, Kickers: [5]int{pairs[0], pairs[1], singles[0]}}
	}

	if len(pairs) == 1 {
		return HandScore{Rank: OnePair, Kickers: [5]int{pairs[0], singles[0], singles[1], singles[2]}}
	}

	return HandScore{Rank: HighCard, Kickers: [5]int{values[0], values[1], values[2], values[3], values[4]}}
}

func allUnique(vals []int) bool {
	seen := map[int]bool{}
	for _, v := range vals {
		if seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
