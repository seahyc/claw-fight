package prisoners_dilemma

import (
	"fmt"

	"github.com/claw-fight/server/engines"
)

const defaultRounds = 100

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Name() string   { return "prisoners_dilemma" }
func (e *Engine) MinPlayers() int { return 2 }
func (e *Engine) MaxPlayers() int { return 2 }

func (e *Engine) DescribeRules() string {
	return "Iterated Prisoner's Dilemma: Each round, both players simultaneously choose to cooperate or defect. " +
		"Both Cooperate: 3 pts each. One Defects while other Cooperates: defector gets 5, cooperator gets 0. " +
		"Both Defect: 1 pt each. Highest cumulative score after all rounds wins."
}

func (e *Engine) InitGame(players []engines.PlayerID, options map[string]any) (*engines.GameState, error) {
	if len(players) != 2 {
		return nil, fmt.Errorf("prisoners_dilemma requires exactly 2 players")
	}

	totalRounds := defaultRounds
	if r, ok := options["rounds"]; ok {
		switch v := r.(type) {
		case float64:
			totalRounds = int(v)
		case int:
			totalRounds = v
		}
		if totalRounds < 1 {
			totalRounds = 1
		}
	}

	scores := make(map[string]any)
	for _, p := range players {
		scores[string(p)] = 0
	}

	roundChoices := make(map[string]any)

	return &engines.GameState{
		Phase:      "play",
		TurnNumber: 0,
		Data: map[string]any{
			"total_rounds":  totalRounds,
			"scores":        scores,
			"round_choices": roundChoices,
			"history":       []any{},
			"round_scores":  []any{},
		},
		Players:     players,
		CurrentTurn: "", // simultaneous - no specific current turn
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

	return nil
}

func (e *Engine) ApplyAction(state *engines.GameState, player engines.PlayerID, action engines.Action) (*engines.ActionResult, error) {
	choice := action.Data["choice"].(string)
	roundChoices := state.Data["round_choices"].(map[string]any)

	roundChoices[string(player)] = choice

	// Check if both players have submitted
	if len(roundChoices) < 2 {
		return &engines.ActionResult{
			Success: true,
			Message: "Waiting for opponent's choice",
			Data: map[string]any{
				"status": "waiting",
			},
		}, nil
	}

	// Both players have chosen - resolve the round
	p1 := state.Players[0]
	p2 := state.Players[1]
	c1 := roundChoices[string(p1)].(string)
	c2 := roundChoices[string(p2)].(string)

	s1, s2 := calculateScores(c1, c2)

	// Update cumulative scores
	scores := state.Data["scores"].(map[string]any)
	scores[string(p1)] = toInt(scores[string(p1)]) + s1
	scores[string(p2)] = toInt(scores[string(p2)]) + s2

	// Record history
	roundResult := map[string]any{
		string(p1): c1,
		string(p2): c2,
	}
	roundScoreEntry := map[string]any{
		string(p1): s1,
		string(p2): s2,
	}

	history := state.Data["history"].([]any)
	state.Data["history"] = append(history, roundResult)

	roundScores := state.Data["round_scores"].([]any)
	state.Data["round_scores"] = append(roundScores, roundScoreEntry)

	// Advance turn and clear choices
	state.TurnNumber++
	state.Data["round_choices"] = make(map[string]any)

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
				string(p1): toInt(scores[string(p1)]),
				string(p2): toInt(scores[string(p2)]),
			},
		},
	}, nil
}

func (e *Engine) GetPlayerView(state *engines.GameState, player engines.PlayerID) *engines.PlayerView {
	scores := state.Data["scores"].(map[string]any)
	roundChoices := state.Data["round_choices"].(map[string]any)
	totalRounds := toInt(state.Data["total_rounds"])
	history := state.Data["history"].([]any)
	roundScores := state.Data["round_scores"].([]any)

	_, hasSubmitted := roundChoices[string(player)]

	var availableActions []string
	if !hasSubmitted && state.TurnNumber < totalRounds {
		availableActions = []string{"choose"}
	}

	// Build board: history of all past rounds
	pastRounds := make([]map[string]any, len(history))
	for i, h := range history {
		hMap := h.(map[string]any)
		rsMap := roundScores[i].(map[string]any)
		pastRounds[i] = map[string]any{
			"round":   i + 1,
			"choices": hMap,
			"scores":  rsMap,
		}
	}

	// Calculate cooperation rates
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

	// Cumulative scores for both players
	scoreBoard := make(map[string]any)
	for _, p := range state.Players {
		scoreBoard[string(p)] = toInt(scores[string(p)])
	}

	return &engines.PlayerView{
		Phase:            state.Phase,
		YourTurn:         !hasSubmitted && state.TurnNumber < totalRounds,
		Simultaneous:     true,
		Board:            pastRounds,
		AvailableActions: availableActions,
		TurnNumber:       state.TurnNumber + 1, // 1-indexed for display
		GameSpecific: map[string]any{
			"scores":            scoreBoard,
			"total_rounds":      totalRounds,
			"rounds_remaining":  totalRounds - state.TurnNumber,
			"cooperation_rates": cooperationRates,
			"waiting":           hasSubmitted,
		},
	}
}

func (e *Engine) CheckGameOver(state *engines.GameState) *engines.GameResult {
	totalRounds := toInt(state.Data["total_rounds"])
	roundChoices := state.Data["round_choices"].(map[string]any)

	// Game is over when all rounds are complete and no pending choices
	if state.TurnNumber < totalRounds || len(roundChoices) > 0 {
		return nil
	}

	scores := state.Data["scores"].(map[string]any)
	p1 := state.Players[0]
	p2 := state.Players[1]
	s1 := toInt(scores[string(p1)])
	s2 := toInt(scores[string(p2)])

	resultScores := map[engines.PlayerID]int{
		p1: s1,
		p2: s2,
	}

	state.Phase = "finished"

	if s1 > s2 {
		return &engines.GameResult{
			Finished: true,
			Winner:   p1,
			Scores:   resultScores,
			Reason:   fmt.Sprintf("%s wins %d to %d after %d rounds", string(p1), s1, s2, totalRounds),
		}
	} else if s2 > s1 {
		return &engines.GameResult{
			Finished: true,
			Winner:   p2,
			Scores:   resultScores,
			Reason:   fmt.Sprintf("%s wins %d to %d after %d rounds", string(p2), s2, s1, totalRounds),
		}
	}

	return &engines.GameResult{
		Finished: true,
		Draw:     true,
		Scores:   resultScores,
		Reason:   fmt.Sprintf("Draw at %d points each after %d rounds", s1, totalRounds),
	}
}

func calculateScores(c1, c2 string) (s1, s2 int) {
	switch {
	case c1 == "cooperate" && c2 == "cooperate":
		return 3, 3
	case c1 == "cooperate" && c2 == "defect":
		return 0, 5
	case c1 == "defect" && c2 == "cooperate":
		return 5, 0
	default: // both defect
		return 1, 1
	}
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
