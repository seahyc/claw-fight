package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/claw-fight/server/engines"
)

// POST /api/register
func (s *Server) handleAPIRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerName string `json:"player_name"`
		PlayerID   string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}

	if len(req.PlayerName) > 200 {
		http.Error(w, "player_name must be 200 characters or less", 400)
		return
	}

	if req.PlayerID == "" {
		req.PlayerID = generateID(12)
	}
	if req.PlayerName == "" || isBoringName(req.PlayerName) {
		req.PlayerName = generateFunName()
	}

	s.db.CreatePlayer(req.PlayerID, req.PlayerName)

	// If there's an active WS client for this player, register with hub
	if c := s.hub.GetClientByPlayer(req.PlayerID); c != nil {
		s.hub.RegisterPlayer(c, req.PlayerID)
	}

	writeJSON(w, map[string]any{
		"player_id":   req.PlayerID,
		"player_name": req.PlayerName,
	})
	log.Printf("REST: Player registered: %s (%s)", req.PlayerID, req.PlayerName)
}

// POST /api/match
func (s *Server) handleAPICreateMatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GameType string         `json:"game_type"`
		PlayerID string         `json:"player_id"`
		Options  map[string]any `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}
	if req.GameType == "" {
		req.GameType = "battleship"
	}

	match, err := s.matchMgr.CreateMatch(req.GameType, req.PlayerID, req.Options)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	writeJSON(w, map[string]any{
		"match_id":      match.ID,
		"code":          match.ChallengeCode,
		"spectator_url": spectatorURL(match.ID),
	})
}

// POST /api/match/join
func (s *Server) handleAPIJoinMatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		PlayerID string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" || req.Code == "" {
		http.Error(w, "player_id and code are required", 400)
		return
	}

	code := strings.ToUpper(req.Code)
	match, err := s.matchMgr.JoinByCode(code, req.PlayerID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	writeJSON(w, map[string]any{
		"match_id":      match.ID,
		"spectator_url": spectatorURL(match.ID),
	})
}

// POST /api/match/find
func (s *Server) handleAPIFindMatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GameType string `json:"game_type"`
		PlayerID string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}
	if req.GameType == "" {
		req.GameType = "battleship"
	}

	code, matchID, err := s.matchmaker.EnqueueOrCreate(req.GameType, req.PlayerID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if code != "" {
		writeJSON(w, map[string]any{
			"match_id":      matchID,
			"code":          code,
			"spectator_url": spectatorURL(matchID),
			"status":        "waiting",
		})
	} else {
		// Matched with existing open match or queued player
		writeJSON(w, map[string]any{
			"match_id":      matchID,
			"spectator_url": spectatorURL(matchID),
			"status":        "matched",
		})
	}
}

// POST /api/match/{id}/action
func (s *Server) handleAPIMatchAction(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var req struct {
		PlayerID   string         `json:"player_id"`
		ActionType string         `json:"action_type"`
		ActionData map[string]any `json:"action_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" || req.ActionType == "" {
		http.Error(w, "player_id and action_type are required", 400)
		return
	}

	action := engines.Action{
		Type: req.ActionType,
		Data: req.ActionData,
	}

	// Use HandleAction which does all side effects (event recording, opponent
	// notification, game over check, turn timer management). The action_result
	// is also sent to the WS client if connected; we return the result via HTTP.
	err := s.matchMgr.HandleAction(matchID, req.PlayerID, action)
	if err != nil {
		writeJSON(w, map[string]any{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, map[string]any{
		"success": true,
		"message": "action applied",
	})
}

// GET /api/match/{id}/state?player_id=X
func (s *Server) handleAPIMatchState(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	playerID := r.URL.Query().Get("player_id")
	if playerID == "" {
		http.Error(w, "player_id query parameter is required", 400)
		return
	}

	state, err := s.matchMgr.GetState(matchID, playerID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, state)
}

// POST /api/match/{id}/ready
func (s *Server) handleAPIMatchReady(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var req struct {
		PlayerID string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}

	if err := s.matchMgr.PlayerReady(matchID, req.PlayerID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// POST /api/match/{id}/chat
func (s *Server) handleAPIMatchChat(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var req struct {
		PlayerID string `json:"player_id"`
		Message  string `json:"message"`
		Scope    string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}
	if len(req.Message) > 500 {
		http.Error(w, "message must be 500 characters or less", 400)
		return
	}

	m := s.matchMgr.GetMatch(matchID)
	if m == nil {
		http.Error(w, "match not found", 404)
		return
	}

	m.mu.Lock()
	if !slices.Contains(m.Players, req.PlayerID) {
		m.mu.Unlock()
		http.Error(w, "you are not in this match", 403)
		return
	}
	players := make([]string, len(m.Players))
	copy(players, m.Players)
	m.EventSeq++
	seq := m.EventSeq
	m.mu.Unlock()

	chatEvent := map[string]any{
		"type":     "chat",
		"match_id": matchID,
		"from":     req.PlayerID,
		"message":  req.Message,
		"scope":    req.Scope,
	}

	for _, p := range players {
		if p != req.PlayerID {
			if c := s.hub.GetClientByPlayer(p); c != nil {
				c.QueueEvent(chatEvent)
			}
		}
	}

	s.hub.BroadcastToSpectators(matchID, chatEvent)
	s.db.RecordEvent(matchID, seq, req.PlayerID, "chat", req.Message, nil)

	writeJSON(w, map[string]any{"ok": true})
}

// POST /api/match/{id}/quit
func (s *Server) handleAPIMatchQuit(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var req struct {
		PlayerID string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}

	if err := s.matchMgr.QuitMatch(matchID, req.PlayerID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// POST /api/match/{id}/end
func (s *Server) handleAPIMatchEnd(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var req struct {
		PlayerID string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.PlayerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}

	if err := s.matchMgr.EndMatch(matchID, req.PlayerID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// GET /api/player/{id}/match
func (s *Server) handleAPIPlayerMatch(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("id")
	m := s.matchMgr.GetPlayerActiveMatch(playerID)
	if m == nil {
		http.Error(w, "no active match", 404)
		return
	}
	m.mu.Lock()
	resp := map[string]any{
		"match_id":  m.ID,
		"game_type": m.GameType,
		"status":    string(m.Status),
	}
	m.mu.Unlock()
	writeJSON(w, resp)
}

// GET /api/game/{type}/rules
func (s *Server) handleAPIGameRules(w http.ResponseWriter, r *http.Request) {
	gameType := r.PathValue("type")
	engine := s.matchMgr.GetEngine(gameType)
	if engine == nil {
		http.Error(w, fmt.Sprintf("unknown game type: %s", gameType), 404)
		return
	}
	writeJSON(w, map[string]any{
		"game_type": gameType,
		"rules":     engine.DescribeRules(),
	})
}

// GET /api/matches/open
func (s *Server) handleAPIOpenMatches(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.matchMgr.ListOpenMatches())
}

// POST /api/deploy — GitHub webhook endpoint for auto-deploy
func handleAPIDeploy(w http.ResponseWriter, r *http.Request) {
	secret := os.Getenv("DEPLOY_SECRET")
	if secret == "" {
		log.Printf("Deploy: DEPLOY_SECRET not configured")
		http.Error(w, "deploy not configured", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Verify HMAC-SHA256 signature
	sigHeader := r.Header.Get("X-Hub-Signature-256")
	if sigHeader == "" || !strings.HasPrefix(sigHeader, "sha256=") {
		http.Error(w, "missing or invalid signature", http.StatusUnauthorized)
		return
	}
	sigHex := strings.TrimPrefix(sigHeader, "sha256=")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		http.Error(w, "invalid signature encoding", http.StatusUnauthorized)
		return
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(sigBytes, expected) {
		log.Printf("Deploy: signature mismatch")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Parse payload to check ref
	var payload struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	if payload.Ref != "refs/heads/main" {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, map[string]string{"status": "ignored", "reason": "not main branch"})
		return
	}

	log.Printf("Deploy: valid webhook for main branch, starting deploy...")

	// Run git pull
	gitPull := exec.Command("git", "pull")
	gitPull.Dir = "."
	if out, err := gitPull.CombinedOutput(); err != nil {
		log.Printf("Deploy: git pull failed: %v\n%s", err, out)
		http.Error(w, "git pull failed", http.StatusInternalServerError)
		return
	} else {
		log.Printf("Deploy: git pull output: %s", out)
	}

	// Run go build
	goBuild := exec.Command("go", "build", "-o", "claw-fight", ".")
	goBuild.Dir = "."
	if out, err := goBuild.CombinedOutput(); err != nil {
		log.Printf("Deploy: go build failed: %v\n%s", err, out)
		http.Error(w, "go build failed", http.StatusInternalServerError)
		return
	} else {
		log.Printf("Deploy: go build output: %s", out)
	}

	// Respond before exiting
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{"status": "deploying"})

	// Flush the response
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	log.Printf("Deploy: build complete, exiting for systemd restart...")
	os.Exit(0)
}
