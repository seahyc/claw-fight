package prisoners_dilemma

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/claw-fight/server/engines"
)

const (
	minRounds = 50
	maxRounds = 100
)

type ChaosEvent struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	SpyPlayer   int    `json:"spy_player"`
}

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Name() string   { return "prisoners_dilemma" }
func (e *Engine) MinPlayers() int { return 2 }
func (e *Engine) MaxPlayers() int { return 2 }

func (e *Engine) DescribeRules() string {
	return "Iterated Prisoner's Dilemma with Chaos: Each round, both players simultaneously choose to cooperate or defect. " +
		"Payoffs: Both Cooperate: 3 pts each. One Defects while other Cooperates: defector gets 7, cooperator gets 0. " +
		"Both Defect: 1 pt each. " +
		"Chaos Events (30% chance per round): double_stakes (2x scores), betrayal_bonus (+3 for defecting), " +
		"mercy_round (CC=6,6 DD=0,0), spy_round (one player sees the other's choice first), " +
		"reversal (choices are flipped), jackpot (CC=5,5 one-defect=10,0 DD=0,0). " +
		"Hidden Objectives: Each player gets a secret objective worth 20 bonus points. " +
		"Danger Zone: A player 50+ points behind gets 1.5x scoring for 3 rounds. " +
		"Elimination: If your score drops to 0 or below, the game ends immediately. " +
		"The game lasts 50-100 rounds (exact number hidden). Highest cumulative score wins."
}

func (e *Engine) InitGame(players []engines.PlayerID, options map[string]any) (*engines.GameState, error) {
	if len(players) != 2 {
		return nil, fmt.Errorf("prisoners_dilemma requires exactly 2 players")
	}

	totalRounds := cryptoRandInt(minRounds, maxRounds+1)

	scores := make(map[string]any)
	for _, p := range players {
		scores[string(p)] = 0
	}

	// Assign different secret objectives
	objectives := []struct{ name, desc string }{
		{"The Betrayer", "Defect at least 8 times during the game"},
		{"The Streak", "Cooperate 5 times in a row"},
		{"The Alternator", "Alternate between cooperate and defect for 6 consecutive rounds"},
		{"The Closer", "Defect in the last 3 rounds of the game"},
		{"The Mirror", "Match your opponent's previous choice at least 10 times"},
	}

	idx1 := cryptoRandInt(0, len(objectives))
	idx2 := idx1
	for idx2 == idx1 {
		idx2 = cryptoRandInt(0, len(objectives))
	}

	secretObjectives := map[string]any{
		string(players[0]): map[string]any{"name": objectives[idx1].name, "description": objectives[idx1].desc},
		string(players[1]): map[string]any{"name": objectives[idx2].name, "description": objectives[idx2].desc},
	}

	dangerZone := map[string]any{
		string(players[0]): map[string]any{"active": false, "rounds_remaining": 0},
		string(players[1]): map[string]any{"active": false, "rounds_remaining": 0},
	}

	return &engines.GameState{
		Phase:      "play",
		TurnNumber: 0,
		Data: map[string]any{
			"total_rounds":     totalRounds,
			"scores":           scores,
			"round_choices":    make(map[string]any),
			"history":          []any{},
			"round_scores":     []any{},
			"events":           map[string]any{},
			"secret_objectives": secretObjectives,
			"danger_zone":      dangerZone,
		},
		Players:     players,
		CurrentTurn: "",
	}, nil
}

func (e *Engine) ValidateAction(state *engines.GameState, player engines.PlayerID, action engines.Action) error {
	if action.Type != "choose" {
		return fmt.Errorf("invalid action type: %s, must be 'choose'", action.Type)
	}

	choiceRaw, ok := action.Data["choice"]
	if !ok {
		return fmt.Errorf("missing 'choice' field")
	}

	choice, ok := choiceRaw.(string)
	if !ok {
		return fmt.Errorf("choice must be a string")
	}

	if choice != "cooperate" && choice != "defect" {
		return fmt.Errorf("choice must be 'cooperate' or 'defect', got '%s'", choice)
	}

	roundChoices := state.Data["round_choices"].(map[string]any)
	if _, already := roundChoices[string(player)]; already {
		return fmt.Errorf("already submitted choice for this round")
	}

	// Spy round: block spy from submitting before non-spy
	event := e.getOrGenerateEvent(state)
	if event != nil && event["type"] == "spy_round" {
		spyIdx := engines.ToInt(event["spy_player"])
		if int(spyIdx) >= 0 && int(spyIdx) < len(state.Players) {
			spyPlayer := state.Players[spyIdx]
			if player == spyPlayer && len(roundChoices) == 0 {
				return fmt.Errorf("spy must wait for opponent to choose first")
			}
		}
	}

	return nil
}

func (e *Engine) ApplyAction(state *engines.GameState, player engines.PlayerID, action engines.Action) (*engines.ActionResult, error) {
	choice := action.Data["choice"].(string)
	roundChoices := state.Data["round_choices"].(map[string]any)

	roundChoices[string(player)] = choice

	// Ensure event is generated for this round
	event := e.getOrGenerateEvent(state)

	// Spy round: if non-spy submitted first, reveal to spy and wait
	if event != nil && event["type"] == "spy_round" && len(roundChoices) == 1 {
		spyIdx := engines.ToInt(event["spy_player"])
		if spyIdx >= 0 && spyIdx < len(state.Players) {
			spyPlayer := state.Players[spyIdx]
			if player != spyPlayer {
				// Non-spy submitted first - store revealed choice and set turn to spy
				state.Data["spy_revealed_choice"] = choice
				state.CurrentTurn = spyPlayer
				return &engines.ActionResult{
					Success: true,
					Message: "Waiting for opponent's choice",
					Data:    map[string]any{"status": "waiting"},
				}, nil
			}
		}
	}

	// Check if both players have submitted
	if len(roundChoices) < 2 {
		return &engines.ActionResult{
			Success: true,
			Message: "Waiting for opponent's choice",
			Data:    map[string]any{"status": "waiting"},
		}, nil
	}

	// Both players have chosen - resolve the round
	p1 := state.Players[0]
	p2 := state.Players[1]
	c1 := roundChoices[string(p1)].(string)
	c2 := roundChoices[string(p2)].(string)

	s1, s2 := calculateScores(c1, c2)

	// Apply chaos event modifiers
	if event != nil {
		s1, s2 = applyChaosModifier(event, c1, c2, s1, s2)
	}

	// Danger zone 1.5x multiplier
	dangerZone := state.Data["danger_zone"].(map[string]any)
	dz1 := dangerZone[string(p1)].(map[string]any)
	dz2 := dangerZone[string(p2)].(map[string]any)
	if engines.ToBool(dz1["active"]) {
		s1 = s1 * 3 / 2
	}
	if engines.ToBool(dz2["active"]) {
		s2 = s2 * 3 / 2
	}

	// Update cumulative scores
	scores := state.Data["scores"].(map[string]any)
	scores[string(p1)] = engines.ToInt(scores[string(p1)]) + s1
	scores[string(p2)] = engines.ToInt(scores[string(p2)]) + s2

	// Update danger zone
	updateDangerZone(dangerZone, state.Players, scores)

	// Record history
	roundResult := map[string]any{
		string(p1): c1,
		string(p2): c2,
	}
	if event != nil {
		roundResult["event"] = event["type"]
	}
	roundScoreEntry := map[string]any{
		string(p1): s1,
		string(p2): s2,
	}

	history := state.Data["history"].([]any)
	state.Data["history"] = append(history, roundResult)

	roundScores := state.Data["round_scores"].([]any)
	state.Data["round_scores"] = append(roundScores, roundScoreEntry)

	// Clean up spy state
	delete(state.Data, "spy_revealed_choice")

	// Advance turn and clear choices
	state.TurnNumber++
	state.Data["round_choices"] = make(map[string]any)
	state.CurrentTurn = ""

	return &engines.ActionResult{
		Success: true,
		Message: fmt.Sprintf("Round %d resolved", state.TurnNumber),
		Data: map[string]any{
			"status": "resolved",
			"round":  state.TurnNumber,
			"choices": map[string]any{
				string(p1): c1,
				string(p2): c2,
			},
			"round_scores": map[string]any{
				string(p1): s1,
				string(p2): s2,
			},
			"cumulative_scores": map[string]any{
				string(p1): engines.ToInt(scores[string(p1)]),
				string(p2): engines.ToInt(scores[string(p2)]),
			},
		},
	}, nil
}

func (e *Engine) GetPlayerView(state *engines.GameState, player engines.PlayerID) *engines.PlayerView {
	scores := state.Data["scores"].(map[string]any)
	roundChoices := state.Data["round_choices"].(map[string]any)
	totalRounds := engines.ToInt(state.Data["total_rounds"])
	history := state.Data["history"].([]any)

	_, hasSubmitted := roundChoices[string(player)]

	// Generate event lazily for current round
	event := e.getOrGenerateEvent(state)

	var availableActions []string
	if !hasSubmitted && state.TurnNumber < totalRounds {
		availableActions = []string{"choose"}
	}

	// Build board: last 5 rounds of history
	maxHistory := 5
	startIdx := 0
	if len(history) > maxHistory {
		startIdx = len(history) - maxHistory
	}
	pastRounds := make([]map[string]any, len(history)-startIdx)
	for i := startIdx; i < len(history); i++ {
		hMap := history[i].(map[string]any)
		entry := map[string]any{
			"round":   i + 1,
			"choices": map[string]any{},
		}
		for _, p := range state.Players {
			if v, ok := hMap[string(p)]; ok {
				choices := entry["choices"].(map[string]any)
				choices[string(p)] = v
			}
		}
		if ev, ok := hMap["event"]; ok {
			entry["event"] = ev
		}
		pastRounds[i-startIdx] = entry
	}

	// Cooperation rates
	cooperationRates := make(map[string]any)
	for _, p := range state.Players {
		coopCount := 0
		for _, h := range history {
			hMap := h.(map[string]any)
			if hMap[string(p)] == "cooperate" {
				coopCount++
			}
		}
		rate := 0.0
		if len(history) > 0 {
			rate = float64(coopCount) / float64(len(history))
		}
		cooperationRates[string(p)] = rate
	}

	// Score board
	scoreBoard := make(map[string]any)
	for _, p := range state.Players {
		scoreBoard[string(p)] = engines.ToInt(scores[string(p)])
	}

	// Fuzzy rounds remaining
	var roundsRemaining any
	if state.TurnNumber >= minRounds-10 {
		roundsRemaining = totalRounds - state.TurnNumber
	} else {
		roundsRemaining = fmt.Sprintf("at least %d", minRounds-state.TurnNumber)
	}

	// Current event info
	var currentEvent map[string]any
	if event != nil {
		currentEvent = map[string]any{
			"type":        event["type"],
			"description": event["description"],
		}
		// If spy round and this player is the spy, include revealed choice
		if event["type"] == "spy_round" {
			spyIdx := engines.ToInt(event["spy_player"])
			if spyIdx >= 0 && spyIdx < len(state.Players) && state.Players[spyIdx] == player {
				if revealed, ok := state.Data["spy_revealed_choice"]; ok {
					currentEvent["opponent_revealed_choice"] = revealed
				}
			}
		}
	}

	// Secret objective
	secretObjectives := state.Data["secret_objectives"].(map[string]any)
	objData := secretObjectives[string(player)].(map[string]any)
	objName := objData["name"].(string)
	objDesc := objData["description"].(string)
	progress, completed := checkObjectiveProgress(state, player, objName)

	secretObjective := map[string]any{
		"name":        objName,
		"description": objDesc,
		"progress":    progress,
		"completed":   completed,
	}

	// Danger zone
	dangerZone := state.Data["danger_zone"].(map[string]any)
	playerDZ := dangerZone[string(player)].(map[string]any)
	var opponent engines.PlayerID
	for _, p := range state.Players {
		if p != player {
			opponent = p
		}
	}
	opponentDZ := dangerZone[string(opponent)].(map[string]any)

	// Simultaneous flag: false during spy round when waiting for spy
	simultaneous := true
	if event != nil && event["type"] == "spy_round" && len(roundChoices) == 1 {
		simultaneous = false
	}

	gameSpecific := map[string]any{
		"scores":              scoreBoard,
		"rounds_range":        "50-100",
		"rounds_remaining":    roundsRemaining,
		"cooperation_rates":   cooperationRates,
		"full_history_length": len(history),
		"waiting":             hasSubmitted,
		"secret_objective":    secretObjective,
		"danger_zone":         engines.ToBool(playerDZ["active"]),
		"opponent_danger_zone": engines.ToBool(opponentDZ["active"]),
	}
	if currentEvent != nil {
		gameSpecific["current_event"] = currentEvent
	}

	return &engines.PlayerView{
		Phase:            state.Phase,
		YourTurn:         !hasSubmitted && state.TurnNumber < totalRounds,
		Simultaneous:     simultaneous,
		Board:            pastRounds,
		AvailableActions: availableActions,
		TurnNumber:       state.TurnNumber + 1,
		GameSpecific:     gameSpecific,
	}
}

func (e *Engine) CheckGameOver(state *engines.GameState) *engines.GameResult {
	totalRounds := engines.ToInt(state.Data["total_rounds"])
	roundChoices := state.Data["round_choices"].(map[string]any)
	scores := state.Data["scores"].(map[string]any)
	p1 := state.Players[0]
	p2 := state.Players[1]
	s1 := engines.ToInt(scores[string(p1)])
	s2 := engines.ToInt(scores[string(p2)])

	// Elimination check
	eliminated := false
	if s1 <= 0 && state.TurnNumber > 0 {
		eliminated = true
	}
	if s2 <= 0 && state.TurnNumber > 0 {
		eliminated = true
	}

	roundsComplete := state.TurnNumber >= totalRounds && len(roundChoices) == 0

	if !eliminated && !roundsComplete {
		return nil
	}

	// Award objective bonuses
	for _, p := range state.Players {
		objData := state.Data["secret_objectives"].(map[string]any)[string(p)].(map[string]any)
		objName := objData["name"].(string)
		if checkObjective(state, p, objName) {
			scores[string(p)] = engines.ToInt(scores[string(p)]) + 20
		}
	}

	// Re-read scores after bonus
	s1 = engines.ToInt(scores[string(p1)])
	s2 = engines.ToInt(scores[string(p2)])

	resultScores := map[engines.PlayerID]int{p1: s1, p2: s2}
	state.Phase = "finished"

	reason := ""
	if eliminated {
		reason = "elimination"
	}

	if s1 > s2 {
		if reason == "" {
			reason = fmt.Sprintf("%s wins %d to %d after %d rounds", string(p1), s1, s2, state.TurnNumber)
		} else {
			reason = fmt.Sprintf("%s wins %d to %d (%s at round %d)", string(p1), s1, s2, reason, state.TurnNumber)
		}
		return &engines.GameResult{Finished: true, Winner: p1, Scores: resultScores, Reason: reason}
	} else if s2 > s1 {
		if reason == "" {
			reason = fmt.Sprintf("%s wins %d to %d after %d rounds", string(p2), s2, s1, state.TurnNumber)
		} else {
			reason = fmt.Sprintf("%s wins %d to %d (%s at round %d)", string(p2), s2, s1, reason, state.TurnNumber)
		}
		return &engines.GameResult{Finished: true, Winner: p2, Scores: resultScores, Reason: reason}
	}

	return &engines.GameResult{
		Finished: true,
		Draw:     true,
		Scores:   resultScores,
		Reason:   fmt.Sprintf("Draw at %d points each after %d rounds", s1, state.TurnNumber),
	}
}

// --- Chaos Events ---

var chaosEvents = []struct {
	eventType   string
	description string
}{
	{"double_stakes", "Double Stakes! All scores this round are doubled."},
	{"betrayal_bonus", "Betrayal Bonus! Defectors earn +3 extra points."},
	{"mercy_round", "Mercy Round! Mutual cooperation pays 6 each, mutual defection pays 0."},
	{"spy_round", "Spy Round! One player sees the other's choice before deciding."},
	{"reversal", "Reversal! Your choices are flipped before scoring."},
	{"jackpot", "Jackpot! Mutual cooperation pays 5 each, but betrayal pays 10."},
}

func generateEvent() *ChaosEvent {
	// 30% chance
	n := cryptoRandInt(0, 100)
	if n >= 30 {
		return nil
	}

	idx := cryptoRandInt(0, len(chaosEvents))
	ce := chaosEvents[idx]

	spyPlayer := -1
	if ce.eventType == "spy_round" {
		spyPlayer = cryptoRandInt(0, 2)
	}

	return &ChaosEvent{
		Type:        ce.eventType,
		Description: ce.description,
		SpyPlayer:   spyPlayer,
	}
}

func eventToMap(e *ChaosEvent) map[string]any {
	return map[string]any{
		"type":        e.Type,
		"description": e.Description,
		"spy_player":  e.SpyPlayer,
	}
}

func (e *Engine) getOrGenerateEvent(state *engines.GameState) map[string]any {
	events := state.Data["events"].(map[string]any)
	roundKey := fmt.Sprintf("%d", state.TurnNumber)
	if ev, ok := events[roundKey]; ok {
		if ev == nil {
			return nil
		}
		return ev.(map[string]any)
	}

	ce := generateEvent()
	if ce == nil {
		events[roundKey] = nil
		return nil
	}
	m := eventToMap(ce)
	events[roundKey] = m
	return m
}

func flipChoice(c string) string {
	if c == "cooperate" {
		return "defect"
	}
	return "cooperate"
}

func applyChaosModifier(event map[string]any, c1, c2 string, s1, s2 int) (int, int) {
	switch event["type"] {
	case "double_stakes":
		s1 *= 2
		s2 *= 2
	case "betrayal_bonus":
		if c1 == "defect" {
			s1 += 3
		}
		if c2 == "defect" {
			s2 += 3
		}
	case "mercy_round":
		if c1 == "cooperate" && c2 == "cooperate" {
			s1, s2 = 6, 6
		} else if c1 == "defect" && c2 == "defect" {
			s1, s2 = 0, 0
		}
		// other combos use normal matrix (already in s1, s2)
	case "reversal":
		s1, s2 = calculateScores(flipChoice(c1), flipChoice(c2))
	case "jackpot":
		switch {
		case c1 == "cooperate" && c2 == "cooperate":
			s1, s2 = 5, 5
		case c1 == "defect" && c2 == "defect":
			s1, s2 = 0, 0
		case c1 == "defect" && c2 == "cooperate":
			s1, s2 = 10, 0
		case c1 == "cooperate" && c2 == "defect":
			s1, s2 = 0, 10
		}
	case "spy_round":
		// no score modifier
	}
	return s1, s2
}

// --- Danger Zone ---

func updateDangerZone(dangerZone map[string]any, players []engines.PlayerID, scores map[string]any) {
	p1 := players[0]
	p2 := players[1]
	s1 := engines.ToInt(scores[string(p1)])
	s2 := engines.ToInt(scores[string(p2)])

	for i, p := range players {
		dz := dangerZone[string(p)].(map[string]any)
		active := engines.ToBool(dz["active"])

		if active {
			remaining := engines.ToInt(dz["rounds_remaining"]) - 1
			if remaining <= 0 {
				dz["active"] = false
				dz["rounds_remaining"] = 0
			} else {
				dz["rounds_remaining"] = remaining
			}
		} else {
			// Check if should activate
			var myScore, theirScore int
			if i == 0 {
				myScore, theirScore = s1, s2
			} else {
				myScore, theirScore = s2, s1
			}
			if theirScore-myScore >= 50 {
				dz["active"] = true
				dz["rounds_remaining"] = 3
			}
		}
	}
}

// --- Hidden Objectives ---

func checkObjective(state *engines.GameState, player engines.PlayerID, name string) bool {
	_, completed := checkObjectiveProgress(state, player, name)
	return completed
}

func checkObjectiveProgress(state *engines.GameState, player engines.PlayerID, name string) (string, bool) {
	history := state.Data["history"].([]any)
	playerKey := string(player)

	var opponent engines.PlayerID
	for _, p := range state.Players {
		if p != player {
			opponent = p
		}
	}

	switch name {
	case "The Betrayer":
		count := countChoice(history, playerKey, "defect")
		return fmt.Sprintf("%d/8 defections", count), count >= 8

	case "The Streak":
		streak := longestStreak(history, playerKey, "cooperate")
		return fmt.Sprintf("%d/5 consecutive cooperations", streak), streak >= 5

	case "The Alternator":
		alt := longestAlternating(history, playerKey)
		return fmt.Sprintf("%d/6 alternating rounds", alt), alt >= 6

	case "The Closer":
		if len(history) < 3 {
			return "0/3 final defections (game not ended)", false
		}
		count := countLastNChoice(history, playerKey, "defect", 3)
		return fmt.Sprintf("%d/3 final round defections", count), count >= 3

	case "The Mirror":
		count := countMirrors(history, playerKey, string(opponent))
		return fmt.Sprintf("%d/10 mirrored choices", count), count >= 10
	}

	return "unknown objective", false
}

func countChoice(history []any, player, choice string) int {
	count := 0
	for _, h := range history {
		hMap := h.(map[string]any)
		if hMap[player] == choice {
			count++
		}
	}
	return count
}

func longestStreak(history []any, player, choice string) int {
	best, current := 0, 0
	for _, h := range history {
		hMap := h.(map[string]any)
		if hMap[player] == choice {
			current++
			if current > best {
				best = current
			}
		} else {
			current = 0
		}
	}
	return best
}

func longestAlternating(history []any, player string) int {
	if len(history) < 2 {
		return min(len(history), 1)
	}
	best, current := 1, 1
	for i := 1; i < len(history); i++ {
		prev := history[i-1].(map[string]any)[player].(string)
		curr := history[i].(map[string]any)[player].(string)
		if curr != prev {
			current++
			if current > best {
				best = current
			}
		} else {
			current = 1
		}
	}
	return best
}

func countLastNChoice(history []any, player, choice string, n int) int {
	count := 0
	start := max(len(history)-n, 0)
	for i := start; i < len(history); i++ {
		hMap := history[i].(map[string]any)
		if hMap[player] == choice {
			count++
		}
	}
	return count
}

func countMirrors(history []any, player, opponent string) int {
	count := 0
	for i := 1; i < len(history); i++ {
		prevOpponent := history[i-1].(map[string]any)[opponent].(string)
		currPlayer := history[i].(map[string]any)[player].(string)
		if currPlayer == prevOpponent {
			count++
		}
	}
	return count
}

// --- Scoring ---

func calculateScores(c1, c2 string) (s1, s2 int) {
	switch {
	case c1 == "cooperate" && c2 == "cooperate":
		return 3, 3
	case c1 == "cooperate" && c2 == "defect":
		return 0, 7
	case c1 == "defect" && c2 == "cooperate":
		return 7, 0
	default:
		return 1, 1
	}
}

func cryptoRandInt(minVal, maxVal int) int {
	diff := maxVal - minVal
	if diff <= 0 {
		return minVal
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	if err != nil {
		return minVal
	}
	return minVal + int(n.Int64())
}
