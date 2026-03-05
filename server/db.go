package main

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type Player struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type ELORating struct {
	PlayerID    string `json:"player_id"`
	GameType    string `json:"game_type"`
	Rating      int    `json:"rating"`
	GamesPlayed int    `json:"games_played"`
}

type MatchRecord struct {
	ID            string     `json:"id"`
	GameType      string     `json:"game_type"`
	Status        string     `json:"status"`
	ChallengeCode string    `json:"challenge_code"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	WinnerID      string     `json:"winner_id,omitempty"`
}

type MatchEvent struct {
	MatchID    string `json:"match_id"`
	Seq        int    `json:"seq"`
	PlayerID   string `json:"player_id"`
	ActionType string `json:"action_type"`
	ActionJSON string `json:"action_json"`
	ResultJSON string `json:"result_json"`
	Timestamp  time.Time `json:"timestamp"`
}

type LeaderboardEntry struct {
	PlayerID    string `json:"player_id"`
	PlayerName  string `json:"player_name"`
	GameType    string `json:"game_type"`
	Rating      int    `json:"rating"`
	GamesPlayed int    `json:"games_played"`
}

func NewDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	db := &DB{conn: conn}
	if err := db.createTables(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS players (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS elo_ratings (
		player_id TEXT NOT NULL,
		game_type TEXT NOT NULL,
		rating INTEGER DEFAULT 1200,
		games_played INTEGER DEFAULT 0,
		PRIMARY KEY (player_id, game_type)
	);

	CREATE TABLE IF NOT EXISTS matches (
		id TEXT PRIMARY KEY,
		game_type TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'waiting',
		challenge_code TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		started_at DATETIME,
		ended_at DATETIME,
		winner_id TEXT
	);

	CREATE TABLE IF NOT EXISTS match_players (
		match_id TEXT NOT NULL,
		player_id TEXT NOT NULL,
		seat INTEGER NOT NULL,
		result TEXT,
		PRIMARY KEY (match_id, player_id)
	);

	CREATE TABLE IF NOT EXISTS match_events (
		match_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		player_id TEXT,
		action_type TEXT NOT NULL,
		action_json TEXT,
		result_json TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (match_id, seq)
	);

	CREATE TABLE IF NOT EXISTS tournaments (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		game_type TEXT NOT NULL,
		format TEXT NOT NULL DEFAULT 'swiss',
		config_json TEXT,
		status TEXT NOT NULL DEFAULT 'open',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tournament_entries (
		tournament_id TEXT NOT NULL,
		player_id TEXT NOT NULL,
		seed INTEGER,
		wins INTEGER DEFAULT 0,
		losses INTEGER DEFAULT 0,
		draws INTEGER DEFAULT 0,
		points REAL DEFAULT 0,
		PRIMARY KEY (tournament_id, player_id)
	);

	CREATE TABLE IF NOT EXISTS tournament_rounds (
		tournament_id TEXT NOT NULL,
		round INTEGER NOT NULL,
		match_id TEXT,
		player1_id TEXT,
		player2_id TEXT,
		winner_id TEXT,
		status TEXT DEFAULT 'pending',
		PRIMARY KEY (tournament_id, round, match_id)
	);

	CREATE INDEX IF NOT EXISTS idx_matches_status ON matches(status);
	CREATE INDEX IF NOT EXISTS idx_matches_code ON matches(challenge_code);
	CREATE INDEX IF NOT EXISTS idx_elo_game ON elo_ratings(game_type, rating DESC);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) CreatePlayer(id, name string) error {
	_, err := db.conn.Exec("INSERT OR IGNORE INTO players (id, name) VALUES (?, ?)", id, name)
	return err
}

func (db *DB) GetPlayer(id string) (*Player, error) {
	p := &Player{}
	err := db.conn.QueryRow("SELECT id, name, created_at FROM players WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (db *DB) GetOrCreateELO(playerID, gameType string) (*ELORating, error) {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO elo_ratings (player_id, game_type, rating, games_played) VALUES (?, ?, ?, 0)",
		playerID, gameType, startingRating,
	)
	if err != nil {
		return nil, err
	}
	r := &ELORating{}
	err = db.conn.QueryRow(
		"SELECT player_id, game_type, rating, games_played FROM elo_ratings WHERE player_id = ? AND game_type = ?",
		playerID, gameType,
	).Scan(&r.PlayerID, &r.GameType, &r.Rating, &r.GamesPlayed)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (db *DB) UpdateELO(playerID, gameType string, newRating, gamesPlayed int) error {
	_, err := db.conn.Exec(
		"UPDATE elo_ratings SET rating = ?, games_played = ? WHERE player_id = ? AND game_type = ?",
		newRating, gamesPlayed, playerID, gameType,
	)
	return err
}

func (db *DB) CreateMatch(id, gameType, challengeCode string) error {
	_, err := db.conn.Exec(
		"INSERT INTO matches (id, game_type, status, challenge_code) VALUES (?, ?, 'waiting', ?)",
		id, gameType, challengeCode,
	)
	return err
}

func (db *DB) AddMatchPlayer(matchID, playerID string, seat int) error {
	_, err := db.conn.Exec(
		"INSERT INTO match_players (match_id, player_id, seat) VALUES (?, ?, ?)",
		matchID, playerID, seat,
	)
	return err
}

func (db *DB) UpdateMatchStatus(matchID, status string) error {
	_, err := db.conn.Exec("UPDATE matches SET status = ? WHERE id = ?", status, matchID)
	return err
}

func (db *DB) StartMatch(matchID string) error {
	_, err := db.conn.Exec(
		"UPDATE matches SET status = 'active', started_at = ? WHERE id = ?",
		time.Now(), matchID,
	)
	return err
}

func (db *DB) EndMatch(matchID, winnerID string) error {
	_, err := db.conn.Exec(
		"UPDATE matches SET status = 'finished', ended_at = ?, winner_id = ? WHERE id = ?",
		time.Now(), winnerID, matchID,
	)
	return err
}

func (db *DB) RecordEvent(matchID string, seq int, playerID, actionType string, action, result any) error {
	actionJSON, _ := json.Marshal(action)
	resultJSON, _ := json.Marshal(result)
	_, err := db.conn.Exec(
		"INSERT INTO match_events (match_id, seq, player_id, action_type, action_json, result_json) VALUES (?, ?, ?, ?, ?, ?)",
		matchID, seq, playerID, actionType, string(actionJSON), string(resultJSON),
	)
	return err
}

func (db *DB) GetLeaderboard(gameType string, limit int) ([]LeaderboardEntry, error) {
	rows, err := db.conn.Query(`
		SELECT e.player_id, p.name, e.game_type, e.rating, e.games_played
		FROM elo_ratings e JOIN players p ON e.player_id = p.id
		WHERE e.game_type = ? ORDER BY e.rating DESC LIMIT ?
	`, gameType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(&e.PlayerID, &e.PlayerName, &e.GameType, &e.Rating, &e.GamesPlayed); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (db *DB) GetActiveMatches() ([]MatchRecord, error) {
	rows, err := db.conn.Query(`
		SELECT id, game_type, status, challenge_code, created_at, started_at, ended_at, COALESCE(winner_id, '')
		FROM matches WHERE status IN ('waiting', 'prep', 'active') ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []MatchRecord
	for rows.Next() {
		var m MatchRecord
		if err := rows.Scan(&m.ID, &m.GameType, &m.Status, &m.ChallengeCode, &m.CreatedAt, &m.StartedAt, &m.EndedAt, &m.WinnerID); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, nil
}

func (db *DB) GetMatchByCode(code string) (*MatchRecord, error) {
	m := &MatchRecord{}
	err := db.conn.QueryRow(
		"SELECT id, game_type, status, challenge_code, created_at FROM matches WHERE challenge_code = ? AND status = 'waiting'",
		code,
	).Scan(&m.ID, &m.GameType, &m.Status, &m.ChallengeCode, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (db *DB) GetMatch(id string) (*MatchRecord, error) {
	m := &MatchRecord{}
	err := db.conn.QueryRow(
		"SELECT id, game_type, status, challenge_code, created_at, started_at, ended_at, COALESCE(winner_id, '') FROM matches WHERE id = ?",
		id,
	).Scan(&m.ID, &m.GameType, &m.Status, &m.ChallengeCode, &m.CreatedAt, &m.StartedAt, &m.EndedAt, &m.WinnerID)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (db *DB) GetMatchEvents(matchID string) ([]MatchEvent, error) {
	rows, err := db.conn.Query(
		"SELECT match_id, seq, player_id, action_type, action_json, result_json, timestamp FROM match_events WHERE match_id = ? ORDER BY seq",
		matchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []MatchEvent
	for rows.Next() {
		var e MatchEvent
		if err := rows.Scan(&e.MatchID, &e.Seq, &e.PlayerID, &e.ActionType, &e.ActionJSON, &e.ResultJSON, &e.Timestamp); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func (db *DB) GetMatchHistory(playerID string, limit int) ([]MatchRecord, error) {
	rows, err := db.conn.Query(`
		SELECT m.id, m.game_type, m.status, m.challenge_code, m.created_at, m.started_at, m.ended_at, COALESCE(m.winner_id, '')
		FROM matches m JOIN match_players mp ON m.id = mp.match_id
		WHERE mp.player_id = ? ORDER BY m.created_at DESC LIMIT ?
	`, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []MatchRecord
	for rows.Next() {
		var m MatchRecord
		if err := rows.Scan(&m.ID, &m.GameType, &m.Status, &m.ChallengeCode, &m.CreatedAt, &m.StartedAt, &m.EndedAt, &m.WinnerID); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, nil
}

func (db *DB) CreateTournament(t *Tournament) error {
	configJSON, _ := json.Marshal(t.Config)
	_, err := db.conn.Exec(
		"INSERT INTO tournaments (id, name, game_type, format, config_json, status) VALUES (?, ?, ?, ?, ?, ?)",
		t.ID, t.Name, t.GameType, t.Format, string(configJSON), string(t.Status),
	)
	return err
}

func (db *DB) GetTournament(id string) (*Tournament, error) {
	t := &Tournament{}
	var configJSON string
	var status string
	err := db.conn.QueryRow(
		"SELECT id, name, game_type, format, COALESCE(config_json, '{}'), status, created_at FROM tournaments WHERE id = ?", id,
	).Scan(&t.ID, &t.Name, &t.GameType, &t.Format, &configJSON, &status, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	t.Status = TournamentStatus(status)
	json.Unmarshal([]byte(configJSON), &t.Config)

	// Load entries
	rows, err := db.conn.Query(
		"SELECT player_id, COALESCE(seed, 0), COALESCE(wins, 0), COALESCE(losses, 0), COALESCE(draws, 0), COALESCE(points, 0) FROM tournament_entries WHERE tournament_id = ? ORDER BY seed",
		id,
	)
	if err != nil {
		return t, nil
	}
	defer rows.Close()
	for rows.Next() {
		var e TournamentEntry
		rows.Scan(&e.PlayerID, &e.Seed, &e.Wins, &e.Losses, &e.Draws, &e.Points)
		t.Entries = append(t.Entries, e)
	}

	// Load rounds
	rrows, err := db.conn.Query(
		"SELECT round, match_id, COALESCE(player1_id, ''), COALESCE(player2_id, ''), COALESCE(winner_id, ''), COALESCE(status, 'pending') FROM tournament_rounds WHERE tournament_id = ? ORDER BY round, match_id",
		id,
	)
	if err != nil {
		return t, nil
	}
	defer rrows.Close()
	roundMap := make(map[int]*TournamentRound)
	for rrows.Next() {
		var roundNum int
		var m TournamentMatch
		rrows.Scan(&roundNum, &m.MatchID, &m.Player1, &m.Player2, &m.Winner, &m.Status)
		if roundMap[roundNum] == nil {
			roundMap[roundNum] = &TournamentRound{Number: roundNum}
		}
		roundMap[roundNum].Matches = append(roundMap[roundNum].Matches, m)
	}
	for i := 1; i <= len(roundMap); i++ {
		if r, ok := roundMap[i]; ok {
			t.Rounds = append(t.Rounds, *r)
		}
	}

	return t, nil
}

func (db *DB) ListTournaments() ([]Tournament, error) {
	rows, err := db.conn.Query(
		"SELECT id, name, game_type, format, COALESCE(config_json, '{}'), status, created_at FROM tournaments ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tournaments []Tournament
	for rows.Next() {
		var t Tournament
		var configJSON, status string
		if err := rows.Scan(&t.ID, &t.Name, &t.GameType, &t.Format, &configJSON, &status, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Status = TournamentStatus(status)
		json.Unmarshal([]byte(configJSON), &t.Config)

		// Get player count
		var count int
		db.conn.QueryRow("SELECT COUNT(*) FROM tournament_entries WHERE tournament_id = ?", t.ID).Scan(&count)
		t.Entries = make([]TournamentEntry, count)

		tournaments = append(tournaments, t)
	}
	return tournaments, nil
}

func (db *DB) RegisterTournamentPlayer(tournID, playerID string, seed int) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO tournament_entries (tournament_id, player_id, seed) VALUES (?, ?, ?)",
		tournID, playerID, seed,
	)
	return err
}

func (db *DB) UpdateTournamentStatus(tournID, status string) error {
	_, err := db.conn.Exec("UPDATE tournaments SET status = ? WHERE id = ?", status, tournID)
	return err
}

func (db *DB) RecordTournamentRound(tournID string, round int, matchID, player1, player2, winner, status string) error {
	_, err := db.conn.Exec(
		`INSERT OR REPLACE INTO tournament_rounds (tournament_id, round, match_id, player1_id, player2_id, winner_id, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tournID, round, matchID, player1, player2, winner, status,
	)
	return err
}

func (db *DB) GetTournamentStandings(tournID string) ([]TournamentEntry, error) {
	rows, err := db.conn.Query(
		"SELECT player_id, COALESCE(seed, 0), COALESCE(wins, 0), COALESCE(losses, 0), COALESCE(draws, 0), COALESCE(points, 0) FROM tournament_entries WHERE tournament_id = ? ORDER BY points DESC, wins DESC",
		tournID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []TournamentEntry
	for rows.Next() {
		var e TournamentEntry
		rows.Scan(&e.PlayerID, &e.Seed, &e.Wins, &e.Losses, &e.Draws, &e.Points)
		entries = append(entries, e)
	}
	return entries, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}
