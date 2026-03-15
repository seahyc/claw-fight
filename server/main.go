package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"html/template"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/claw-fight/server/engines"
	"github.com/claw-fight/server/engines/battleship"
	"github.com/claw-fight/server/engines/poker"
	"github.com/claw-fight/server/engines/prisoners_dilemma"
	"github.com/claw-fight/server/engines/tictactoe"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	hub        *Hub
	matchMgr   *MatchManager
	matchmaker *Matchmaker
	tournMgr   *TournamentManager
	db         *DB
	funcMap    template.FuncMap
}

var baseURL string

func spectatorURL(matchID string) string {
	return baseURL + "/match/" + matchID
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "7429"
	}

	baseURL = os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://clawfight.live"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./claw-fight.db"
	}
	db, err := NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.CleanupStaleMatches(); err != nil {
		log.Printf("Warning: failed to cleanup stale matches: %v", err)
	}

	hub := NewHub()
	go hub.Run()

	matchMgr := NewMatchManager(hub, db)
	matchMgr.RegisterEngine(battleship.New())
	matchMgr.RegisterEngine(poker.New())
	matchMgr.RegisterEngine(prisoners_dilemma.New())
	matchMgr.RegisterEngine(tictactoe.New())

	// Wire up disconnect grace period handler
	hub.mu.Lock()
	hub.disconnectHandler = matchMgr.HandlePlayerDisconnect
	hub.mu.Unlock()

	matchmaker := NewMatchmaker(matchMgr, hub, db)
	tournMgr := NewTournamentManager(db, matchMgr)

	funcMap := template.FuncMap{
		"json": func(v any) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"eq": func(a, b string) bool { return a == b },
	}

	srv := &Server{
		hub:        hub,
		matchMgr:   matchMgr,
		matchmaker: matchmaker,
		tournMgr:   tournMgr,
		db:         db,
		funcMap:    funcMap,
	}

	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("GET /", srv.handleHome)
	mux.HandleFunc("GET /match/{id}", srv.handleMatchPage)
	mux.HandleFunc("GET /player/{id}", srv.handlePlayerPage)
	mux.HandleFunc("GET /play", srv.handlePlayPage)
	mux.HandleFunc("GET /play/{id}", srv.handlePlayMatchPage)

	// Tournament pages
	mux.HandleFunc("GET /tournaments", srv.handleTournamentsPage)
	mux.HandleFunc("GET /tournament/{id}", srv.handleTournamentPage)

	// Tournament API
	mux.HandleFunc("GET /api/tournaments", srv.handleAPITournaments)
	mux.HandleFunc("GET /api/tournament/{id}", srv.handleAPITournament)
	mux.HandleFunc("POST /api/tournament", srv.handleAPICreateTournament)
	mux.HandleFunc("POST /api/tournament/{id}/register", srv.handleAPITournamentRegister)
	mux.HandleFunc("POST /api/tournament/{id}/start", srv.handleAPITournamentStart)

	// REST API for game operations
	mux.HandleFunc("POST /api/register", srv.handleAPIRegister)
	mux.HandleFunc("POST /api/match", srv.handleAPICreateMatch)
	mux.HandleFunc("POST /api/match/join", srv.handleAPIJoinMatch)
	mux.HandleFunc("POST /api/match/find", srv.handleAPIFindMatch)
	mux.HandleFunc("POST /api/match/{id}/action", srv.handleAPIMatchAction)
	mux.HandleFunc("POST /api/match/{id}/ready", srv.handleAPIMatchReady)
	mux.HandleFunc("POST /api/match/{id}/chat", srv.handleAPIMatchChat)
	mux.HandleFunc("POST /api/match/{id}/quit", srv.handleAPIMatchQuit)
	mux.HandleFunc("POST /api/match/{id}/end", srv.handleAPIMatchEnd)
	mux.HandleFunc("GET /api/match/{id}/state", srv.handleAPIMatchState)
	mux.HandleFunc("GET /api/match/{id}/history", srv.handleAPIMatchHistory)
	mux.HandleFunc("GET /api/player/{id}/match", srv.handleAPIPlayerMatch)
	mux.HandleFunc("GET /api/game/{type}/rules", srv.handleAPIGameRules)
	mux.HandleFunc("GET /api/matches/open", srv.handleAPIOpenMatches)

	// Deploy webhook
	mux.HandleFunc("POST /api/deploy", handleAPIDeploy)

	// API
	mux.HandleFunc("GET /api/matches", srv.handleAPIMatches)
	mux.HandleFunc("GET /api/leaderboard", srv.handleAPILeaderboard)
	mux.HandleFunc("GET /api/games", srv.handleAPIGames)

	// WebSocket
	mux.HandleFunc("GET /ws", srv.handleWS)
	mux.HandleFunc("GET /ws/spectate/{matchId}", srv.handleSpectateWS)

	// skill.md endpoint
	mux.HandleFunc("GET /skill.md", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/static/skill.md")
	})

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

var templateFiles = map[string]string{
	"home":        "web/templates/home.html",
	"match":       "web/templates/match.html",
	"play":        "web/templates/play.html",
	"play_match":  "web/templates/play_match.html",
	"player":      "web/templates/player.html",
	"tournaments": "web/templates/tournaments.html",
	"tournament":  "web/templates/tournament.html",
}

func (s *Server) renderPage(w http.ResponseWriter, page string, data map[string]any) {
	pageFile, ok := templateFiles[page]
	if !ok {
		http.Error(w, "page not found", 404)
		return
	}
	tmpl, err := template.New("").Funcs(s.funcMap).ParseFiles("web/templates/layout.html", pageFile)
	if err != nil {
		log.Printf("Template parse error for %s: %v", page, err)
		http.Error(w, "Internal server error", 500)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("Template error for %s: %v", page, err)
		http.Error(w, "Internal server error", 500)
	}
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.renderPage(w, "home", map[string]any{
		"Title": "Home",
	})
}

func (s *Server) handleMatchPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m := s.matchMgr.GetMatch(id)
	var gameType, status string
	if m != nil {
		gameType = m.GameType
		status = string(m.Status)
	} else {
		dbMatch, err := s.db.GetMatch(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		gameType = dbMatch.GameType
		status = dbMatch.Status
	}
	s.renderPage(w, "match", map[string]any{
		"Title":    "Match " + id,
		"MatchID":  id,
		"GameType": gameType,
		"Status":   status,
	})
}

func (s *Server) handlePlayerPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	player, err := s.db.GetPlayer(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.renderPage(w, "player", map[string]any{
		"Title":      player.Name,
		"PlayerID":   player.ID,
		"PlayerName": player.Name,
	})
}

func (s *Server) handlePlayPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "play", map[string]any{
		"Title": "Play",
	})
}

func (s *Server) handlePlayMatchPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m := s.matchMgr.GetMatch(id)
	var gameType, status string
	if m != nil {
		gameType = m.GameType
		status = string(m.Status)
	} else {
		dbMatch, err := s.db.GetMatch(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		gameType = dbMatch.GameType
		status = dbMatch.Status
	}
	s.renderPage(w, "play_match", map[string]any{
		"Title":    "Play - " + id,
		"MatchID":  id,
		"GameType": gameType,
		"Status":   status,
	})
}

func (s *Server) handleAPIMatches(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "completed" {
		limit := 10
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		matches, err := s.db.GetRecentMatches(limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, matches)
		return
	}
	matches, err := s.db.GetActiveMatches()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, matches)
}

func (s *Server) handleAPILeaderboard(w http.ResponseWriter, r *http.Request) {
	gameType := r.URL.Query().Get("game")
	if gameType == "" {
		gameType = "battleship"
	}
	limit := 50
	entries, err := s.db.GetLeaderboard(gameType, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, entries)
}

func (s *Server) handleAPIGames(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.matchMgr.ListGames())
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:     s.hub,
		conn:    conn,
		send:    make(chan []byte, 256),
		eventCh: make(chan struct{}, 1),
	}

	s.hub.register <- client
	go client.WritePump()
	go client.ReadPump(s.handleClientMessage)
}

func (s *Server) handleSpectateWS(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("matchId")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Spectate WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		hub:     s.hub,
		conn:    conn,
		matchID: matchID,
		send:    make(chan []byte, 256),
		eventCh: make(chan struct{}, 1),
	}

	s.hub.register <- client
	s.hub.AddSpectator(matchID, client)
	go client.WritePump()

	// Send current state immediately
	m := s.matchMgr.GetMatch(matchID)
	if m != nil {
		view := s.matchMgr.GetSpectatorView(m)
		view["type"] = "match_state"
		client.SendJSON(view)
	}

	go client.ReadPump(func(c *Client, msg []byte) {
		// Spectators don't send meaningful messages, just keepalive
	})
}

type WSMessage struct {
	Type       string          `json:"type"`
	PlayerID   string          `json:"player_id,omitempty"`
	PlayerName string          `json:"player_name,omitempty"`
	MatchID    string          `json:"match_id,omitempty"`
	GameType   string          `json:"game_type,omitempty"`
	Code       string          `json:"code,omitempty"`
	Options    map[string]any  `json:"options,omitempty"`
	Action     json.RawMessage `json:"action,omitempty"`
	ActionType string          `json:"action_type,omitempty"`
	ActionData map[string]any  `json:"action_data,omitempty"`
	Message    string          `json:"message,omitempty"`
	Scope      string          `json:"scope,omitempty"`
	Types      []string        `json:"types,omitempty"`
	Timeout    int             `json:"timeout,omitempty"` // listen timeout in seconds
}

func (s *Server) handleClientMessage(client *Client, raw []byte) {
	var msg WSMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": "invalid JSON"})
		return
	}

	log.Printf("MSG from %s: type=%s match_id=%s game_type=%s action_type=%s action_data=%v action=%s", client.playerID, msg.Type, msg.MatchID, msg.GameType, msg.ActionType, msg.ActionData, string(msg.Action))

	switch msg.Type {
	case "register":
		s.handleRegister(client, msg)
	case "list_games":
		s.handleListGames(client, msg)
	case "get_rules":
		s.handleGetRules(client, msg)
	case "create_match":
		s.handleCreateMatch(client, msg)
	case "join_match":
		s.handleJoinMatch(client, msg)
	case "find_match":
		s.handleFindMatch(client, msg)
	case "action":
		s.handleActionMsg(client, msg)
	case "get_state":
		s.handleGetState(client, msg)
	case "ready":
		s.handleReady(client, msg)
	case "listen":
		s.handleListen(client, msg)
	case "chat":
		s.handleChat(client, msg)
	case "quit_match":
		s.handleQuitMatch(client, msg)
	case "end_match":
		s.handleEndMatch(client, msg)
	default:
		client.SendJSON(map[string]any{"type": "error", "message": fmt.Sprintf("unknown message type: %s", msg.Type)})
	}
}

var (
	boringNamePrefixes = []string{"claude", "agent", "assistant", "bot", "ai", "model", "llm", "chatbot"}
	funAdjectives      = []string{"CHROME", "NEON", "SHADOW", "IRON", "PIXEL", "COSMIC", "TURBO", "HYPER", "CYBER", "QUANTUM", "THUNDER", "STEALTH", "BLAZING", "ROGUE", "PHANTOM"}
	funNouns           = []string{"VIPER", "GHOST", "FALCON", "WOLF", "PHOENIX", "DRAGON", "TIGER", "COBRA", "HAWK", "LYNX", "RAPTOR", "STORM", "BLADE", "FANG", "SPARK"}
)

func isBoringName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, prefix := range boringNamePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func generateFunName() string {
	ai, _ := rand.Int(rand.Reader, big.NewInt(int64(len(funAdjectives))))
	ni, _ := rand.Int(rand.Reader, big.NewInt(int64(len(funNouns))))
	return funAdjectives[ai.Int64()] + "_" + funNouns[ni.Int64()]
}

func (s *Server) handleRegister(client *Client, msg WSMessage) {
	if len(msg.PlayerName) > 200 {
		client.SendJSON(map[string]any{"type": "error", "message": "player_name must be 200 characters or less"})
		return
	}
	if msg.PlayerID == "" {
		msg.PlayerID = generateID(12)
	}
	if msg.PlayerName == "" || isBoringName(msg.PlayerName) {
		msg.PlayerName = generateFunName()
	}

	s.hub.RegisterPlayer(client, msg.PlayerID)
	s.db.CreatePlayer(msg.PlayerID, msg.PlayerName)

	client.SendJSON(map[string]any{
		"type":      "registered",
		"player_id": msg.PlayerID,
	})
	log.Printf("Player registered: %s (%s)", msg.PlayerID, msg.PlayerName)

	// Check for reconnection to an active match
	s.matchMgr.HandlePlayerReconnect(msg.PlayerID)
}

func (s *Server) handleListGames(client *Client, _ WSMessage) {
	client.SendJSON(map[string]any{
		"type":         "games_list",
		"games":        s.matchMgr.ListGames(),
		"open_matches": s.matchMgr.ListOpenMatches(),
	})
}

func (s *Server) handleGetRules(client *Client, msg WSMessage) {
	engine := s.matchMgr.GetEngine(msg.GameType)
	if engine == nil {
		client.SendJSON(map[string]any{"type": "error", "message": fmt.Sprintf("unknown game type: %s", msg.GameType)})
		return
	}
	client.SendJSON(map[string]any{
		"type":      "rules",
		"game_type": msg.GameType,
		"rules":     engine.DescribeRules(),
	})
}

func (s *Server) handleCreateMatch(client *Client, msg WSMessage) {
	if client.playerID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "must register first"})
		return
	}
	gameType := msg.GameType
	if gameType == "" {
		gameType = "battleship"
	}

	match, err := s.matchMgr.CreateMatch(gameType, client.playerID, msg.Options)
	if err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
		return
	}

	client.matchID = match.ID
	client.SendJSON(map[string]any{
		"type":          "match_created",
		"match_id":      match.ID,
		"code":          match.ID,
		"spectator_url": spectatorURL(match.ID),
	})
}

func (s *Server) handleJoinMatch(client *Client, msg WSMessage) {
	if client.playerID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "must register first"})
		return
	}

	code := strings.ToUpper(msg.Code)
	match, err := s.matchMgr.JoinByCode(code, client.playerID)
	if err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
		return
	}

	client.matchID = match.ID
	client.SendJSON(map[string]any{
		"type":          "match_joined",
		"match_id":      match.ID,
		"spectator_url": spectatorURL(match.ID),
	})
}

func (s *Server) handleFindMatch(client *Client, msg WSMessage) {
	if client.playerID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "must register first"})
		return
	}
	gameType := msg.GameType
	if gameType == "" {
		gameType = "battleship"
	}

	// Try to pair immediately with a waiting player; if none, create a pending
	// match with a shareable code so the player can invite an opponent.
	code, matchID, err := s.matchmaker.EnqueueOrCreate(gameType, client.playerID)
	if err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
		return
	}

	if code != "" {
		// No opponent yet – return a code the player can share
		client.matchID = matchID
		client.SendJSON(map[string]any{
			"type":          "match_queued",
			"match_id":      matchID,
			"code":          code,
			"spectator_url": spectatorURL(matchID),
			"message":       "No opponent found yet. Share the code with your opponent so they can join, or wait for auto-match.",
		})
	}
	// If code == "" an opponent was found immediately; match_found is sent by the matchmaker.
}

func (s *Server) handleActionMsg(client *Client, msg WSMessage) {
	sendErr := func(message string) {
		client.SendJSON(map[string]any{
			"type":     "action_result",
			"match_id": msg.MatchID,
			"success":  false,
			"message":  message,
		})
	}

	if client.playerID == "" {
		sendErr("must register first")
		return
	}
	matchID := msg.MatchID
	if matchID == "" {
		matchID = client.matchID
	}
	if matchID == "" {
		sendErr("no match specified")
		return
	}

	var action engines.Action

	// Support both formats:
	// 1. {action: {type: "...", data: {...}}}  (raw JSON blob)
	// 2. {action_type: "...", action_data: {...}}  (separate fields from MCP client)
	if msg.ActionType != "" {
		action = engines.Action{
			Type: msg.ActionType,
			Data: msg.ActionData,
		}
	} else if len(msg.Action) > 0 {
		if err := json.Unmarshal(msg.Action, &action); err != nil {
			sendErr("invalid action format")
			return
		}
	} else {
		sendErr("missing action")
		return
	}

	if err := s.matchMgr.HandleAction(matchID, client.playerID, action); err != nil {
		// Error already sent to client in HandleAction as action_result
		log.Printf("Action error: %v", err)
	}
}

func (s *Server) handleGetState(client *Client, msg WSMessage) {
	matchID := msg.MatchID
	if matchID == "" {
		matchID = client.matchID
	}
	if matchID == "" {
		client.SendJSON(map[string]any{"type": "game_state", "match_id": "", "error": "no match specified"})
		return
	}

	state, err := s.matchMgr.GetState(matchID, client.playerID)
	if err != nil {
		client.SendJSON(map[string]any{"type": "game_state", "match_id": matchID, "error": err.Error()})
		return
	}
	client.SendJSON(state)
}

func (s *Server) handleReady(client *Client, msg WSMessage) {
	matchID := msg.MatchID
	if matchID == "" {
		matchID = client.matchID
	}
	if matchID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "no match specified"})
		return
	}

	if err := s.matchMgr.PlayerReady(matchID, client.playerID); err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
	}
}

func (s *Server) handleTournamentsPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "tournaments", map[string]any{
		"Title": "Tournaments",
	})
}

func (s *Server) handleTournamentPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t := s.tournMgr.GetTournament(id)
	if t == nil {
		http.NotFound(w, r)
		return
	}
	s.renderPage(w, "tournament", map[string]any{
		"Title":          t.Name,
		"TournamentID":   t.ID,
		"TournamentName": t.Name,
		"GameType":       t.GameType,
		"Format":         t.Format,
	})
}

func (s *Server) handleAPITournaments(w http.ResponseWriter, r *http.Request) {
	tournaments := s.tournMgr.ListTournaments()
	type tournamentListItem struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		GameType    string `json:"game_type"`
		Format      string `json:"format"`
		Status      string `json:"status"`
		PlayerCount int    `json:"player_count"`
		CreatedAt   string `json:"created_at"`
	}
	var items []tournamentListItem
	for _, t := range tournaments {
		items = append(items, tournamentListItem{
			ID:          t.ID,
			Name:        t.Name,
			GameType:    t.GameType,
			Format:      t.Format,
			Status:      string(t.Status),
			PlayerCount: len(t.Entries),
			CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	writeJSON(w, items)
}

func (s *Server) handleAPITournament(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t := s.tournMgr.GetTournament(id)
	if t == nil {
		http.Error(w, "tournament not found", 404)
		return
	}
	standings := s.tournMgr.GetStandings(id)
	writeJSON(w, map[string]any{
		"tournament": t,
		"standings":  standings,
	})
}

func (s *Server) handleAPICreateTournament(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string           `json:"name"`
		GameType string           `json:"game_type"`
		Format   string           `json:"format"`
		Config   TournamentConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	if req.Name == "" || req.GameType == "" {
		http.Error(w, "name and game_type are required", 400)
		return
	}
	if req.Format == "" {
		req.Format = TournamentSwiss
	}

	t, err := s.tournMgr.CreateTournament(req.Name, req.GameType, req.Format, req.Config)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(201)
	writeJSON(w, t)
}

func (s *Server) handleAPITournamentRegister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	if err := s.tournMgr.RegisterPlayer(id, req.PlayerID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]string{"status": "registered"})
}

func (s *Server) handleAPITournamentStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.tournMgr.StartTournament(id); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t := s.tournMgr.GetTournament(id)
	writeJSON(w, t)
}

func (s *Server) handleListen(client *Client, msg WSMessage) {
	if client.playerID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "must register first"})
		return
	}

	// Cancel any previous listen
	client.mu.Lock()
	if client.listenCancel != nil {
		close(client.listenCancel)
	}
	cancel := make(chan struct{})
	client.listenCancel = cancel
	client.mu.Unlock()

	// Run in goroutine so ReadPump isn't blocked
	go func() {
		timeout := 5 * time.Minute
		if msg.Timeout > 0 && msg.Timeout <= 300 {
			timeout = time.Duration(msg.Timeout) * time.Second
		}

		events := client.DrainEvents(msg.MatchID, msg.Types)
		if len(events) == 0 {
			select {
			case <-client.eventCh:
			case <-time.After(timeout):
			case <-cancel:
				return
			}
			events = client.DrainEvents(msg.MatchID, msg.Types)
		}

		if len(events) == 0 {
			events = []map[string]any{}
		}

		client.SendJSON(map[string]any{
			"type":   "events",
			"events": events,
		})
	}()
}

func (s *Server) handleChat(client *Client, msg WSMessage) {
	if client.playerID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "must register first"})
		return
	}
	if len(msg.Message) > 500 {
		client.SendJSON(map[string]any{"type": "error", "message": "message must be 500 characters or less"})
		return
	}

	matchID := msg.MatchID
	if matchID == "" {
		matchID = client.matchID
	}
	if matchID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "no match specified"})
		return
	}

	m := s.matchMgr.GetMatch(matchID)
	if m == nil {
		client.SendJSON(map[string]any{"type": "error", "message": "match not found"})
		return
	}

	m.mu.Lock()
	if !slices.Contains(m.Players, client.playerID) {
		m.mu.Unlock()
		client.SendJSON(map[string]any{"type": "error", "message": "you are not in this match"})
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
		"from":     client.playerID,
		"message":  msg.Message,
		"scope":    msg.Scope,
	}

	// Queue to other players
	for _, p := range players {
		if p != client.playerID {
			if c := s.hub.GetClientByPlayer(p); c != nil {
				c.QueueEvent(chatEvent)
			}
		}
	}

	// Broadcast to spectators
	s.hub.BroadcastToSpectators(matchID, chatEvent)

	// Record in match_events
	s.db.RecordEvent(matchID, seq, client.playerID, "chat", msg.Message, nil)

	client.SendJSON(map[string]any{"type": "chat_sent"})
}

func (s *Server) handleQuitMatch(client *Client, msg WSMessage) {
	if client.playerID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "must register first"})
		return
	}
	matchID := msg.MatchID
	if matchID == "" {
		matchID = client.matchID
	}
	if matchID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "no match specified"})
		return
	}

	if err := s.matchMgr.QuitMatch(matchID, client.playerID); err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
		return
	}

	client.matchID = ""
	client.SendJSON(map[string]any{
		"type":     "match_quit",
		"match_id": matchID,
	})
}

func (s *Server) handleEndMatch(client *Client, msg WSMessage) {
	if client.playerID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "must register first"})
		return
	}
	matchID := msg.MatchID
	if matchID == "" {
		matchID = client.matchID
	}
	if matchID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "no match specified"})
		return
	}

	if err := s.matchMgr.EndMatch(matchID, client.playerID); err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
		return
	}

	client.matchID = ""
	client.SendJSON(map[string]any{
		"type":     "match_ended",
		"match_id": matchID,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
