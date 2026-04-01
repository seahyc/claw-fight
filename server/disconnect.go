package main

import (
	"fmt"
	"log"
	"time"

	"github.com/claw-fight/server/engines"
)

// HandlePlayerDisconnect is called by the hub when a WebSocket client disconnects.
// Instead of immediately forfeiting, it starts a grace period.
func (mm *MatchManager) HandlePlayerDisconnect(playerID string) {
	activeMatch := mm.findPlayerMatch(playerID, StatusActive, StatusWaiting, StatusPrep)

	if activeMatch == nil {
		return
	}

	activeMatch.mu.Lock()

	if activeMatch.Status == StatusWaiting {
		// No opponent yet — close the match immediately rather than leaving it
		// as a ghost in the matchmaking pool.
		matchID := activeMatch.ID
		activeMatch.Status = StatusFinished
		activeMatch.mu.Unlock()
		mm.mu.Lock()
		delete(mm.matches, matchID)
		mm.mu.Unlock()
		log.Printf("Closed waiting match %s: creator %s disconnected", matchID, playerID)
		return
	}

	defer activeMatch.mu.Unlock()

	if activeMatch.Status != StatusActive {
		// Prep phase disconnect — just note it, game hasn't started
		return
	}

	now := time.Now()
	activeMatch.Disconnected[playerID] = now

	// Pause turn timer if it's this player's turn
	if activeMatch.CurrentTurn == playerID && activeMatch.TurnTimer != nil {
		activeMatch.TurnTimer.Stop()
		// Estimate remaining time (we don't track start precisely, so use full timeout as safe default)
		activeMatch.TurnPausedFor = playerID
		activeMatch.TurnPausedLeft = activeMatch.TurnTimeout / 2 // conservative estimate
	}

	// Notify opponent
	for _, pid := range activeMatch.Players {
		if pid != playerID {
			if c := mm.hub.GetClientByPlayer(pid); c != nil {
				c.QueueEvent(map[string]any{
					"type":          "opponent_disconnected",
					"match_id":      activeMatch.ID,
					"message":       "Your opponent has disconnected. They have 5 minutes to reconnect.",
					"grace_seconds": int(disconnectGrace.Seconds()),
				})
			}
		}
	}

	matchID := activeMatch.ID
	log.Printf("Player %s disconnected from match %s, starting %v grace period", playerID, matchID, disconnectGrace)

	// Start grace timer
	activeMatch.GraceTimers[playerID] = time.AfterFunc(disconnectGrace, func() {
		mm.handleGraceExpired(matchID, playerID)
	})
}

// handleGraceExpired forfeits the match for the disconnected player.
func (mm *MatchManager) handleGraceExpired(matchID, playerID string) {
	mm.mu.RLock()
	m, ok := mm.matches[matchID]
	mm.mu.RUnlock()
	if !ok {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if they reconnected in the meantime
	if _, disconnected := m.Disconnected[playerID]; !disconnected {
		return
	}
	if m.Status != StatusActive {
		return
	}

	// Grace period expired — forfeit
	var winner string
	for _, p := range m.Players {
		if p != playerID {
			winner = p
			break
		}
	}

	log.Printf("Grace period expired for player %s in match %s, forfeiting", playerID, matchID)
	mm.finishMatch(m, &engines.GameResult{
		Finished: true,
		Winner:   engines.PlayerID(winner),
		Reason:   fmt.Sprintf("%s forfeited (disconnected for 5m)", playerID),
	})
}

// HandlePlayerReconnect restores a disconnected player to their active match.
func (mm *MatchManager) HandlePlayerReconnect(playerID string) {
	activeMatch := mm.findPlayerMatch(playerID, StatusActive)

	if activeMatch == nil {
		return
	}

	activeMatch.mu.Lock()
	defer activeMatch.mu.Unlock()

	// Verify player is actually disconnected (may have changed between find and lock)
	if _, disconnected := activeMatch.Disconnected[playerID]; !disconnected {
		return
	}

	// Cancel grace timer
	if timer, ok := activeMatch.GraceTimers[playerID]; ok {
		timer.Stop()
		delete(activeMatch.GraceTimers, playerID)
	}
	delete(activeMatch.Disconnected, playerID)

	log.Printf("Player %s reconnected to match %s", playerID, activeMatch.ID)

	// Notify opponent
	for _, pid := range activeMatch.Players {
		if pid != playerID {
			if c := mm.hub.GetClientByPlayer(pid); c != nil {
				c.QueueEvent(map[string]any{
					"type":     "opponent_reconnected",
					"match_id": activeMatch.ID,
					"message":  "Your opponent has reconnected.",
				})
			}
		}
	}

	// Resume turn timer if it was paused for this player
	if activeMatch.TurnPausedFor == playerID {
		activeMatch.TurnPausedFor = ""
		mm.startTurnTimer(activeMatch)
	}

	// Send current game state to reconnected player
	if c := mm.hub.GetClientByPlayer(playerID); c != nil {
		c.matchID = activeMatch.ID
		if activeMatch.State != nil {
			view := activeMatch.Engine.GetPlayerView(activeMatch.State, engines.PlayerID(playerID))
			mm.sendPlayerTurn(c, activeMatch.ID, view)
		}
	}
}
