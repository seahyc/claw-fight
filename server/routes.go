package main

import (
	"encoding/json"
	"net/http"
)

func (s *Server) setupRoutes(mux *http.ServeMux) {
	// Pages
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("GET /match/{id}", s.handleMatchPage)
	mux.HandleFunc("GET /player/{id}", s.handlePlayerPage)
	mux.HandleFunc("GET /play", s.handlePlayPage)
	mux.HandleFunc("GET /play/{id}", s.handlePlayMatchPage)

	// Tournament pages
	mux.HandleFunc("GET /tournaments", s.handleTournamentsPage)
	mux.HandleFunc("GET /tournament/{id}", s.handleTournamentPage)

	// Tournament API
	mux.HandleFunc("GET /api/tournaments", s.handleAPITournaments)
	mux.HandleFunc("GET /api/tournament/{id}", s.handleAPITournament)
	mux.HandleFunc("POST /api/tournament", s.handleAPICreateTournament)
	mux.HandleFunc("POST /api/tournament/{id}/register", s.handleAPITournamentRegister)
	mux.HandleFunc("POST /api/tournament/{id}/start", s.handleAPITournamentStart)

	// REST API for game operations
	mux.HandleFunc("POST /api/register", s.handleAPIRegister)
	mux.HandleFunc("POST /api/match", s.handleAPICreateMatch)
	mux.HandleFunc("POST /api/match/join", s.handleAPIJoinMatch)
	mux.HandleFunc("POST /api/match/find", s.handleAPIFindMatch)
	mux.HandleFunc("POST /api/match/{id}/action", s.handleAPIMatchAction)
	mux.HandleFunc("POST /api/match/{id}/ready", s.handleAPIMatchReady)
	mux.HandleFunc("POST /api/match/{id}/chat", s.handleAPIMatchChat)
	mux.HandleFunc("POST /api/match/{id}/quit", s.handleAPIMatchQuit)
	mux.HandleFunc("POST /api/match/{id}/end", s.handleAPIMatchEnd)
	mux.HandleFunc("GET /api/match/{id}/poll", s.handleAPIPoll)
	mux.HandleFunc("GET /api/match/{id}/state", s.handleAPIMatchState)
	mux.HandleFunc("GET /api/match/{id}/history", s.handleAPIMatchHistory)
	mux.HandleFunc("GET /api/player/{id}/match", s.handleAPIPlayerMatch)
	mux.HandleFunc("GET /api/game/{type}/rules", s.handleAPIGameRules)
	mux.HandleFunc("GET /api/matches/open", s.handleAPIOpenMatches)

	// Auth
	mux.HandleFunc("POST /api/auth/token", s.handleAPIAuthToken)

	// Deploy webhook
	mux.HandleFunc("POST /api/deploy", handleAPIDeploy)

	// API
	mux.HandleFunc("GET /api/matches", s.handleAPIMatches)
	mux.HandleFunc("GET /api/leaderboard", s.handleAPILeaderboard)
	mux.HandleFunc("GET /api/games", s.handleAPIGames)

	// WebSocket
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /ws/spectate/{matchId}", s.handleSpectateWS)

	// skill.md endpoint
	mux.HandleFunc("GET /skill.md", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/static/skill.md")
	})

	// Static files — short cache to avoid stale JS/CSS
	staticFS := http.StripPrefix("/static/", http.FileServer(http.Dir("web/static")))
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=60")
		staticFS.ServeHTTP(w, r)
	}))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
