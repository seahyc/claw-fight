package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/claw-fight/server/engines"
)

const (
	defaultTurnTimeout = 5 * time.Minute
	defaultPrepTime    = 5 * time.Second
	maxForfeits        = 3
	disconnectGrace    = 5 * time.Minute
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

	// Activity tracking
	LastActivityAt time.Time // updated on every action or player join

	// Disconnect grace period tracking
	Disconnected   map[string]time.Time   // playerID -> disconnect time
	GraceTimers    map[string]*time.Timer // playerID -> grace expiry timer
	TurnPausedFor  string                 // playerID whose turn timer is paused due to disconnect
	TurnPausedLeft time.Duration          // remaining turn time when paused
}

type MatchManager struct {
	mu       sync.RWMutex
	matches  map[string]*Match
	hub      *Hub
	db       *DB
	registry map[string]engines.GameEngine
}

func NewMatchManager(hub *Hub, db *DB) *MatchManager {
	mm := &MatchManager{
		matches:  make(map[string]*Match),
		hub:      hub,
		db:       db,
		registry: make(map[string]engines.GameEngine),
	}
	go mm.runCleanup()
	return mm
}

const (
	waitingMatchTTL = 30 * time.Minute // waiting match with no activity
	activeMatchTTL  = 30 * time.Minute // active match with no moves
)

// runCleanup periodically removes stale matches that have had no activity.
// This catches agents that stop mid-game without explicitly quitting.
func (mm *MatchManager) runCleanup() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		mm.mu.RLock()
		candidates := make([]*Match, 0, len(mm.matches))
		for _, m := range mm.matches {
			candidates = append(candidates, m)
		}
		mm.mu.RUnlock()

		for _, m := range candidates {
			m.mu.Lock()
			if m.Status == StatusFinished {
				m.mu.Unlock()
				continue
			}
			lastActivity := m.LastActivityAt
			if lastActivity.IsZero() {
				lastActivity = m.CreatedAt
			}
			idle := now.Sub(lastActivity)
			ttl := waitingMatchTTL
			if m.Status == StatusActive || m.Status == StatusPrep {
				ttl = activeMatchTTL
			}
			stale := idle > ttl
			matchID := m.ID
			status := m.Status
			m.mu.Unlock()

			if stale {
				mm.mu.Lock()
				delete(mm.matches, matchID)
				mm.mu.Unlock()
				log.Printf("Cleaned up stale %s match %s (idle %v > TTL %v)", status, matchID, idle.Round(time.Second), ttl)
			}
		}
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
				"code":      m.ID,
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

	matchID := generateID(4)

	m := &Match{
		ID:            matchID,
		GameType:      gameType,
		Players:       []string{playerID},
		Engine:        engine,
		Status:        StatusWaiting,
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
		TurnTimeout:    defaultTurnTimeout,
		PrepDuration:   defaultPrepTime,
		ReadyPlayers:   make(map[string]bool),
		ForfeitCount:   make(map[string]int),
		Disconnected:   make(map[string]time.Time),
		GraceTimers:    make(map[string]*time.Timer),
	}

	mm.mu.Lock()
	mm.matches[matchID] = m
	mm.mu.Unlock()

	if err := mm.db.CreateMatch(matchID, gameType, matchID); err != nil {
		log.Printf("Failed to persist match: %v", err)
	}
	if err := mm.db.AddMatchPlayer(matchID, playerID, 0); err != nil {
		log.Printf("Failed to persist match player: %v", err)
	}

	log.Printf("Match created: %s (game: %s, by: %s)", matchID, gameType, playerID)
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
	id := strings.ToUpper(code)
	mm.mu.RLock()
	m, ok := mm.matches[id]
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
	m.LastActivityAt = time.Now()

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
	// Persist event with the spectator game state snapshot so replays can render move-by-move
	mm.db.RecordEvent(m.ID, m.EventSeq, playerID, action.Type, action, map[string]any{
		"result":     result,
		"game_state": spectatorState,
	})
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
	if currentPlayer == "" {
		// Simultaneous game — no single-player timeout
		return
	}
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

	// Persist the final game state so the match history page can render the board
	if finalState != nil {
		stateJSON, err := json.Marshal(finalState)
		if err != nil {
			log.Printf("Failed to marshal final state for match %s: %v", m.ID, err)
		} else if err := mm.db.SaveFinalState(m.ID, string(stateJSON)); err != nil {
			log.Printf("Failed to save final state for match %s: %v", m.ID, err)
		}
	}

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

func waitingFor(view *engines.PlayerView) any {
	if view.YourTurn {
		return nil
	}
	switch view.Phase {
	case "setup":
		return "opponent_setup"
	case "play":
		return "opponent_move"
	case "waiting":
		return "opponent"
	default:
		return "opponent"
	}
}

func (mm *MatchManager) sendPlayerTurn(c *Client, matchID string, view *engines.PlayerView) {
	c.QueueEvent(map[string]any{
		"type":              "your_turn",
		"match_id":          matchID,
		"phase":             view.Phase,
		"your_turn":         view.YourTurn,
		"waiting_for":       waitingFor(view),
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

// findPlayerMatch finds a match containing the given player without holding mm.mu
// while locking individual matches, avoiding deadlock with finishMatch.
func (mm *MatchManager) findPlayerMatch(playerID string, statusFilter ...MatchStatus) *Match {
	// Phase 1: collect candidate match pointers under mm.mu.RLock()
	mm.mu.RLock()
	candidates := make([]*Match, 0, len(mm.matches))
	for _, m := range mm.matches {
		candidates = append(candidates, m)
	}
	mm.mu.RUnlock()

	// Phase 2: check each candidate under m.mu.Lock() (no mm.mu held)
	for _, m := range candidates {
		m.mu.Lock()
		if len(statusFilter) > 0 {
			matched := false
			for _, s := range statusFilter {
				if m.Status == s {
					matched = true
					break
				}
			}
			if !matched {
				m.mu.Unlock()
				continue
			}
		}
		if slices.Contains(m.Players, playerID) {
			m.mu.Unlock()
			return m
		}
		m.mu.Unlock()
	}
	return nil
}

// GetPlayerActiveMatch returns the active match for a player, if any.
func (mm *MatchManager) GetPlayerActiveMatch(playerID string) *Match {
	return mm.findPlayerMatch(playerID, StatusActive, StatusWaiting, StatusPrep)
}

func generateID(n int) string {
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

// Helper to marshal match info for JSON API
func (m *Match) ToJSON() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := map[string]any{
		"id":             m.ID,
		"game_type":      m.GameType,
		"status":         string(m.Status),
		"players":        m.Players,
		"challenge_code": m.ID,
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
			"type":        "game_state",
			"match_id":    m.ID,
			"phase":       string(m.Status),
			"status":      string(m.Status),
			"your_turn":   false,
			"waiting_for": "opponent",
		}, nil
	}

	if !slices.Contains(m.Players, playerID) {
		return nil, fmt.Errorf("you are not a player in this match")
	}

	view := m.Engine.GetPlayerView(m.State, engines.PlayerID(playerID))
	if view == nil {
		return nil, fmt.Errorf("could not get player view")
	}

	playerIdx := 0
	for i, p := range m.Players {
		if p == playerID {
			playerIdx = i + 1
			break
		}
	}

	return map[string]any{
		"type":              "game_state",
		"match_id":          m.ID,
		"phase":             view.Phase,
		"your_turn":         view.YourTurn,
		"waiting_for":       waitingFor(view),
		"simultaneous":      view.Simultaneous,
		"board":             view.Board,
		"available_actions": view.AvailableActions,
		"turn_number":       view.TurnNumber,
		"game_specific":     view.GameSpecific,
		"player_index":      playerIdx,
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
