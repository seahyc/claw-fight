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

	// Broadcast to spectators
	actionText := action.Type
	if result.Message != "" {
		actionText += " - " + result.Message
	}
	mm.hub.BroadcastToSpectators(m.ID, map[string]any{
		"type":        "action",
		"match_id":    m.ID,
		"player":      playerID,
		"action_type": action.Type,
		"text":        actionText,
		"result":      result,
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
	mm.hub.BroadcastToSpectators(m.ID, map[string]any{
		"type":     "game_over",
		"match_id": m.ID,
		"result":   result,
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

	views := make(map[string]any)
	for _, p := range m.Players {
		views[p] = m.Engine.GetPlayerView(m.State, engines.PlayerID(p))
	}

	// Determine current turn as player index (1 or 2)
	currentTurnIdx := 0
	for i, p := range m.Players {
		if p == string(m.State.CurrentTurn) {
			currentTurnIdx = i + 1
			break
		}
	}

	return map[string]any{
		"match_id":     m.ID,
		"game_type":    m.GameType,
		"status":       string(m.Status),
		"players":      playerInfos,
		"player_views": views,
		"game_state":   views,
		"turn_number":  m.State.TurnNumber,
		"current_turn": currentTurnIdx,
	}
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

// MarshalAction parses an action from a raw JSON message
func MarshalAction(data json.RawMessage) (engines.Action, error) {
	var action engines.Action
	if err := json.Unmarshal(data, &action); err != nil {
		return action, err
	}
	return action, nil
}
