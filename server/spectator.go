package main

import (
	"fmt"

	"github.com/claw-fight/server/engines"
)

// SpectatorView for Battleship - reveals everything
type BattleshipSpectatorView struct {
	MatchID     string         `json:"match_id"`
	GameType    string         `json:"game_type"`
	Status      string         `json:"status"`
	Players     []string       `json:"players"`
	Boards      map[string]any `json:"boards"`
	TurnNumber  int            `json:"turn_number"`
	CurrentTurn string         `json:"current_turn"`
}

func (mm *MatchManager) GetSpectatorView(m *Match) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return mm.getSpectatorViewLocked(m)
}

// getSpectatorViewLocked requires caller to already hold m.mu
func (mm *MatchManager) getSpectatorViewLocked(m *Match) map[string]any {
	// Build player info with names from DB
	playerInfos := make([]map[string]any, len(m.Players))
	for i, pid := range m.Players {
		name := pid
		if p, err := mm.db.GetPlayer(pid); err == nil && p.Name != "" {
			name = p.Name
		}
		elo := 1200
		if e, err := mm.db.GetOrCreateELO(pid, m.GameType); err == nil {
			elo = e.Rating
		}
		playerInfos[i] = map[string]any{
			"id":   pid,
			"name": name,
			"elo":  elo,
		}
	}

	if m.State == nil {
		return map[string]any{
			"match_id":  m.ID,
			"game_type": m.GameType,
			"status":    string(m.Status),
			"players":   playerInfos,
		}
	}

	spectatorState := mm.buildSpectatorGameState(m)

	// Determine current turn as player index (1 or 2)
	currentTurnIdx := 0
	for i, p := range m.Players {
		if p == string(m.State.CurrentTurn) {
			currentTurnIdx = i + 1
			break
		}
	}

	view := map[string]any{
		"match_id":     m.ID,
		"game_type":    m.GameType,
		"status":       string(m.Status),
		"players":      playerInfos,
		"game_state":   spectatorState,
		"turn_number":  m.State.TurnNumber,
		"current_turn": currentTurnIdx,
	}

	if m.State != nil && len(m.State.ActionLog) > 0 {
		logEntries := m.State.ActionLog
		if len(logEntries) > 30 {
			logEntries = logEntries[len(logEntries)-30:]
		}
		actionLog := make([]map[string]any, len(logEntries))
		for i, entry := range logEntries {
			playerName := string(entry.Player)
			if p, err := mm.db.GetPlayer(string(entry.Player)); err == nil && p.Name != "" {
				playerName = p.Name
			}
			actionLog[i] = map[string]any{
				"player":      playerName,
				"action_type": entry.Action.Type,
				"message":     entry.Result.Message,
				"seq":         entry.Seq,
			}
		}
		view["action_log"] = actionLog
	}

	return view
}

// buildSpectatorGameState creates a spectator-friendly game state from the raw
// engine state. Each game type renderer expects a different flat format.
// Caller must hold m.mu.
func (mm *MatchManager) buildSpectatorGameState(m *Match) map[string]any {
	if m.State == nil || m.Engine == nil {
		return nil
	}

	switch m.GameType {
	case "prisoners_dilemma":
		return mm.buildPrisonersSpectatorState(m)
	case "poker":
		return mm.buildPokerSpectatorState(m)
	case "battleship":
		return mm.buildBattleshipSpectatorState(m)
	case "tictactoe":
		return mm.buildTictactoeSpectatorState(m)
	default:
		// Fallback: raw player views keyed by player ID
		views := make(map[string]any)
		for _, p := range m.Players {
			views[p] = m.Engine.GetPlayerView(m.State, engines.PlayerID(p))
		}
		return views
	}
}

func (mm *MatchManager) buildPrisonersSpectatorState(m *Match) map[string]any {
	data := m.State.Data
	scores := data["scores"].(map[string]any)
	history := data["history"].([]any)
	totalRounds := data["total_rounds"]

	p1 := m.Players[0]
	p2 := m.Players[1]

	// Build score arrays [p1Score, p2Score]
	scoreArr := []any{scores[p1], scores[p2]}

	// Build cooperation rates
	coopCounts := map[string]int{p1: 0, p2: 0}
	for _, h := range history {
		hMap := h.(map[string]any)
		if hMap[p1] == "cooperate" {
			coopCounts[p1]++
		}
		if hMap[p2] == "cooperate" {
			coopCounts[p2]++
		}
	}
	var coopRates []float64
	if len(history) > 0 {
		coopRates = []float64{
			float64(coopCounts[p1]) / float64(len(history)),
			float64(coopCounts[p2]) / float64(len(history)),
		}
	} else {
		coopRates = []float64{0, 0}
	}

	// Build moves array: each entry is [p1choice, p2choice]
	moves := make([][]string, len(history))
	for i, h := range history {
		hMap := h.(map[string]any)
		c1, _ := hMap[p1].(string)
		c2, _ := hMap[p2].(string)
		moves[i] = []string{c1, c2}
	}

	// Build cumulative score history for chart: [[p1cumul, p2cumul], ...]
	scoreHistory := make([][]int, 0, len(history))
	roundScores, _ := data["round_scores"].([]any)
	cumP1, cumP2 := 0, 0
	for _, rs := range roundScores {
		rsMap := rs.(map[string]any)
		s1, _ := rsMap[p1]
		s2, _ := rsMap[p2]
		cumP1 += engines.ToInt(s1)
		cumP2 += engines.ToInt(s2)
		scoreHistory = append(scoreHistory, []int{cumP1, cumP2})
	}

	result := map[string]any{
		"current_round":     m.State.TurnNumber + 1,
		"total_rounds":      totalRounds,
		"scores":            scoreArr,
		"cooperation_rates": coopRates,
		"moves":             moves,
		"score_history":     scoreHistory,
	}

	// Chaos event for current round
	if events, ok := data["events"].(map[string]any); ok {
		roundKey := fmt.Sprintf("%d", m.State.TurnNumber)
		if event, ok := events[roundKey]; ok {
			result["current_event"] = event
		}
	}

	// Secret objectives (spectators see both)
	if objectives, ok := data["secret_objectives"].(map[string]any); ok {
		spectatorObjs := make([]map[string]any, 2)
		for i, pid := range []string{p1, p2} {
			if obj, ok := objectives[pid].(map[string]any); ok {
				spectatorObjs[i] = map[string]any{
					"name":        obj["name"],
					"description": obj["description"],
				}
			} else {
				spectatorObjs[i] = map[string]any{}
			}
		}
		result["secret_objectives"] = spectatorObjs
	}

	// Danger zone status
	if dangerZone, ok := data["danger_zone"].(map[string]any); ok {
		dzStatus := make([]bool, 2)
		for i, pid := range []string{p1, p2} {
			if dz, ok := dangerZone[pid].(map[string]any); ok {
				dzStatus[i], _ = dz["active"].(bool)
			}
		}
		result["danger_zone"] = dzStatus
	}

	return result
}

func (mm *MatchManager) buildPokerSpectatorState(m *Match) map[string]any {
	data := m.State.Data
	chips := data["chips"].(map[string]any)
	community, _ := data["community"]
	pot := data["pot"]
	playerBets, _ := data["player_bets"]
	allIn, _ := data["all_in_players"]
	hands, _ := data["hands"]

	p1 := m.Players[0]
	p2 := m.Players[1]

	handsMap, _ := hands.(map[string]any)
	betsMap, _ := playerBets.(map[string]any)
	allInMap, _ := allIn.(map[string]any)

	// Parse community cards into card objects for renderer
	communityCards := parseCardStrings(community)

	// Build player views - show cards at showdown, hide otherwise
	showCards := m.State.Phase == "showdown" || m.State.Phase == "finished"
	showdownResult, _ := data["showdown_result"].(map[string]any)
	if showdownResult != nil {
		showCards = true
	}

	players := make([]map[string]any, 2)
	for i, pid := range []string{p1, p2} {
		p := map[string]any{
			"chips": chips[pid],
		}
		if betsMap != nil {
			p["current_bet"] = betsMap[pid]
		}
		if allInMap != nil && allInMap[pid] == true {
			p["last_action"] = "ALL IN"
		}
		// Show cards at showdown or if we have showdown_result
		if showCards && handsMap != nil {
			p["hand"] = parseCardStrings(handsMap[pid])
		} else {
			p["hand"] = []map[string]any{{}, {}} // face-down cards
		}
		players[i] = p
	}

	result := map[string]any{
		"community_cards": communityCards,
		"pot":             pot,
		"players":         players,
		"phase":           m.State.Phase,
		"hand_number":     data["hand_number"],
	}

	if showdownResult != nil {
		result["showdown"] = showdownResult
	}

	return result
}

// parseCardStrings converts card data ([]string like ["Ah","Kd"] or []any) into
// card objects [{value, suit}] for the poker renderer
func parseCardStrings(raw any) []map[string]any {
	var strs []string
	switch v := raw.(type) {
	case []string:
		strs = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				strs = append(strs, s)
			}
		}
	default:
		return nil
	}

	cards := make([]map[string]any, len(strs))
	for i, s := range strs {
		if len(s) >= 2 {
			cards[i] = map[string]any{
				"value": string(s[0]),
				"suit":  string(s[1]),
			}
		} else {
			cards[i] = map[string]any{"face_down": true}
		}
	}
	return cards
}

func (mm *MatchManager) buildBattleshipSpectatorState(m *Match) map[string]any {
	views := make(map[string]any)
	shipStatus := make(map[string]any)
	for _, p := range m.Players {
		pv := m.Engine.GetPlayerView(m.State, engines.PlayerID(p))
		views[p] = pv
		if gs, ok := pv.GameSpecific["ships"]; ok {
			switch ships := gs.(type) {
			case []map[string]any:
				total := len(ships)
				sunk := 0
				for _, ship := range ships {
					if isSunk, ok := ship["sunk"].(bool); ok && isSunk {
						sunk++
					}
				}
				shipStatus[p] = map[string]any{"sunk": sunk, "total": total}
			case []any:
				total := len(ships)
				sunk := 0
				for _, s := range ships {
					if ship, ok := s.(map[string]any); ok {
						if isSunk, ok := ship["sunk"].(bool); ok && isSunk {
							sunk++
						}
					}
				}
				shipStatus[p] = map[string]any{"sunk": sunk, "total": total}
			}
		}
	}

	result := map[string]any{
		"views":       views,
		"ship_status": shipStatus,
	}

	if m.State != nil && len(m.State.ActionLog) > 0 {
		lastEntry := m.State.ActionLog[len(m.State.ActionLog)-1]
		if lastEntry.Action.Type == "fire" {
			if target, ok := lastEntry.Action.Data["target"]; ok {
				result["last_action"] = map[string]any{
					"player": string(lastEntry.Player),
					"target": target,
				}
			}
		}
	}

	return result
}

func (mm *MatchManager) buildTictactoeSpectatorState(m *Match) map[string]any {
	data := m.State.Data
	rawBoard := data["board"].([]any)

	boardSize := len(rawBoard)
	board := make([][]string, boardSize)
	for i, rowRaw := range rawBoard {
		row := rowRaw.([]any)
		board[i] = make([]string, len(row))
		for j, cell := range row {
			if s, ok := cell.(string); ok {
				board[i][j] = s
			} else {
				board[i][j] = ""
			}
		}
	}

	currentPlayer := "X"
	if m.State.CurrentTurn == m.State.Players[1] {
		currentPlayer = "O"
	}

	return map[string]any{
		"board":          board,
		"board_size":     boardSize,
		"current_player": currentPlayer,
		"move_count":     engines.ToInt(data["move_count"]),
	}
}

func (mm *MatchManager) broadcastSpectatorState(m *Match) {
	// NOTE: caller must already hold m.mu - use getSpectatorViewLocked
	view := mm.getSpectatorViewLocked(m)
	view["type"] = "match_state"
	mm.hub.BroadcastToSpectators(m.ID, view)
}
