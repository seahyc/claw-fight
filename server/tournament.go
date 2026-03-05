package main

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

const (
	TournamentSwiss     = "swiss"
	TournamentRoundRobin = "round_robin"
)

type TournamentStatus string

const (
	TournStatusOpen     TournamentStatus = "open"
	TournStatusActive   TournamentStatus = "active"
	TournStatusFinished TournamentStatus = "finished"
)

type Tournament struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	GameType  string           `json:"game_type"`
	Format    string           `json:"format"`
	Status    TournamentStatus `json:"status"`
	Config    TournamentConfig `json:"config"`
	Entries   []TournamentEntry `json:"entries"`
	Rounds    []TournamentRound `json:"rounds"`
	CreatedAt time.Time        `json:"created_at"`
}

type TournamentConfig struct {
	MaxPlayers   int            `json:"max_players"`
	SwissRounds  int            `json:"swiss_rounds"`
	MatchOptions map[string]any `json:"match_options,omitempty"`
}

type TournamentEntry struct {
	PlayerID string  `json:"player_id"`
	Seed     int     `json:"seed"`
	Wins     int     `json:"wins"`
	Losses   int     `json:"losses"`
	Draws    int     `json:"draws"`
	Points   float64 `json:"points"`
}

type TournamentRound struct {
	Number  int               `json:"number"`
	Matches []TournamentMatch `json:"matches"`
}

type TournamentMatch struct {
	MatchID string `json:"match_id"`
	Player1 string `json:"player1"`
	Player2 string `json:"player2"`
	Winner  string `json:"winner"`
	Status  string `json:"status"`
}

type TournamentManager struct {
	mu          sync.RWMutex
	tournaments map[string]*Tournament
	db          *DB
	matchMgr    *MatchManager
}

func NewTournamentManager(db *DB, matchMgr *MatchManager) *TournamentManager {
	return &TournamentManager{
		tournaments: make(map[string]*Tournament),
		db:          db,
		matchMgr:    matchMgr,
	}
}

func (tm *TournamentManager) CreateTournament(name, gameType, format string, config TournamentConfig) (*Tournament, error) {
	if format != TournamentSwiss && format != TournamentRoundRobin {
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
	if tm.matchMgr.GetEngine(gameType) == nil {
		return nil, fmt.Errorf("unknown game type: %s", gameType)
	}
	if config.MaxPlayers < 2 {
		config.MaxPlayers = 16
	}
	if format == TournamentSwiss && config.SwissRounds < 1 {
		config.SwissRounds = 5
	}

	t := &Tournament{
		ID:        generateID(10),
		Name:      name,
		GameType:  gameType,
		Format:    format,
		Status:    TournStatusOpen,
		Config:    config,
		CreatedAt: time.Now(),
	}

	tm.mu.Lock()
	tm.tournaments[t.ID] = t
	tm.mu.Unlock()

	if err := tm.db.CreateTournament(t); err != nil {
		log.Printf("Failed to persist tournament: %v", err)
	}

	log.Printf("Tournament created: %s (%s, %s, %s)", t.ID, t.Name, t.GameType, t.Format)
	return t, nil
}

func (tm *TournamentManager) RegisterPlayer(tournID, playerID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	t, ok := tm.tournaments[tournID]
	if !ok {
		return fmt.Errorf("tournament not found: %s", tournID)
	}
	if t.Status != TournStatusOpen {
		return fmt.Errorf("tournament is not accepting registrations")
	}
	for _, e := range t.Entries {
		if e.PlayerID == playerID {
			return fmt.Errorf("player already registered")
		}
	}
	if len(t.Entries) >= t.Config.MaxPlayers {
		return fmt.Errorf("tournament is full")
	}

	entry := TournamentEntry{
		PlayerID: playerID,
		Seed:     len(t.Entries) + 1,
	}
	t.Entries = append(t.Entries, entry)

	if err := tm.db.RegisterTournamentPlayer(tournID, playerID, entry.Seed); err != nil {
		log.Printf("Failed to persist tournament entry: %v", err)
	}

	return nil
}

func (tm *TournamentManager) StartTournament(tournID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	t, ok := tm.tournaments[tournID]
	if !ok {
		return fmt.Errorf("tournament not found: %s", tournID)
	}
	if t.Status != TournStatusOpen {
		return fmt.Errorf("tournament already started or finished")
	}
	if len(t.Entries) < 2 {
		return fmt.Errorf("need at least 2 players")
	}

	t.Status = TournStatusActive

	if t.Format == TournamentRoundRobin {
		tm.generateRoundRobinPairings(t)
	} else {
		tm.generateSwissRound(t)
	}

	if err := tm.db.UpdateTournamentStatus(tournID, string(TournStatusActive)); err != nil {
		log.Printf("Failed to update tournament status: %v", err)
	}
	tm.persistRounds(t)

	log.Printf("Tournament %s started with %d players", t.ID, len(t.Entries))
	return nil
}

func (tm *TournamentManager) GenerateNextRound(tournID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	t, ok := tm.tournaments[tournID]
	if !ok {
		return fmt.Errorf("tournament not found: %s", tournID)
	}
	if t.Status != TournStatusActive {
		return fmt.Errorf("tournament is not active")
	}
	if t.Format == TournamentRoundRobin {
		return fmt.Errorf("round-robin pairings are generated upfront")
	}

	// Check that current round is complete
	if len(t.Rounds) > 0 {
		lastRound := t.Rounds[len(t.Rounds)-1]
		for _, m := range lastRound.Matches {
			if m.Status != "finished" {
				return fmt.Errorf("current round is not complete")
			}
		}
	}

	if len(t.Rounds) >= t.Config.SwissRounds {
		t.Status = TournStatusFinished
		if err := tm.db.UpdateTournamentStatus(tournID, string(TournStatusFinished)); err != nil {
			log.Printf("Failed to update tournament status: %v", err)
		}
		return fmt.Errorf("all swiss rounds completed")
	}

	tm.generateSwissRound(t)
	tm.persistRounds(t)
	return nil
}

func (tm *TournamentManager) RecordMatchResult(tournID, matchID, winnerID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	t, ok := tm.tournaments[tournID]
	if !ok {
		return fmt.Errorf("tournament not found: %s", tournID)
	}

	found := false
	for i := range t.Rounds {
		for j := range t.Rounds[i].Matches {
			m := &t.Rounds[i].Matches[j]
			if m.MatchID == matchID {
				m.Winner = winnerID
				m.Status = "finished"
				found = true

				// Update entry stats
				for k := range t.Entries {
					if t.Entries[k].PlayerID == m.Player1 {
						if winnerID == m.Player1 {
							t.Entries[k].Wins++
							t.Entries[k].Points += 1
						} else if winnerID == "" {
							t.Entries[k].Draws++
							t.Entries[k].Points += 0.5
						} else {
							t.Entries[k].Losses++
						}
					}
					if t.Entries[k].PlayerID == m.Player2 {
						if winnerID == m.Player2 {
							t.Entries[k].Wins++
							t.Entries[k].Points += 1
						} else if winnerID == "" {
							t.Entries[k].Draws++
							t.Entries[k].Points += 0.5
						} else {
							t.Entries[k].Losses++
						}
					}
				}
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("match %s not found in tournament %s", matchID, tournID)
	}

	// Check if tournament is complete
	if tm.isTournamentComplete(t) {
		t.Status = TournStatusFinished
		if err := tm.db.UpdateTournamentStatus(tournID, string(TournStatusFinished)); err != nil {
			log.Printf("Failed to update tournament status: %v", err)
		}
	}

	tm.persistRounds(t)
	return nil
}

func (tm *TournamentManager) GetStandings(tournID string) []TournamentEntry {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	t, ok := tm.tournaments[tournID]
	if !ok {
		return nil
	}

	standings := make([]TournamentEntry, len(t.Entries))
	copy(standings, t.Entries)

	sort.Slice(standings, func(i, j int) bool {
		if standings[i].Points != standings[j].Points {
			return standings[i].Points > standings[j].Points
		}
		return standings[i].Wins > standings[j].Wins
	})

	return standings
}

func (tm *TournamentManager) GetTournament(tournID string) *Tournament {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tournaments[tournID]
}

func (tm *TournamentManager) ListTournaments() []*Tournament {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var list []*Tournament
	for _, t := range tm.tournaments {
		list = append(list, t)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	return list
}

// Swiss pairing: sort by points descending, pair adjacent players.
// Avoid re-pairing. Odd player count gives lowest-ranked a bye.
func (tm *TournamentManager) generateSwissRound(t *Tournament) {
	roundNum := len(t.Rounds) + 1

	// Build sorted list by points (descending)
	sorted := make([]TournamentEntry, len(t.Entries))
	copy(sorted, t.Entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Points > sorted[j].Points
	})

	// Track previous pairings
	played := make(map[string]map[string]bool)
	for _, r := range t.Rounds {
		for _, m := range r.Matches {
			if played[m.Player1] == nil {
				played[m.Player1] = make(map[string]bool)
			}
			if played[m.Player2] == nil {
				played[m.Player2] = make(map[string]bool)
			}
			played[m.Player1][m.Player2] = true
			played[m.Player2][m.Player1] = true
		}
	}

	round := TournamentRound{Number: roundNum}
	paired := make(map[string]bool)

	// Handle bye for odd number of players
	if len(sorted)%2 == 1 {
		// Give bye to lowest-ranked unpaired player (who hasn't had a bye)
		byePlayer := sorted[len(sorted)-1].PlayerID
		paired[byePlayer] = true

		// Record bye as a win
		for k := range t.Entries {
			if t.Entries[k].PlayerID == byePlayer {
				t.Entries[k].Wins++
				t.Entries[k].Points += 1
				break
			}
		}

		round.Matches = append(round.Matches, TournamentMatch{
			MatchID: generateID(8),
			Player1: byePlayer,
			Player2: "BYE",
			Winner:  byePlayer,
			Status:  "finished",
		})
	}

	// Pair adjacent unpaired players, avoiding rematches
	for i := 0; i < len(sorted); i++ {
		p1 := sorted[i].PlayerID
		if paired[p1] {
			continue
		}
		for j := i + 1; j < len(sorted); j++ {
			p2 := sorted[j].PlayerID
			if paired[p2] {
				continue
			}
			if played[p1] != nil && played[p1][p2] {
				continue
			}
			// Pair them
			matchID := generateID(8)
			round.Matches = append(round.Matches, TournamentMatch{
				MatchID: matchID,
				Player1: p1,
				Player2: p2,
				Status:  "pending",
			})
			paired[p1] = true
			paired[p2] = true
			break
		}
	}

	// If any player couldn't avoid a rematch, pair them anyway
	var unpaired []string
	for _, e := range sorted {
		if !paired[e.PlayerID] {
			unpaired = append(unpaired, e.PlayerID)
		}
	}
	for i := 0; i+1 < len(unpaired); i += 2 {
		matchID := generateID(8)
		round.Matches = append(round.Matches, TournamentMatch{
			MatchID: matchID,
			Player1: unpaired[i],
			Player2: unpaired[i+1],
			Status:  "pending",
		})
	}

	t.Rounds = append(t.Rounds, round)
}

// Round-robin: generate all pairings upfront using circle method.
func (tm *TournamentManager) generateRoundRobinPairings(t *Tournament) {
	players := make([]string, len(t.Entries))
	for i, e := range t.Entries {
		players[i] = e.PlayerID
	}

	n := len(players)
	// If odd number, add a dummy "BYE" player
	if n%2 == 1 {
		players = append(players, "BYE")
		n++
	}

	numRounds := n - 1
	for r := 0; r < numRounds; r++ {
		round := TournamentRound{Number: r + 1}
		for i := 0; i < n/2; i++ {
			p1 := players[i]
			p2 := players[n-1-i]

			if p1 == "BYE" || p2 == "BYE" {
				// This player gets a bye
				realPlayer := p1
				if realPlayer == "BYE" {
					realPlayer = p2
				}
				round.Matches = append(round.Matches, TournamentMatch{
					MatchID: generateID(8),
					Player1: realPlayer,
					Player2: "BYE",
					Winner:  realPlayer,
					Status:  "finished",
				})
				// Record bye win
				for k := range t.Entries {
					if t.Entries[k].PlayerID == realPlayer {
						t.Entries[k].Wins++
						t.Entries[k].Points += 1
						break
					}
				}
				continue
			}

			matchID := generateID(8)
			round.Matches = append(round.Matches, TournamentMatch{
				MatchID: matchID,
				Player1: p1,
				Player2: p2,
				Status:  "pending",
			})
		}
		t.Rounds = append(t.Rounds, round)

		// Rotate: fix first player, rotate rest
		last := players[n-1]
		copy(players[2:], players[1:n-1])
		players[1] = last
	}
}

func (tm *TournamentManager) isTournamentComplete(t *Tournament) bool {
	if t.Format == TournamentSwiss {
		if len(t.Rounds) < t.Config.SwissRounds {
			return false
		}
	}
	for _, r := range t.Rounds {
		for _, m := range r.Matches {
			if m.Status != "finished" {
				return false
			}
		}
	}
	return true
}

func (tm *TournamentManager) persistRounds(t *Tournament) {
	for _, r := range t.Rounds {
		for _, m := range r.Matches {
			if err := tm.db.RecordTournamentRound(t.ID, r.Number, m.MatchID, m.Player1, m.Player2, m.Winner, m.Status); err != nil {
				log.Printf("Failed to persist tournament round: %v", err)
			}
		}
	}
}
