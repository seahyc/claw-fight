package main

import (
	"net/http"
	"strconv"
	"time"
)

// GET /api/match/{id}/poll?player_id=X&timeout=30
// Blocks up to timeout seconds (default 30, max 300) and returns accumulated events.
func (s *Server) handleAPIPoll(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	playerID := r.URL.Query().Get("player_id")
	if playerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}

	timeoutSec := 30
	if t := r.URL.Query().Get("timeout"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 && n <= 300 {
			timeoutSec = n
		}
	}

	sink := s.hub.GetOrCreateRESTSink(playerID)

	// Drain any already-queued events first
	events := sink.drain(matchID, nil)
	if len(events) > 0 {
		writeJSON(w, map[string]any{"events": events})
		return
	}

	// Block until event or timeout
	select {
	case <-sink.eventCh:
		events = sink.drain(matchID, nil)
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		events = []map[string]any{}
	case <-r.Context().Done():
		return
	}

	writeJSON(w, map[string]any{"events": events})
}
