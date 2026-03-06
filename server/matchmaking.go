package main

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

type QueueEntry struct {
	PlayerID  string
	Rating    int
	EnqueueAt time.Time
}

type MatchmakingQueue struct {
	mu      sync.Mutex
	entries []QueueEntry
}

type Matchmaker struct {
	mu     sync.RWMutex
	queues map[string]*MatchmakingQueue // gameType -> queue
	mm     *MatchManager
	hub    *Hub
	db     *DB
}

func NewMatchmaker(mm *MatchManager, hub *Hub, db *DB) *Matchmaker {
	m := &Matchmaker{
		queues: make(map[string]*MatchmakingQueue),
		mm:     mm,
		hub:    hub,
		db:     db,
	}
	go m.runLoop()
	return m
}

func (m *Matchmaker) Enqueue(gameType, playerID string) error {
	engine := m.mm.GetEngine(gameType)
	if engine == nil {
		return fmt.Errorf("unknown game type: %s", gameType)
	}

	elo, err := m.db.GetOrCreateELO(playerID, gameType)
	if err != nil {
		return fmt.Errorf("failed to get rating: %w", err)
	}

	m.mu.Lock()
	if m.queues[gameType] == nil {
		m.queues[gameType] = &MatchmakingQueue{}
	}
	q := m.queues[gameType]
	m.mu.Unlock()

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if already in queue
	for _, e := range q.entries {
		if e.PlayerID == playerID {
			return fmt.Errorf("already in queue")
		}
	}

	q.entries = append(q.entries, QueueEntry{
		PlayerID:  playerID,
		Rating:    elo.Rating,
		EnqueueAt: time.Now(),
	})

	log.Printf("Player %s queued for %s (rating: %d)", playerID, gameType, elo.Rating)

	// Notify player
	if c := m.hub.GetClientByPlayer(playerID); c != nil {
		c.SendJSON(map[string]any{
			"type":    "waiting",
			"message": "Searching for opponent...",
		})
	}

	return nil
}

// EnqueueOrCreate tries to immediately pair with a waiting player.
// If successful, it triggers match_found for both and returns ("", "", nil).
// If no match, it creates a pending match with a shareable code and returns (code, matchID, nil).
func (m *Matchmaker) EnqueueOrCreate(gameType, playerID string) (code string, matchID string, err error) {
	engine := m.mm.GetEngine(gameType)
	if engine == nil {
		return "", "", fmt.Errorf("unknown game type: %s", gameType)
	}

	elo, err := m.db.GetOrCreateELO(playerID, gameType)
	if err != nil {
		return "", "", fmt.Errorf("failed to get rating: %w", err)
	}

	m.mu.Lock()
	if m.queues[gameType] == nil {
		m.queues[gameType] = &MatchmakingQueue{}
	}
	q := m.queues[gameType]
	m.mu.Unlock()

	q.mu.Lock()

	// Check for an existing waiting player to pair with
	for i, e := range q.entries {
		ratingDiff := math.Abs(float64(e.Rating - elo.Rating))
		if ratingDiff <= 500 { // generous range for immediate pair
			opponent := e.PlayerID
			q.entries = append(q.entries[:i], q.entries[i+1:]...)
			q.mu.Unlock()
			go m.createMatchForPair(gameType, opponent, playerID)
			return "", "", nil
		}
	}

	q.mu.Unlock()

	// Check in-memory open matches before creating a new one
	openMatches := m.mm.ListOpenMatches()
	var oldest map[string]any
	var oldestTime time.Time
	for _, om := range openMatches {
		if om["game_type"] != gameType {
			continue
		}
		mid := om["match_id"].(string)
		mm := m.mm.GetMatch(mid)
		if mm == nil {
			continue
		}
		mm.mu.Lock()
		created := mm.CreatedAt
		mm.mu.Unlock()
		if oldest == nil || created.Before(oldestTime) {
			oldest = om
			oldestTime = created
		}
	}
	if oldest != nil {
		mid := oldest["match_id"].(string)
		joined, err := m.mm.JoinMatch(mid, playerID)
		if err == nil {
			log.Printf("Player %s joined existing open match %s via automatch", playerID, mid)
			// Notify both players
			for _, pid := range []string{playerID} {
				if c := m.hub.GetClientByPlayer(pid); c != nil {
					c.SendJSON(map[string]any{
						"type":          "match_found",
						"match_id":      joined.ID,
						"game_type":     gameType,
						"spectator_url": spectatorURL(joined.ID),
						"message":       "Match found! Game will start shortly.",
					})
				}
			}
			return "", joined.ID, nil
		}
		log.Printf("Failed to join open match %s: %v, creating new", mid, err)
	}

	// No match found – create a pending match with a code
	match, err := m.mm.CreateMatch(gameType, playerID, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create match: %w", err)
	}

	log.Printf("Player %s queued for %s (rating: %d)", playerID, gameType, elo.Rating)
	return match.ChallengeCode, match.ID, nil
}

func (m *Matchmaker) runLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.RLock()
		gameTypes := make([]string, 0, len(m.queues))
		for gt := range m.queues {
			gameTypes = append(gameTypes, gt)
		}
		m.mu.RUnlock()

		for _, gt := range gameTypes {
			m.tryMatch(gt)
		}
	}
}

func (m *Matchmaker) tryMatch(gameType string) {
	m.mu.RLock()
	q := m.queues[gameType]
	m.mu.RUnlock()
	if q == nil {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) < 2 {
		return
	}

	// Try to find a pair within acceptable ELO range
	// Range expands over time: starts at 100, grows by 50 per 10 seconds
	for i := 0; i < len(q.entries); i++ {
		for j := i + 1; j < len(q.entries); j++ {
			a := q.entries[i]
			b := q.entries[j]

			waitA := time.Since(a.EnqueueAt).Seconds()
			waitB := time.Since(b.EnqueueAt).Seconds()
			maxWait := math.Max(waitA, waitB)

			allowedDiff := 100.0 + (maxWait/10.0)*50.0
			ratingDiff := math.Abs(float64(a.Rating - b.Rating))

			if ratingDiff <= allowedDiff {
				// Remove both from queue
				q.entries = append(q.entries[:j], q.entries[j+1:]...)
				q.entries = append(q.entries[:i], q.entries[i+1:]...)

				go m.createMatchForPair(gameType, a.PlayerID, b.PlayerID)
				return
			}
		}
	}
}

func (m *Matchmaker) createMatchForPair(gameType, player1, player2 string) {
	match, err := m.mm.CreateMatch(gameType, player1, nil)
	if err != nil {
		log.Printf("Matchmaking: failed to create match: %v", err)
		return
	}

	if _, err := m.mm.JoinMatch(match.ID, player2); err != nil {
		log.Printf("Matchmaking: failed to join match: %v", err)
		return
	}

	log.Printf("Matchmaking: paired %s vs %s in match %s", player1, player2, match.ID)

	// Notify both players
	for _, pid := range []string{player1, player2} {
		if c := m.hub.GetClientByPlayer(pid); c != nil {
			c.SendJSON(map[string]any{
				"type":          "match_found",
				"match_id":      match.ID,
				"game_type":     gameType,
				"spectator_url": spectatorURL(match.ID),
				"message":       "Match found! Game will start shortly. Use wait_for_turn to get your first game state, then use perform_action to play.",
			})
		}
	}
}
