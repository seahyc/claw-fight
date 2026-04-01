package main

import (
	"encoding/json"
	"net/http"
)

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

