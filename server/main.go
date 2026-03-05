package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/claw-fight/server/engines"
	"github.com/claw-fight/server/engines/battleship"
	"github.com/claw-fight/server/engines/poker"
	"github.com/claw-fight/server/engines/prisoners_dilemma"
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
	pages      map[string]*template.Template
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	db, err := NewDB("./claw-fight.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	hub := NewHub()
	go hub.Run()

	matchMgr := NewMatchManager(hub, db)
	matchMgr.RegisterEngine(battleship.New())
	matchMgr.RegisterEngine(poker.New())
	matchMgr.RegisterEngine(prisoners_dilemma.New())

	matchmaker := NewMatchmaker(matchMgr, hub, db)
	tournMgr := NewTournamentManager(db, matchMgr)

	funcMap := template.FuncMap{
		"json": func(v any) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"eq": func(a, b string) bool { return a == b },
	}

	pages := map[string]*template.Template{
		"home":        template.Must(template.New("").Funcs(funcMap).ParseFiles("web/templates/layout.html", "web/templates/home.html")),
		"match":       template.Must(template.New("").Funcs(funcMap).ParseFiles("web/templates/layout.html", "web/templates/match.html")),
		"player":      template.Must(template.New("").Funcs(funcMap).ParseFiles("web/templates/layout.html", "web/templates/player.html")),
		"tournaments": template.Must(template.New("").Funcs(funcMap).ParseFiles("web/templates/layout.html", "web/templates/tournaments.html")),
		"tournament":  template.Must(template.New("").Funcs(funcMap).ParseFiles("web/templates/layout.html", "web/templates/tournament.html")),
	}

	srv := &Server{
		hub:        hub,
		matchMgr:   matchMgr,
		matchmaker: matchmaker,
		tournMgr:   tournMgr,
		db:         db,
		pages:      pages,
	}

	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("GET /", srv.handleHome)
	mux.HandleFunc("GET /match/{id}", srv.handleMatchPage)
	mux.HandleFunc("GET /player/{id}", srv.handlePlayerPage)

	// Tournament pages
	mux.HandleFunc("GET /tournaments", srv.handleTournamentsPage)
	mux.HandleFunc("GET /tournament/{id}", srv.handleTournamentPage)

	// Tournament API
	mux.HandleFunc("GET /api/tournaments", srv.handleAPITournaments)
	mux.HandleFunc("GET /api/tournament/{id}", srv.handleAPITournament)
	mux.HandleFunc("POST /api/tournament", srv.handleAPICreateTournament)
	mux.HandleFunc("POST /api/tournament/{id}/register", srv.handleAPITournamentRegister)
	mux.HandleFunc("POST /api/tournament/{id}/start", srv.handleAPITournamentStart)

	// API
	mux.HandleFunc("GET /api/matches", srv.handleAPIMatches)
	mux.HandleFunc("GET /api/leaderboard", srv.handleAPILeaderboard)
	mux.HandleFunc("GET /api/games", srv.handleAPIGames)

	// WebSocket
	mux.HandleFunc("GET /ws", srv.handleWS)
	mux.HandleFunc("GET /ws/spectate/{matchId}", srv.handleSpectateWS)

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func (s *Server) renderPage(w http.ResponseWriter, page string, data map[string]any) {
	tmpl, ok := s.pages[page]
	if !ok {
		http.Error(w, "page not found", 404)
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

func (s *Server) handleAPIMatches(w http.ResponseWriter, r *http.Request) {
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
		hub:  s.hub,
		conn: conn,
		send: make(chan []byte, 256),
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
	Type     string          `json:"type"`
	PlayerID string          `json:"player_id,omitempty"`
	PlayerName string        `json:"player_name,omitempty"`
	MatchID  string          `json:"match_id,omitempty"`
	GameType string          `json:"game_type,omitempty"`
	Code     string          `json:"code,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
	Action   json.RawMessage `json:"action,omitempty"`
}

func (s *Server) handleClientMessage(client *Client, raw []byte) {
	var msg WSMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": "invalid JSON"})
		return
	}

	switch msg.Type {
	case "register":
		s.handleRegister(client, msg)
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
	default:
		client.SendJSON(map[string]any{"type": "error", "message": fmt.Sprintf("unknown message type: %s", msg.Type)})
	}
}

func (s *Server) handleRegister(client *Client, msg WSMessage) {
	if msg.PlayerID == "" {
		msg.PlayerID = generateID(12)
	}
	if msg.PlayerName == "" {
		msg.PlayerName = "Agent-" + msg.PlayerID[:6]
	}

	s.hub.RegisterPlayer(client, msg.PlayerID)
	s.db.CreatePlayer(msg.PlayerID, msg.PlayerName)

	client.SendJSON(map[string]any{
		"type":      "registered",
		"player_id": msg.PlayerID,
	})
	log.Printf("Player registered: %s (%s)", msg.PlayerID, msg.PlayerName)
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
		"code":          match.ChallengeCode,
		"spectator_url": "/match/" + match.ID,
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
		"spectator_url": "/match/" + match.ID,
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

	if err := s.matchmaker.Enqueue(gameType, client.playerID); err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
	}
}

func (s *Server) handleActionMsg(client *Client, msg WSMessage) {
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

	var action engines.Action
	if err := json.Unmarshal(msg.Action, &action); err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": "invalid action format"})
		return
	}

	if err := s.matchMgr.HandleAction(matchID, client.playerID, action); err != nil {
		// Error already sent to client in HandleAction
		log.Printf("Action error: %v", err)
	}
}

func (s *Server) handleGetState(client *Client, msg WSMessage) {
	matchID := msg.MatchID
	if matchID == "" {
		matchID = client.matchID
	}
	if matchID == "" {
		client.SendJSON(map[string]any{"type": "error", "message": "no match specified"})
		return
	}

	state, err := s.matchMgr.GetState(matchID, client.playerID)
	if err != nil {
		client.SendJSON(map[string]any{"type": "error", "message": err.Error()})
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
