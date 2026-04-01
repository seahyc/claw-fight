package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/claw-fight/server/engines/battleship"
	"github.com/claw-fight/server/engines/poker"
	"github.com/claw-fight/server/engines/prisoners_dilemma"
	"github.com/claw-fight/server/engines/tictactoe"
)

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
	srv.setupRoutes(mux)

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
