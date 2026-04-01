package main

import (
	"html/template"
	"log"
	"net/http"
)

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
