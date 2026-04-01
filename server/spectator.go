package main

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

// buildSpectatorGameState delegates to the engine's GetSpectatorView.
// Caller must hold m.mu.
func (mm *MatchManager) buildSpectatorGameState(m *Match) map[string]any {
	if m.State == nil || m.Engine == nil {
		return nil
	}
	return m.Engine.GetSpectatorView(m.State)
}

func (mm *MatchManager) broadcastSpectatorState(m *Match) {
	// NOTE: caller must already hold m.mu - use getSpectatorViewLocked
	view := mm.getSpectatorViewLocked(m)
	view["type"] = "match_state"
	mm.hub.BroadcastToSpectators(m.ID, view)
}
