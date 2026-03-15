package tictactoe

import (
	"fmt"

	"github.com/claw-fight/server/engines"
)

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Name() string   { return "tictactoe" }
func (e *Engine) MinPlayers() int { return 2 }
func (e *Engine) MaxPlayers() int { return 2 }

func (e *Engine) DescribeRules() string {
	return "Tic Tac Toe on a 3x3 grid. Player 1 is X, Player 2 is O. " +
		"Players alternate turns placing their mark on an empty cell. " +
		"Action: mark with data {\"position\": 0-8} where 0=top-left, 8=bottom-right (left-to-right, top-to-bottom). " +
		"First to get three in a row (horizontal, vertical, or diagonal) wins. " +
		"If all 9 cells are filled with no winner, it's a draw."
}

func (e *Engine) InitGame(players []engines.PlayerID, options map[string]any) (*engines.GameState, error) {
	if len(players) != 2 {
		return nil, fmt.Errorf("tictactoe requires exactly 2 players")
	}

	// Initialize empty 3x3 board as [][]string
	board := make([]any, 3)
	for i := range 3 {
		row := make([]any, 3)
		for j := range 3 {
			row[j] = ""
		}
		board[i] = row
	}

	return &engines.GameState{
		Phase:      "play",
		TurnNumber: 0,
		Data: map[string]any{
			"board":      board,
			"move_count": 0,
		},
		Players:     players,
		CurrentTurn: players[0], // Player 1 (X) goes first
	}, nil
}

func (e *Engine) ValidateAction(state *engines.GameState, player engines.PlayerID, action engines.Action) error {
	if action.Type != "mark" {
		return fmt.Errorf("invalid action type: %s, must be 'mark'", action.Type)
	}

	if state.CurrentTurn != player {
		return fmt.Errorf("not your turn")
	}

	posRaw, ok := action.Data["position"]
	if !ok {
		return fmt.Errorf("missing 'position' field")
	}

	pos := toInt(posRaw)
	if pos < 0 || pos > 8 {
		return fmt.Errorf("position must be 0-8, got %d", pos)
	}

	board := getBoard(state)
	row, col := pos/3, pos%3
	if board[row][col] != "" {
		return fmt.Errorf("position %d is already occupied", pos)
	}

	return nil
}

func (e *Engine) ApplyAction(state *engines.GameState, player engines.PlayerID, action engines.Action) (*engines.ActionResult, error) {
	pos := toInt(action.Data["position"])
	row, col := pos/3, pos%3

	// Determine mark: Player 1 = X, Player 2 = O
	mark := "X"
	if player == state.Players[1] {
		mark = "O"
	}

	// Place the mark
	board := state.Data["board"].([]any)
	boardRow := board[row].([]any)
	boardRow[col] = mark

	moveCount := toInt(state.Data["move_count"]) + 1
	state.Data["move_count"] = moveCount
	state.TurnNumber = moveCount

	// Switch turn to other player
	if player == state.Players[0] {
		state.CurrentTurn = state.Players[1]
	} else {
		state.CurrentTurn = state.Players[0]
	}

	return &engines.ActionResult{
		Success: true,
		Message: fmt.Sprintf("%s marks position %d (%d,%d)", mark, pos, row, col),
		Data: map[string]any{
			"mark":     mark,
			"position": pos,
			"row":      row,
			"col":      col,
		},
	}, nil
}

func (e *Engine) GetPlayerView(state *engines.GameState, player engines.PlayerID) *engines.PlayerView {
	board := getBoard(state)

	// Determine whose turn and what mark
	currentMark := "X"
	if state.CurrentTurn == state.Players[1] {
		currentMark = "O"
	}

	yourTurn := state.CurrentTurn == player

	// Available positions
	var available []int
	for i := range 9 {
		r, c := i/3, i%3
		if board[r][c] == "" {
			available = append(available, i)
		}
	}

	var availableActions []string
	if yourTurn && len(available) > 0 {
		availableActions = []string{"mark"}
	}

	// Convert board to [][]string for the view
	viewBoard := make([][]string, 3)
	for i := range 3 {
		viewBoard[i] = make([]string, 3)
		for j := range 3 {
			viewBoard[i][j] = board[i][j]
		}
	}

	return &engines.PlayerView{
		Phase:            state.Phase,
		YourTurn:         yourTurn,
		Simultaneous:     false,
		Board:            viewBoard,
		AvailableActions: availableActions,
		TurnNumber:       state.TurnNumber,
		GameSpecific: map[string]any{
			"current_player":      currentMark,
			"available_positions": available,
			"move_count":          toInt(state.Data["move_count"]),
		},
	}
}

func (e *Engine) CheckGameOver(state *engines.GameState) *engines.GameResult {
	board := getBoard(state)

	// Check all winning lines
	lines := [][3][2]int{
		// Rows
		{{0, 0}, {0, 1}, {0, 2}},
		{{1, 0}, {1, 1}, {1, 2}},
		{{2, 0}, {2, 1}, {2, 2}},
		// Columns
		{{0, 0}, {1, 0}, {2, 0}},
		{{0, 1}, {1, 1}, {2, 1}},
		{{0, 2}, {1, 2}, {2, 2}},
		// Diagonals
		{{0, 0}, {1, 1}, {2, 2}},
		{{0, 2}, {1, 1}, {2, 0}},
	}

	for _, line := range lines {
		a := board[line[0][0]][line[0][1]]
		b := board[line[1][0]][line[1][1]]
		c := board[line[2][0]][line[2][1]]
		if a != "" && a == b && b == c {
			// We have a winner
			var winner engines.PlayerID
			if a == "X" {
				winner = state.Players[0]
			} else {
				winner = state.Players[1]
			}
			state.Phase = "finished"
			return &engines.GameResult{
				Finished: true,
				Winner:   winner,
				Reason:   fmt.Sprintf("%s wins with three %s in a row", string(winner), a),
			}
		}
	}

	// Check draw: all cells filled
	moveCount := toInt(state.Data["move_count"])
	if moveCount >= 9 {
		state.Phase = "finished"
		return &engines.GameResult{
			Finished: true,
			Draw:     true,
			Reason:   "Draw - all cells filled with no winner",
		}
	}

	return nil
}

// --- Helpers ---

func getBoard(state *engines.GameState) [3][3]string {
	var board [3][3]string
	raw := state.Data["board"].([]any)
	for i, rowRaw := range raw {
		row := rowRaw.([]any)
		for j, cell := range row {
			if s, ok := cell.(string); ok {
				board[i][j] = s
			}
		}
	}
	return board
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
