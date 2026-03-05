package poker

import "testing"

func TestHandRankings(t *testing.T) {
	tests := []struct {
		name  string
		cards []string
		rank  int
	}{
		{"Royal Flush", []string{"As", "Ks", "Qs", "Js", "Ts", "2d", "3c"}, RoyalFlush},
		{"Straight Flush", []string{"9h", "8h", "7h", "6h", "5h", "2d", "3c"}, StraightFlush},
		{"Four of a Kind", []string{"As", "Ah", "Ad", "Ac", "Ks", "2d", "3c"}, FourOfAKind},
		{"Full House", []string{"As", "Ah", "Ad", "Ks", "Kh", "2d", "3c"}, FullHouse},
		{"Flush", []string{"As", "Ks", "Qs", "Js", "9s", "2d", "3c"}, Flush},
		{"Straight", []string{"9s", "8h", "7d", "6c", "5s", "2d", "3c"}, Straight},
		{"Three of a Kind", []string{"As", "Ah", "Ad", "Ks", "Qh", "2d", "3c"}, ThreeOfAKind},
		{"Two Pair", []string{"As", "Ah", "Ks", "Kh", "Qd", "2d", "3c"}, TwoPair},
		{"One Pair", []string{"As", "Ah", "Ks", "Qh", "Jd", "2d", "3c"}, OnePair},
		{"High Card", []string{"As", "Kh", "Qd", "Jc", "9s", "2d", "3c"}, HighCard},
		{"Ace-low Straight", []string{"As", "2h", "3d", "4c", "5s", "Kd", "Qc"}, Straight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := BestHand(tt.cards)
			if score.Rank != tt.rank {
				t.Errorf("expected rank %d (%s), got %d (%s)", tt.rank, RankName(tt.rank), score.Rank, RankName(score.Rank))
			}
		})
	}
}

func TestHandComparison(t *testing.T) {
	// Pair of aces beats pair of kings
	pairAces := BestHand([]string{"As", "Ah", "Kd", "Qc", "Js", "2d", "3c"})
	pairKings := BestHand([]string{"Ks", "Kh", "Qd", "Jc", "9s", "2d", "3c"})
	if pairAces.Beats(pairKings) <= 0 {
		t.Error("pair of aces should beat pair of kings")
	}

	// Flush beats straight
	flush := BestHand([]string{"As", "Ks", "Qs", "Js", "9s", "2d", "3c"})
	straight := BestHand([]string{"9s", "8h", "7d", "6c", "5s", "2d", "Ac"})
	if flush.Beats(straight) <= 0 {
		t.Error("flush should beat straight")
	}
}

func TestAceLowStraightKicker(t *testing.T) {
	// 6-high straight should beat ace-low (5-high) straight
	sixHigh := BestHand([]string{"6s", "5h", "4d", "3c", "2s", "Kd", "Qc"})
	aceLow := BestHand([]string{"As", "2h", "3d", "4c", "5s", "Kd", "Qc"})
	if sixHigh.Beats(aceLow) <= 0 {
		t.Error("6-high straight should beat ace-low straight")
	}
}
