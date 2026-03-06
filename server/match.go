package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/claw-fight/server/engines"
)

const (
	defaultTurnTimeout = 60 * time.Second
	defaultPrepTime    = 5 * time.Second
	maxForfeits        = 3
)

type MatchStatus string

const (
	StatusWaiting  MatchStatus = "waiting"
	StatusPrep     MatchStatus = "prep"
	StatusActive   MatchStatus = "active"
	StatusFinished MatchStatus = "finished"
)

type Match struct {
	mu            sync.Mutex
	ID            string
	GameType      string
	Players       []string
	Engine        engines.GameEngine
	State         *engines.GameState
	Status        MatchStatus
	ChallengeCode string
	CreatedAt     time.Time
	StartedAt     time.Time
	EndedAt       time.Time
	WinnerID      string
	TurnTimeout   time.Duration
	PrepDuration  time.Duration
	CurrentTurn   string
	ReadyPlayers  map[string]bool
	ForfeitCount  map[string]int
	TurnTimer     *time.Timer
	PrepTimer     *time.Timer
	EventSeq      int
}

type MatchManager struct {
	mu       sync.RWMutex
	matches  map[string]*Match
	byCode   map[string]*Match
	hub      *Hub
	db       *DB
	registry map[string]engines.GameEngine
}

func NewMatchManager(hub *Hub, db *DB) *MatchManager {
	return &MatchManager{
		matches:  make(map[string]*Match),
		byCode:   make(map[string]*Match),
		hub:      hub,
		db:       db,
		registry: make(map[string]engines.GameEngine),
	}
}

func (mm *MatchManager) RegisterEngine(engine engines.GameEngine) {
	mm.registry[engine.Name()] = engine
}

func (mm *MatchManager) GetEngine(name string) engines.GameEngine {
	return mm.registry[name]
}

func (mm *MatchManager) ListGames() []map[string]any {
	var games []map[string]any
	for _, e := range mm.registry {
		games = append(games, map[string]any{
			"name":        e.Name(),
			"min_players": e.MinPlayers(),
			"max_players": e.MaxPlayers(),
			"rules":       e.DescribeRules(),
		})
	}
	return games
}

func (mm *MatchManager) ListOpenMatches() []map[string]any {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	var open []map[string]any
	for _, m := range mm.matches {
		m.mu.Lock()
		if m.Status == StatusWaiting {
			open = append(open, map[string]any{
				"match_id":  m.ID,
				"game_type": m.GameType,
				"code":      m.ChallengeCode,
				"players":   len(m.Players),
				"needs":     m.Engine.MaxPlayers() - len(m.Players),
			})
		}
		m.mu.Unlock()
	}
	return open
}

func (mm *MatchManager) CreateMatch(gameType, playerID string, options map[string]any) (*Match, error) {
	engine := mm.GetEngine(gameType)
	if engine == nil {
		return nil, fmt.Errorf("unknown game type: %s", gameType)
	}

	matchID := generateID(8)
	code := generateCode(6)

	m := &Match{
		ID:            matchID,
		GameType:      gameType,
		Players:       []string{playerID},
		Engine:        engine,
		Status:        StatusWaiting,
		ChallengeCode: code,
		CreatedAt:     time.Now(),
		TurnTimeout:   defaultTurnTimeout,
		PrepDuration:  defaultPrepTime,
		ReadyPlayers:  make(map[string]bool),
		ForfeitCount:  make(map[string]int),
	}

	mm.mu.Lock()
	mm.matches[matchID] = m
	mm.byCode[code] = m
	mm.mu.Unlock()

	if err := mm.db.CreateMatch(matchID, gameType, code); err != nil {
		log.Printf("Failed to persist match: %v", err)
	}
	if err := mm.db.AddMatchPlayer(matchID, playerID, 0); err != nil {
		log.Printf("Failed to persist match player: %v", err)
	}

	log.Printf("Match created: %s (code: %s, game: %s, by: %s)", matchID, code, gameType, playerID)
	return m, nil
}

func (mm *MatchManager) JoinMatch(matchID, playerID string) (*Match, error) {
	mm.mu.RLock()
	m, ok := mm.matches[matchID]
	mm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("match not found: %s", matchID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Status != StatusWaiting {
		return nil, fmt.Errorf("match is not accepting players")
	}

	if slices.Contains(m.Players, playerID) {
		return nil, fmt.Errorf("already in this match")
	}

	if len(m.Players) >= m.Engine.MaxPlayers() {
		return nil, fmt.Errorf("match is full")
	}

	m.Players = append(m.Players, playerID)

	if err := mm.db.AddMatchPlayer(matchID, playerID, len(m.Players)-1); err != nil {
		log.Printf("Failed to persist match player: %v", err)
	}

	if len(m.Players) >= m.Engine.MinPlayers() {
		mm.startPrepPhase(m)
	}

	return m, nil
}

func (mm *MatchManager) JoinByCode(code, playerID string) (*Match, error) {
	mm.mu.RLock()
	m, ok := mm.byCode[code]
	mm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("invalid challenge code: %s", code)
	}
	return mm.JoinMatch(m.ID, playerID)
}

func (mm *MatchManager) startPrepPhase(m *Match) {
	m.Status = StatusPrep
	mm.db.UpdateMatchStatus(m.ID, "prep")

	log.Printf("Match %s entering prep phase (%v)", m.ID, m.PrepDuration)

	for _, pid := range m.Players {
		if c := mm.hub.GetClientByPlayer(pid); c != nil {
			c.SendJSON(map[string]any{
				"type":     "prep_phase",
				"match_id": m.ID,
				"duration": int(m.PrepDuration.Seconds()),
			})
		}
	}

	m.PrepTimer = time.AfterFunc(m.PrepDuration, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.Status == StatusPrep {
			mm.startGame(m)
		}
	})
}

func (mm *MatchManager) PlayerReady(matchID, playerID string) error {
	mm.mu.RLock()
	m, ok := mm.matches[matchID]
	mm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("match not found")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Status != StatusPrep {
		return fmt.Errorf("match is not in prep phase")
	}

	m.ReadyPlayers[playerID] = true

	allReady := true
	for _, p := range m.Players {
		if !m.ReadyPlayers[p] {
			allReady = false
			break
		}
	}

	if allReady {
		if m.PrepTimer != nil {
			m.PrepTimer.Stop()
		}
		mm.startGame(m)
	}

	return nil
}

func (mm *MatchManager) startGame(m *Match) {
	playerIDs := make([]engines.PlayerID, len(m.Players))
	for i, p := range m.Players {
		playerIDs[i] = engines.PlayerID(p)
	}

	state, err := m.Engine.InitGame(playerIDs, nil)
	if err != nil {
		log.Printf("Failed to init game for match %s: %v", m.ID, err)
		return
	}

	m.State = state
	m.Status = StatusActive
	m.StartedAt = time.Now()
	m.CurrentTurn = string(state.CurrentTurn)

	mm.db.StartMatch(m.ID)

	log.Printf("Match %s started: %v", m.ID, m.Players)

	for _, pid := range m.Players {
		if c := mm.hub.GetClientByPlayer(pid); c != nil {
			c.SendJSON(map[string]any{
				"type":     "game_start",
				"match_id": m.ID,
			})
			view := m.Engine.GetPlayerView(state, engines.PlayerID(pid))
			mm.sendPlayerTurn(c, m.ID, view)
		}
	}

	mm.broadcastSpectatorState(m)
	mm.startTurnTimer(m)
}

func (mm *MatchManager) HandleAction(matchID, playerID string, action engines.Action) error {
	mm.mu.RLock()
	m, ok := mm.matches[matchID]
	mm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("match not found")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Status != StatusActive {
		return fmt.Errorf("match is not active")
	}

	pid := engines.PlayerID(playerID)

	log.Printf("HandleAction: match=%s player=%s action_type=%s data=%v", matchID, playerID, action.Type, action.Data)

	if err := m.Engine.ValidateAction(m.State, pid, action); err != nil {
		if c := mm.hub.GetClientByPlayer(playerID); c != nil {
			c.SendJSON(map[string]any{
				"type":     "action_result",
				"match_id": m.ID,
				"success":  false,
				"message":  err.Error(),
			})
		}
		return err
	}

	result, err := m.Engine.ApplyAction(m.State, pid, action)
	if err != nil {
		return err
	}

	m.EventSeq++
	mm.db.RecordEvent(m.ID, m.EventSeq, playerID, action.Type, action, result)

	// Stop current turn timer
	if m.TurnTimer != nil {
		m.TurnTimer.Stop()
	}

	// Send result to acting player
	if c := mm.hub.GetClientByPlayer(playerID); c != nil {
		c.SendJSON(map[string]any{
			"type":     "action_result",
			"match_id": m.ID,
			"success":  result.Success,
			"message":  result.Message,
			"data":     result.Data,
		})
	}

	// Send opponent action to other players
	for _, p := range m.Players {
		if p != playerID {
			if c := mm.hub.GetClientByPlayer(p); c != nil {
				c.SendJSON(map[string]any{
					"type":        "opponent_action",
					"match_id":    m.ID,
					"action_type": action.Type,
					"message":     result.Message,
					"data":        result.Data,
				})
			}
		}
	}

	// Broadcast to spectators - resolve player name
	playerName := playerID
	if p, err := mm.db.GetPlayer(playerID); err == nil && p.Name != "" {
		playerName = p.Name
	}
	actionText := action.Type
	if result.Message != "" {
		actionText += " - " + result.Message
	}
	spectatorState := mm.buildSpectatorGameState(m)
	// Determine current turn index for spectator
	currentTurnIdx := 0
	for i, p := range m.Players {
		if p == string(m.State.CurrentTurn) {
			currentTurnIdx = i + 1
			break
		}
	}
	mm.hub.BroadcastToSpectators(m.ID, map[string]any{
		"type":         "action",
		"match_id":     m.ID,
		"player":       playerName,
		"action_type":  action.Type,
		"text":         actionText,
		"result":       result,
		"game_state":   spectatorState,
		"current_turn": currentTurnIdx,
		"timestamp":    time.Now().UnixMilli(),
	})

	// Check game over
	gameResult := m.Engine.CheckGameOver(m.State)
	if gameResult != nil && gameResult.Finished {
		mm.finishMatch(m, gameResult)
		return nil
	}

	// Update current turn and send state
	m.CurrentTurn = string(m.State.CurrentTurn)
	for _, p := range m.Players {
		if c := mm.hub.GetClientByPlayer(p); c != nil {
			view := m.Engine.GetPlayerView(m.State, engines.PlayerID(p))
			mm.sendPlayerTurn(c, m.ID, view)
		}
	}

	mm.broadcastSpectatorState(m)
	mm.startTurnTimer(m)

	return nil
}

func (mm *MatchManager) startTurnTimer(m *Match) {
	if m.TurnTimeout <= 0 {
		return
	}
	currentPlayer := m.CurrentTurn
	m.TurnTimer = time.AfterFunc(m.TurnTimeout, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.Status != StatusActive || m.CurrentTurn != currentPlayer {
			return
		}
		m.ForfeitCount[currentPlayer]++
		log.Printf("Turn timeout for player %s in match %s (forfeit %d/%d)",
			currentPlayer, m.ID, m.ForfeitCount[currentPlayer], maxForfeits)

		if m.ForfeitCount[currentPlayer] >= maxForfeits {
			var winner string
			for _, p := range m.Players {
				if p != currentPlayer {
					winner = p
					break
				}
			}
			mm.finishMatch(m, &engines.GameResult{
				Finished: true,
				Winner:   engines.PlayerID(winner),
				Reason:   fmt.Sprintf("%s forfeited (3 turn timeouts)", currentPlayer),
			})
			return
		}

		// Skip turn - advance to next player
		for i, p := range m.Players {
			if p == currentPlayer {
				nextIdx := (i + 1) % len(m.Players)
				m.CurrentTurn = m.Players[nextIdx]
				m.State.CurrentTurn = engines.PlayerID(m.CurrentTurn)
				break
			}
		}

		for _, p := range m.Players {
			if c := mm.hub.GetClientByPlayer(p); c != nil {
				view := m.Engine.GetPlayerView(m.State, engines.PlayerID(p))
				mm.sendPlayerTurn(c, m.ID, view)
			}
		}
		mm.startTurnTimer(m)
	})
}

func (mm *MatchManager) finishMatch(m *Match, result *engines.GameResult) {
	m.Status = StatusFinished
	m.EndedAt = time.Now()
	m.WinnerID = string(result.Winner)

	mm.db.EndMatch(m.ID, m.WinnerID)

	if m.TurnTimer != nil {
		m.TurnTimer.Stop()
	}

	log.Printf("Match %s finished. Winner: %s, Reason: %s", m.ID, result.Winner, result.Reason)

	// Update ELO
	if len(m.Players) == 2 && !result.Draw {
		winner := m.WinnerID
		var loser string
		for _, p := range m.Players {
			if p != winner {
				loser = p
				break
			}
		}
		mm.updateELO(winner, loser, m.GameType, result.Draw)
	}

	// Notify players via event queue
	for _, p := range m.Players {
		if c := mm.hub.GetClientByPlayer(p); c != nil {
			c.QueueEvent(map[string]any{
				"type":     "game_over",
				"match_id": m.ID,
				"winner":   string(result.Winner),
				"draw":     result.Draw,
				"scores":   result.Scores,
				"reason":   result.Reason,
			})
		}
	}

	// Notify spectators
	finalState := mm.buildSpectatorGameState(m)
	mm.hub.BroadcastToSpectators(m.ID, map[string]any{
		"type":       "game_over",
		"match_id":   m.ID,
		"result":     fmt.Sprintf("%s", result.Reason),
		"game_state": finalState,
		"timestamp":  time.Now().UnixMilli(),
	})

	// Cleanup
	mm.mu.Lock()
	delete(mm.byCode, m.ChallengeCode)
	mm.mu.Unlock()
}

func (mm *MatchManager) updateELO(winnerID, loserID, gameType string, draw bool) {
	wElo, err := mm.db.GetOrCreateELO(winnerID, gameType)
	if err != nil {
		log.Printf("Failed to get ELO for %s: %v", winnerID, err)
		return
	}
	lElo, err := mm.db.GetOrCreateELO(loserID, gameType)
	if err != nil {
		log.Printf("Failed to get ELO for %s: %v", loserID, err)
		return
	}

	wNew, lNew := CalculateELO(wElo.Rating, lElo.Rating, wElo.GamesPlayed, lElo.GamesPlayed, draw)

	mm.db.UpdateELO(winnerID, gameType, wNew, wElo.GamesPlayed+1)
	mm.db.UpdateELO(loserID, gameType, lNew, lElo.GamesPlayed+1)

	log.Printf("ELO updated: %s %d->%d, %s %d->%d", winnerID, wElo.Rating, wNew, loserID, lElo.Rating, lNew)
}

func (mm *MatchManager) sendPlayerTurn(c *Client, matchID string, view *engines.PlayerView) {
	c.QueueEvent(map[string]any{
		"type":              "your_turn",
		"match_id":          matchID,
		"phase":             view.Phase,
		"your_turn":         view.YourTurn,
		"simultaneous":      view.Simultaneous,
		"board":             view.Board,
		"available_actions": view.AvailableActions,
		"turn_number":       view.TurnNumber,
		"game_specific":     view.GameSpecific,
	})
}

func (mm *MatchManager) GetMatch(id string) *Match {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.matches[id]
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
		cumP1 += toInt(s1)
		cumP2 += toInt(s2)
		scoreHistory = append(scoreHistory, []int{cumP1, cumP2})
	}

	result := map[string]any{
		"current_round":    m.State.TurnNumber + 1,
		"total_rounds":     totalRounds,
		"scores":           scoreArr,
		"cooperation_rates": coopRates,
		"moves":            moves,
		"score_history":    scoreHistory,
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

func (mm *MatchManager) broadcastSpectatorState(m *Match) {
	// NOTE: caller must already hold m.mu - use getSpectatorViewLocked
	view := mm.getSpectatorViewLocked(m)
	view["type"] = "match_state"
	mm.hub.BroadcastToSpectators(m.ID, view)
}

func generateID(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	return randomString(n, chars)
}

func generateCode(n int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return randomString(n, chars)
}

func randomString(n int, chars string) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[idx.Int64()]
	}
	return string(b)
}

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

// Helper to marshal match info for JSON API
func (m *Match) ToJSON() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := map[string]any{
		"id":             m.ID,
		"game_type":      m.GameType,
		"status":         string(m.Status),
		"players":        m.Players,
		"challenge_code": m.ChallengeCode,
		"created_at":     m.CreatedAt,
	}
	if !m.StartedAt.IsZero() {
		result["started_at"] = m.StartedAt
	}
	if !m.EndedAt.IsZero() {
		result["ended_at"] = m.EndedAt
		result["winner_id"] = m.WinnerID
	}
	return result
}

// GetState returns player view for a specific player, thread-safe
func (mm *MatchManager) GetState(matchID, playerID string) (map[string]any, error) {
	mm.mu.RLock()
	m, ok := mm.matches[matchID]
	mm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("match not found")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.State == nil {
		return map[string]any{
			"type":     "game_state",
			"match_id": m.ID,
			"phase":    string(m.Status),
		}, nil
	}

	if !slices.Contains(m.Players, playerID) {
		return nil, fmt.Errorf("you are not a player in this match")
	}

	view := m.Engine.GetPlayerView(m.State, engines.PlayerID(playerID))
	if view == nil {
		return nil, fmt.Errorf("could not get player view")
	}
	return map[string]any{
		"type":              "game_state",
		"match_id":          m.ID,
		"phase":             view.Phase,
		"your_turn":         view.YourTurn,
		"simultaneous":      view.Simultaneous,
		"board":             view.Board,
		"available_actions": view.AvailableActions,
		"turn_number":       view.TurnNumber,
		"game_specific":     view.GameSpecific,
	}, nil
}

// QuitMatch removes a player from a match, resetting it to waiting state.
// The match stays open for a new player to join with the same code.
func (mm *MatchManager) QuitMatch(matchID, playerID string) error {
	mm.mu.RLock()
	m, ok := mm.matches[matchID]
	mm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("match not found: %s", matchID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	idx := slices.Index(m.Players, playerID)
	if idx == -1 {
		return fmt.Errorf("you are not in this match")
	}

	if m.Status == StatusFinished {
		return fmt.Errorf("match is already finished")
	}

	// Stop any running timers
	if m.TurnTimer != nil {
		m.TurnTimer.Stop()
	}
	if m.PrepTimer != nil {
		m.PrepTimer.Stop()
	}

	// Remove the player
	m.Players = slices.Delete(m.Players, idx, idx+1)
	m.State = nil
	m.Status = StatusWaiting
	m.ReadyPlayers = make(map[string]bool)
	m.ForfeitCount = make(map[string]int)
	m.CurrentTurn = ""

	mm.db.UpdateMatchStatus(m.ID, "waiting")

	// Notify remaining players that opponent left
	for _, pid := range m.Players {
		if c := mm.hub.GetClientByPlayer(pid); c != nil {
			c.QueueEvent(map[string]any{
				"type":     "opponent_left",
				"match_id": m.ID,
				"message":  "Your opponent has left the match. Waiting for a new player.",
			})
		}
	}

	log.Printf("Player %s quit match %s", playerID, matchID)
	return nil
}

// EndMatch closes a match entirely. Only the creator (first player) can end it.
func (mm *MatchManager) EndMatch(matchID, playerID string) error {
	mm.mu.RLock()
	m, ok := mm.matches[matchID]
	mm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("match not found: %s", matchID)
	}

	m.mu.Lock()

	if m.Status == StatusFinished {
		m.mu.Unlock()
		return fmt.Errorf("match is already finished")
	}

	// Check if caller is in the match (creator may have been index 0 originally,
	// but after quits the Players slice changes - allow any current player to end)
	if !slices.Contains(m.Players, playerID) {
		m.mu.Unlock()
		return fmt.Errorf("you are not in this match")
	}

	// Stop any running timers
	if m.TurnTimer != nil {
		m.TurnTimer.Stop()
	}
	if m.PrepTimer != nil {
		m.PrepTimer.Stop()
	}

	// Notify all connected players
	for _, pid := range m.Players {
		if pid != playerID {
			if c := mm.hub.GetClientByPlayer(pid); c != nil {
				c.QueueEvent(map[string]any{
					"type":     "match_ended",
					"match_id": m.ID,
					"message":  "The match has been ended.",
				})
			}
		}
	}

	m.Status = StatusFinished
	m.EndedAt = time.Now()
	m.mu.Unlock()

	// Cleanup from manager maps
	mm.mu.Lock()
	delete(mm.byCode, m.ChallengeCode)
	delete(mm.matches, m.ID)
	mm.mu.Unlock()

	mm.db.UpdateMatchStatus(m.ID, "ended")

	log.Printf("Match %s ended by player %s", matchID, playerID)
	return nil
}

// MarshalAction parses an action from a raw JSON message
func MarshalAction(data json.RawMessage) (engines.Action, error) {
	var action engines.Action
	if err := json.Unmarshal(data, &action); err != nil {
		return action, err
	}
	return action, nil
}
