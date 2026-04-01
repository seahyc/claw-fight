package poker

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/claw-fight/server/engines"
)

const (
	smallBlind   = 10
	bigBlind     = 20
	startChips   = 1000
	maxHands     = 50
	phasePreflop = "preflop"
	phaseFlop    = "flop"
	phaseTurn    = "turn"
	phaseRiver   = "river"
	phaseShowdown = "showdown"
	phaseFinished = "finished"
)

type PokerEngine struct{}

func New() *PokerEngine {
	return &PokerEngine{}
}

func (e *PokerEngine) Name() string       { return "poker" }
func (e *PokerEngine) MinPlayers() int     { return 2 }
func (e *PokerEngine) MaxPlayers() int     { return 2 }

func (e *PokerEngine) DescribeRules() string {
	return "Heads-up Texas Hold'em Poker. 2 players, 1000 starting chips, 10/20 blinds. " +
		"Play up to 50 hands. Standard poker hand rankings. Actions: check, call, bet, raise, fold, all_in."
}

func (e *PokerEngine) InitGame(players []engines.PlayerID, options map[string]any) (*engines.GameState, error) {
	if len(players) != 2 {
		return nil, fmt.Errorf("poker requires exactly 2 players")
	}

	state := &engines.GameState{
		Players: players,
		Data:    map[string]any{},
	}

	chips := map[string]int{
		string(players[0]): startChips,
		string(players[1]): startChips,
	}
	state.Data["chips"] = chips
	state.Data["dealer"] = 0
	state.Data["hand_number"] = 0

	startNewHand(state)
	return state, nil
}

func startNewHand(state *engines.GameState) {
	handNum := engines.GetInt(state.Data, "hand_number") + 1
	state.Data["hand_number"] = handNum

	dealer := engines.GetInt(state.Data, "dealer")
	chips := getChips(state)

	// Shuffle deck
	deck := shuffleDeck()
	state.Data["deck"] = deck

	// Deal hole cards
	hands := map[string][]string{
		string(state.Players[0]): {deck[0], deck[1]},
		string(state.Players[1]): {deck[2], deck[3]},
	}
	state.Data["hands"] = hands
	state.Data["deck_pos"] = 4
	state.Data["community"] = []string{}

	// Post blinds
	sbPlayer := dealer        // In heads-up, dealer is SB
	bbPlayer := 1 - dealer

	sbID := string(state.Players[sbPlayer])
	bbID := string(state.Players[bbPlayer])

	sbAmount := smallBlind
	bbAmount := bigBlind
	if chips[sbID] < sbAmount {
		sbAmount = chips[sbID]
	}
	if chips[bbID] < bbAmount {
		bbAmount = chips[bbID]
	}

	chips[sbID] -= sbAmount
	chips[bbID] -= bbAmount

	pot := sbAmount + bbAmount
	state.Data["chips"] = chips
	state.Data["pot"] = pot
	state.Data["current_bet"] = bbAmount
	state.Data["player_bets"] = map[string]int{
		sbID: sbAmount,
		bbID: bbAmount,
	}
	state.Data["betting_round"] = phasePreflop
	state.Data["last_raiser"] = ""
	state.Data["acted_this_round"] = map[string]bool{}
	state.Data["all_in_players"] = map[string]bool{}

	// Check if either player is all-in from blinds
	allIn := engines.GetStringBoolMap(state.Data, "all_in_players")
	if chips[sbID] == 0 {
		allIn[sbID] = true
	}
	if chips[bbID] == 0 {
		allIn[bbID] = true
	}
	state.Data["all_in_players"] = allIn

	// Pre-flop: dealer/SB acts first in heads-up
	state.Phase = phasePreflop
	state.CurrentTurn = state.Players[sbPlayer]

	// If SB is all-in from posting blind, skip to BB
	if allIn[sbID] {
		acted := engines.GetStringBoolMap(state.Data, "acted_this_round")
		acted[sbID] = true
		state.Data["acted_this_round"] = acted
		state.CurrentTurn = state.Players[bbPlayer]
		// If both all-in, run out the board
		if allIn[bbID] {
			runOutBoard(state)
			return
		}
	}
}

func (e *PokerEngine) ValidateAction(state *engines.GameState, player engines.PlayerID, action engines.Action) error {
	if state.Phase == phaseFinished || state.Phase == phaseShowdown {
		return fmt.Errorf("hand is not in a betting phase")
	}
	if state.CurrentTurn != player {
		return fmt.Errorf("not your turn")
	}

	allIn := engines.GetStringBoolMap(state.Data, "all_in_players")
	if allIn[string(player)] {
		return fmt.Errorf("you are all-in, no actions available")
	}

	chips := getChips(state)
	playerChips := chips[string(player)]
	currentBet := engines.GetInt(state.Data, "current_bet")
	playerBets := engines.GetStringIntMap(state.Data, "player_bets")
	toCall := currentBet - playerBets[string(player)]

	switch action.Type {
	case "fold":
		return nil
	case "check":
		if toCall > 0 {
			return fmt.Errorf("cannot check, must call %d or fold", toCall)
		}
		return nil
	case "call":
		if toCall == 0 {
			return fmt.Errorf("nothing to call, use check instead")
		}
		return nil
	case "bet":
		if toCall > 0 {
			return fmt.Errorf("cannot bet when there is a bet to call, use raise")
		}
		amount, err := engines.GetActionAmount(action)
		if err != nil {
			return err
		}
		if amount < bigBlind && amount < playerChips {
			return fmt.Errorf("minimum bet is %d", bigBlind)
		}
		if amount > playerChips {
			return fmt.Errorf("cannot bet more than your chips (%d)", playerChips)
		}
		return nil
	case "raise":
		if toCall == 0 {
			return fmt.Errorf("nothing to raise, use bet instead")
		}
		amount, err := engines.GetActionAmount(action)
		if err != nil {
			return err
		}
		// amount is the total raise-to amount (player's total bet this round)
		totalBet := amount
		raiseSize := totalBet - currentBet
		if raiseSize < bigBlind && (totalBet-playerBets[string(player)]) < playerChips {
			return fmt.Errorf("minimum raise is %d (to %d)", bigBlind, currentBet+bigBlind)
		}
		needed := totalBet - playerBets[string(player)]
		if needed > playerChips {
			return fmt.Errorf("cannot raise to %d, you only have %d chips", totalBet, playerChips)
		}
		return nil
	case "all_in":
		return nil
	default:
		return fmt.Errorf("unknown action: %s", action.Type)
	}
}

func (e *PokerEngine) ApplyAction(state *engines.GameState, player engines.PlayerID, action engines.Action) (*engines.ActionResult, error) {
	if err := e.ValidateAction(state, player, action); err != nil {
		return &engines.ActionResult{Success: false, Message: err.Error()}, err
	}

	chips := getChips(state)
	playerChips := chips[string(player)]
	currentBet := engines.GetInt(state.Data, "current_bet")
	playerBets := engines.GetStringIntMap(state.Data, "player_bets")
	pot := engines.GetInt(state.Data, "pot")
	acted := engines.GetStringBoolMap(state.Data, "acted_this_round")
	allIn := engines.GetStringBoolMap(state.Data, "all_in_players")
	pid := string(player)
	toCall := currentBet - playerBets[pid]

	result := &engines.ActionResult{Success: true, Data: map[string]any{}}

	switch action.Type {
	case "fold":
		result.Message = fmt.Sprintf("%s folds", pid)
		state.Data["folded"] = pid
		// Award pot to other player
		other := engines.OtherPlayer(state, player)
		chips[string(other)] += pot
		state.Data["pot"] = 0
		state.Data["chips"] = chips
		finishHand(state)
		return result, nil

	case "check":
		result.Message = fmt.Sprintf("%s checks", pid)
		acted[pid] = true

	case "call":
		callAmount := min(toCall, playerChips)
		chips[pid] -= callAmount
		playerBets[pid] += callAmount
		pot += callAmount
		result.Message = fmt.Sprintf("%s calls %d", pid, callAmount)
		result.Data["amount"] = callAmount
		acted[pid] = true
		if chips[pid] == 0 {
			allIn[pid] = true
		}

	case "bet":
		amount, _ := engines.GetActionAmount(action)
		if amount > playerChips {
			amount = playerChips
		}
		chips[pid] -= amount
		playerBets[pid] += amount
		pot += amount
		state.Data["current_bet"] = playerBets[pid]
		state.Data["last_raiser"] = pid
		result.Message = fmt.Sprintf("%s bets %d", pid, amount)
		result.Data["amount"] = amount
		// Reset acted for other player
		for _, p := range state.Players {
			if string(p) != pid {
				acted[string(p)] = false
			}
		}
		acted[pid] = true
		if chips[pid] == 0 {
			allIn[pid] = true
		}

	case "raise":
		amount, _ := engines.GetActionAmount(action) // total raise-to
		needed := min(amount-playerBets[pid], playerChips)
		chips[pid] -= needed
		playerBets[pid] += needed
		pot += needed
		state.Data["current_bet"] = playerBets[pid]
		state.Data["last_raiser"] = pid
		result.Message = fmt.Sprintf("%s raises to %d", pid, playerBets[pid])
		result.Data["amount"] = needed
		for _, p := range state.Players {
			if string(p) != pid {
				acted[string(p)] = false
			}
		}
		acted[pid] = true
		if chips[pid] == 0 {
			allIn[pid] = true
		}

	case "all_in":
		amount := playerChips
		chips[pid] -= amount
		playerBets[pid] += amount
		pot += amount
		if playerBets[pid] > currentBet {
			state.Data["current_bet"] = playerBets[pid]
			state.Data["last_raiser"] = pid
			for _, p := range state.Players {
				if string(p) != pid {
					acted[string(p)] = false
				}
			}
		}
		allIn[pid] = true
		acted[pid] = true
		result.Message = fmt.Sprintf("%s goes all-in for %d", pid, amount)
		result.Data["amount"] = amount
	}

	state.Data["chips"] = chips
	state.Data["player_bets"] = playerBets
	state.Data["pot"] = pot
	state.Data["acted_this_round"] = acted
	state.Data["all_in_players"] = allIn

	state.TurnNumber++

	// Check if betting round is over
	advanceBetting(state)

	return result, nil
}

func advanceBetting(state *engines.GameState) {
	acted := engines.GetStringBoolMap(state.Data, "acted_this_round")
	allIn := engines.GetStringBoolMap(state.Data, "all_in_players")

	// Check if all non-all-in players have acted
	allActed := true
	for _, p := range state.Players {
		pid := string(p)
		if !allIn[pid] && !acted[pid] {
			allActed = false
			// Next turn goes to this player
			state.CurrentTurn = p
			break
		}
	}

	if !allActed {
		// Find next player to act
		dealer := engines.GetInt(state.Data, "dealer")
		round := engines.GetString(state.Data, "betting_round")
		var first int
		if round == phasePreflop {
			first = dealer // SB/dealer acts first preflop in heads-up
		} else {
			first = 1 - dealer // Non-dealer acts first post-flop
		}
		for i := range 2 {
			idx := (first + i) % 2
			pid := string(state.Players[idx])
			if !acted[pid] && !allIn[pid] {
				state.CurrentTurn = state.Players[idx]
				return
			}
		}
		return
	}

	// Both all-in - run out remaining community cards
	if allIn[string(state.Players[0])] && allIn[string(state.Players[1])] {
		runOutBoard(state)
		return
	}

	// All active players have acted - advance to next round
	advanceToNextRound(state)
}

func advanceToNextRound(state *engines.GameState) {
	// Reset betting state for new round
	state.Data["current_bet"] = 0
	state.Data["player_bets"] = map[string]int{
		string(state.Players[0]): 0,
		string(state.Players[1]): 0,
	}
	state.Data["acted_this_round"] = map[string]bool{}
	state.Data["last_raiser"] = ""

	dealer := engines.GetInt(state.Data, "dealer")
	allIn := engines.GetStringBoolMap(state.Data, "all_in_players")

	round := engines.GetString(state.Data, "betting_round")
	switch round {
	case phasePreflop:
		dealCommunity(state, 3) // flop
		state.Data["betting_round"] = phaseFlop
		state.Phase = phaseFlop
	case phaseFlop:
		dealCommunity(state, 1) // turn
		state.Data["betting_round"] = phaseTurn
		state.Phase = phaseTurn
	case phaseTurn:
		dealCommunity(state, 1) // river
		state.Data["betting_round"] = phaseRiver
		state.Phase = phaseRiver
	case phaseRiver:
		resolveShowdown(state)
		return
	}

	// If both players are all-in after dealing, run out the rest
	if allIn[string(state.Players[0])] && allIn[string(state.Players[1])] {
		runOutBoard(state)
		return
	}

	// Non-dealer acts first post-flop
	first := 1 - dealer
	for i := range 2 {
		idx := (first + i) % 2
		pid := string(state.Players[idx])
		if !allIn[pid] {
			state.CurrentTurn = state.Players[idx]
			return
		}
	}
}

func runOutBoard(state *engines.GameState) {
	round := engines.GetString(state.Data, "betting_round")
	switch round {
	case phasePreflop:
		dealCommunity(state, 3)
		dealCommunity(state, 1)
		dealCommunity(state, 1)
	case phaseFlop:
		dealCommunity(state, 1)
		dealCommunity(state, 1)
	case phaseTurn:
		dealCommunity(state, 1)
	}
	resolveShowdown(state)
}

func dealCommunity(state *engines.GameState, count int) {
	deck := engines.GetStringSlice(state.Data, "deck")
	pos := engines.GetInt(state.Data, "deck_pos")
	community := engines.GetStringSlice(state.Data, "community")

	for range count {
		community = append(community, deck[pos])
		pos++
	}
	state.Data["community"] = community
	state.Data["deck_pos"] = pos
}

func resolveShowdown(state *engines.GameState) {
	state.Phase = phaseShowdown
	state.Data["betting_round"] = phaseShowdown

	hands := getHands(state)
	community := engines.GetStringSlice(state.Data, "community")
	chips := getChips(state)
	pot := engines.GetInt(state.Data, "pot")

	p0 := string(state.Players[0])
	p1 := string(state.Players[1])

	cards0 := append([]string{}, hands[p0]...)
	cards0 = append(cards0, community...)
	cards1 := append([]string{}, hands[p1]...)
	cards1 = append(cards1, community...)

	score0 := BestHand(cards0)
	score1 := BestHand(cards1)

	cmp := score0.Beats(score1)

	state.Data["showdown_result"] = map[string]any{
		"hands": map[string]any{
			p0: map[string]any{"cards": hands[p0], "rank": score0.Rank, "rank_name": RankName(score0.Rank)},
			p1: map[string]any{"cards": hands[p1], "rank": score1.Rank, "rank_name": RankName(score1.Rank)},
		},
	}

	if cmp > 0 {
		chips[p0] += pot
		state.Data["hand_winner"] = p0
	} else if cmp < 0 {
		chips[p1] += pot
		state.Data["hand_winner"] = p1
	} else {
		// Split pot
		half := pot / 2
		chips[p0] += half
		chips[p1] += pot - half
		state.Data["hand_winner"] = "split"
	}
	state.Data["chips"] = chips
	state.Data["pot"] = 0

	finishHand(state)
}

func finishHand(state *engines.GameState) {
	chips := getChips(state)
	handNum := engines.GetInt(state.Data, "hand_number")

	p0 := string(state.Players[0])
	p1 := string(state.Players[1])

	// Check if game is over
	if chips[p0] <= 0 || chips[p1] <= 0 || handNum >= maxHands {
		state.Phase = phaseFinished
		return
	}

	// Rotate dealer and start next hand
	dealer := engines.GetInt(state.Data, "dealer")
	state.Data["dealer"] = 1 - dealer

	// Clean up hand-specific data
	delete(state.Data, "folded")
	delete(state.Data, "showdown_result")
	delete(state.Data, "hand_winner")

	startNewHand(state)
}

func (e *PokerEngine) GetPlayerView(state *engines.GameState, player engines.PlayerID) *engines.PlayerView {
	pid := string(player)
	chips := getChips(state)
	hands := getHands(state)
	community := engines.GetStringSlice(state.Data, "community")
	pot := engines.GetInt(state.Data, "pot")
	currentBet := engines.GetInt(state.Data, "current_bet")
	playerBets := engines.GetStringIntMap(state.Data, "player_bets")
	dealer := engines.GetInt(state.Data, "dealer")
	handNum := engines.GetInt(state.Data, "hand_number")
	allIn := engines.GetStringBoolMap(state.Data, "all_in_players")

	otherPID := string(engines.OtherPlayer(state, player))

	isMyTurn := state.CurrentTurn == player && state.Phase != phaseFinished && state.Phase != phaseShowdown

	// Determine available actions
	var actions []string
	if isMyTurn && !allIn[pid] {
		toCall := currentBet - playerBets[pid]
		if toCall > 0 {
			actions = append(actions, "call", "raise", "fold", "all_in")
		} else {
			actions = append(actions, "check", "bet", "fold", "all_in")
		}
	}

	// Build board view
	board := map[string]any{
		"community": community,
		"pot":       pot,
		"your_chips":     chips[pid],
		"opponent_chips": chips[otherPID],
		"your_cards":     hands[pid],
		"current_bet":    currentBet,
		"your_bet":       playerBets[pid],
		"opponent_bet":   playerBets[otherPID],
	}

	// Show opponent cards at showdown
	showdownResult, _ := state.Data["showdown_result"].(map[string]any)
	if showdownResult != nil {
		board["opponent_cards"] = hands[otherPID]
		board["showdown"] = showdownResult
	}

	gameSpecific := map[string]any{
		"hand_number": handNum,
		"is_dealer":   state.Players[dealer] == player,
		"your_all_in":     allIn[pid],
		"opponent_all_in": allIn[otherPID],
	}

	if w, ok := state.Data["hand_winner"]; ok {
		gameSpecific["hand_winner"] = w
	}
	if f, ok := state.Data["folded"]; ok {
		gameSpecific["folded"] = f
	}

	return &engines.PlayerView{
		Phase:            state.Phase,
		YourTurn:         isMyTurn,
		Board:            board,
		AvailableActions: actions,
		TurnNumber:       state.TurnNumber,
		GameSpecific:     gameSpecific,
	}
}

func (e *PokerEngine) CheckGameOver(state *engines.GameState) *engines.GameResult {
	if state.Phase != phaseFinished {
		return nil
	}

	chips := getChips(state)
	p0 := string(state.Players[0])
	p1 := string(state.Players[1])

	scores := map[engines.PlayerID]int{
		state.Players[0]: chips[p0],
		state.Players[1]: chips[p1],
	}

	if chips[p0] <= 0 {
		return &engines.GameResult{
			Finished: true,
			Winner:   state.Players[1],
			Scores:   scores,
			Reason:   fmt.Sprintf("%s eliminated", p0),
		}
	}
	if chips[p1] <= 0 {
		return &engines.GameResult{
			Finished: true,
			Winner:   state.Players[0],
			Scores:   scores,
			Reason:   fmt.Sprintf("%s eliminated", p1),
		}
	}

	// 50 hands played
	if chips[p0] > chips[p1] {
		return &engines.GameResult{
			Finished: true,
			Winner:   state.Players[0],
			Scores:   scores,
			Reason:   fmt.Sprintf("50 hands played, %s wins with %d chips", p0, chips[p0]),
		}
	}
	if chips[p1] > chips[p0] {
		return &engines.GameResult{
			Finished: true,
			Winner:   state.Players[1],
			Scores:   scores,
			Reason:   fmt.Sprintf("50 hands played, %s wins with %d chips", p1, chips[p1]),
		}
	}

	return &engines.GameResult{
		Finished: true,
		Draw:     true,
		Scores:   scores,
		Reason:   "50 hands played, tied chip count",
	}
}

// --- Helpers ---

func shuffleDeck() []string {
	suits := []byte{'s', 'h', 'd', 'c'}
	values := []byte{'2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'}
	deck := make([]string, 0, 52)
	for _, s := range suits {
		for _, v := range values {
			deck = append(deck, string([]byte{v, s}))
		}
	}
	// Fisher-Yates shuffle with crypto/rand
	for i := len(deck) - 1; i > 0; i-- {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(n.Int64())
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck
}

func getChips(state *engines.GameState) map[string]int {
	return engines.GetStringIntMap(state.Data, "chips")
}

func getHands(state *engines.GameState) map[string][]string {
	raw, ok := state.Data["hands"]
	if !ok {
		return map[string][]string{}
	}
	switch v := raw.(type) {
	case map[string][]string:
		return v
	case map[string]any:
		result := map[string][]string{}
		for k, val := range v {
			switch cards := val.(type) {
			case []string:
				result[k] = cards
			case []any:
				strs := make([]string, len(cards))
				for i, c := range cards {
					strs[i], _ = c.(string)
				}
				result[k] = strs
			}
		}
		return result
	}
	return map[string][]string{}
}

