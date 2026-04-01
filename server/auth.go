package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// authSecret is used to sign tokens. Falls back to a hardcoded dev secret if not set.
var authSecret = func() []byte {
	if s := os.Getenv("AUTH_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("dev-secret-change-in-production")
}()

// signToken creates an HMAC-signed token: "clw_<playerID>_<expiry>_<sig>"
func signToken(playerID string, expiry time.Time) string {
	payload := fmt.Sprintf("%s:%d", playerID, expiry.Unix())
	mac := hmac.New(sha256.New, authSecret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("clw_%s_%d_%s", playerID, expiry.Unix(), sig)
}

// verifyToken validates a token and returns the playerID, or an error.
func verifyToken(token string) (string, error) {
	if !strings.HasPrefix(token, "clw_") {
		return "", fmt.Errorf("invalid token format")
	}
	// Format: clw_<playerID>_<expiry>_<sig>
	// playerID may contain underscores, so split from the right
	parts := strings.Split(token[4:], "_") // strip "clw_" prefix
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid token format")
	}
	sig := parts[len(parts)-1]
	expiryStr := parts[len(parts)-2]
	playerID := strings.Join(parts[:len(parts)-2], "_")

	var expiry int64
	if _, err := fmt.Sscanf(expiryStr, "%d", &expiry); err != nil {
		return "", fmt.Errorf("invalid token expiry")
	}
	if time.Now().Unix() > expiry {
		return "", fmt.Errorf("token expired")
	}

	payload := fmt.Sprintf("%s:%d", playerID, expiry)
	mac := hmac.New(sha256.New, authSecret)
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", fmt.Errorf("invalid token signature")
	}
	return playerID, nil
}

// POST /api/auth/token — issue a 24h token for a player
func (s *Server) handleAPIAuthToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
		http.Error(w, "player_id is required", 400)
		return
	}
	// Verify player exists
	if _, err := s.db.GetPlayer(req.PlayerID); err != nil {
		http.Error(w, "player not found", 404)
		return
	}
	expiry := time.Now().Add(24 * time.Hour)
	token := signToken(req.PlayerID, expiry)
	writeJSON(w, map[string]any{
		"token":      token,
		"expires_at": expiry.UTC().Format(time.RFC3339),
	})
}

// playerIDFromRequest extracts the player ID from either:
// 1. Authorization: Bearer clw_xxx header (token auth)
// 2. player_id query parameter
// Returns "" if neither is present (caller decides if that's an error)
func playerIDFromRequest(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := auth[7:]
		if playerID, err := verifyToken(token); err == nil {
			return playerID
		}
	}
	return r.URL.Query().Get("player_id")
}
