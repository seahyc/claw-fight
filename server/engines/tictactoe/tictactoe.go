package tictactoe

import (
	"fmt"

	"github.com/claw-fight/server/engines"
)

const (
	boardSize = 5
	winLength = 4
	totalCells = boardSize * boardSize
)

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Name() string   { return "tictactoe" }
func (e *Engine) MinPlayers() int { return 2 }
func (e *Engine) MaxPlayers() int { return 2 }

func (e *Engine) DescribeRules() string {
	return fmt.Sprintf("Tic Tac Toe on a %dx%d grid. Player 1 is X, Player 2 is O. "+
		"Players alternate turns placing their mark on an empty cell. "+
		"Action: mark with data {\"position\": 0-%d} where 0=top-left, %d=bottom-right (left-to-right, top-to-bottom). "+
		"First to get %d in a row (horizontal, vertical, or diagonal) wins. "+
		"If all %d cells are filled with no winner, it's a draw.",
		boardSize, boardSize, totalCells-1, totalCells-1, winLength, totalCells)
}

func (e *Engine) InitGame(players []engines.PlayerID, options map[string]any) (*engines.GameState, error) {
	if len(players) != 2 {
		return nil, fmt.Errorf("tictactoe requires exactly 2 players")
	}

	board := make([]any, boardSize)
	for i := range boardSize {
		row := make([]any, boardSize)
		for j := range boardSize {
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
		CurrentTurn: players[0],
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

	pos := engines.ToInt(posRaw)
	if pos < 0 || pos >= totalCells {
		return fmt.Errorf("position must be 0-%d, got %d", totalCells-1, pos)
	}

	board := getBoard(state)
	row, col := pos/boardSize, pos%boardSize
	if board[row][col] != "" {
		return fmt.Errorf("position %d is already occupied", pos)
	}

	return nil
}

func (e *Engine) ApplyAction(state *engines.GameState, player engines.PlayerID, action engines.Action) (*engines.ActionResult, error) {
	pos := engines.ToInt(action.Data["position"])
	row, col := pos/boardSize, pos%boardSize

	mark := "X"
	if player == state.Players[1] {
		mark = "O"
	}

	board := state.Data["board"].([]any)
	boardRow := board[row].([]any)
	boardRow[col] = mark

	moveCount := engines.ToInt(state.Data["move_count"]) + 1
	state.Data["move_count"] = moveCount
	state.TurnNumber = moveCount

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

	currentMark := "X"
	if state.CurrentTurn == state.Players[1] {
		currentMark = "O"
	}

	yourTurn := state.CurrentTurn == player

	var available []int
	for i := range totalCells {
		r, c := i/boardSize, i%boardSize
		if board[r][c] == "" {
			available = append(available, i)
		}
	}

	var availableActions []string
	if yourTurn && len(available) > 0 {
		availableActions = []string{"mark"}
	}

	viewBoard := make([][]string, boardSize)
	for i := range boardSize {
		viewBoard[i] = make([]string, boardSize)
		for j := range boardSize {
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
			"board_size":          boardSize,
			"win_length":         winLength,
			"current_player":     currentMark,
			"available_positions": available,
			"move_count":         engines.ToInt(state.Data["move_count"]),
		},
	}
}

func (e *Engine) CheckGameOver(state *engines.GameState) *engines.GameResult {
	board := getBoard(state)

	// Check all possible lines of winLength
	directions := [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} // horizontal, vertical, diagonal, anti-diagonal

	for r := range boardSize {
		for c := range boardSize {
			if board[r][c] == "" {
				continue
			}
			mark := board[r][c]
			for _, d := range directions {
				// Check if winLength cells in this direction all match
				if r+d[0]*(winLength-1) < 0 || r+d[0]*(winLength-1) >= boardSize {
					continue
				}
				if c+d[1]*(winLength-1) < 0 || c+d[1]*(winLength-1) >= boardSize {
					continue
				}
				won := true
				for k := 1; k < winLength; k++ {
					if board[r+d[0]*k][c+d[1]*k] != mark {
						won = false
						break
					}
				}
				if won {
					var winner engines.PlayerID
					if mark == "X" {
						winner = state.Players[0]
					} else {
						winner = state.Players[1]
					}
					state.Phase = "finished"
					return &engines.GameResult{
						Finished: true,
						Winner:   winner,
						Reason:   fmt.Sprintf("%s wins with %d %s in a row!", string(winner), winLength, mark),
					}
				}
			}
		}
	}

	// Check draw
	moveCount := engines.ToInt(state.Data["move_count"])
	if moveCount >= totalCells {
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

func getBoard(state *engines.GameState) [boardSize][boardSize]string {
	var board [boardSize][boardSize]string
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

