package battleship

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/claw-fight/server/engines"
)

const boardSize = 10

type ShipDef struct {
	Name string
	Size int
}

var shipDefs = []ShipDef{
	{"carrier", 5},
	{"battleship", 4},
	{"cruiser", 3},
	{"submarine", 3},
	{"destroyer", 2},
}

type Cell int

const (
	CellEmpty Cell = iota
	CellShip
	CellHit
	CellMiss
)

type Ship struct {
	Name   string
	Cells  [][2]int // row, col positions
	Hits   int
	Placed bool
}

type Board struct {
	Grid    [boardSize][boardSize]Cell
	ShipMap [boardSize][boardSize]int // ship index at each cell (-1 = none)
	Ships   []Ship
}

type PlayerData struct {
	Board      Board
	ShotBoard  [boardSize][boardSize]Cell // tracks shots at opponent
	ShipsReady bool
}

type Engine struct{}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Name() string      { return "battleship" }
func (e *Engine) MinPlayers() int    { return 2 }
func (e *Engine) MaxPlayers() int    { return 2 }

func (e *Engine) DescribeRules() string {
	return "Classic Battleship on a 10x10 grid (columns A-J, rows 1-10). " +
		"Ships: carrier(5), battleship(4), cruiser(3), submarine(3), destroyer(2). " +
		"Phase 1 (setup): place_ships action with data: {\"ships\": [{\"name\": \"carrier\", \"start\": \"A1\", \"end\": \"A5\"}, ...]}. " +
		"Ship names must be lowercase. Start/end are coordinates like 'A1', 'J10'. Ships must be horizontal or vertical. " +
		"Phase 2 (play): fire action with data: {\"target\": \"B3\"}. " +
		"You get hit/miss/sunk feedback. First to sink all opponent ships wins."
}

func (e *Engine) InitGame(players []engines.PlayerID, options map[string]any) (*engines.GameState, error) {
	if len(players) != 2 {
		return nil, fmt.Errorf("battleship requires exactly 2 players")
	}

	playerData := make(map[string]any)
	for _, p := range players {
		pd := &PlayerData{
			Board: Board{Ships: make([]Ship, len(shipDefs))},
		}
		// Init ship map to -1 (no ship)
		for r := range boardSize {
			for c := range boardSize {
				pd.Board.ShipMap[r][c] = -1
			}
		}
		for i, sd := range shipDefs {
			pd.Board.Ships[i] = Ship{Name: sd.Name}
		}
		playerData[string(p)] = pd
	}

	return &engines.GameState{
		Phase:       "setup",
		TurnNumber:  0,
		Data:        playerData,
		Players:     players,
		CurrentTurn: players[0],
	}, nil
}

func (e *Engine) ValidateAction(state *engines.GameState, player engines.PlayerID, action engines.Action) error {
	pd := getPlayerData(state, player)
	if pd == nil {
		return fmt.Errorf("player not in game")
	}

	switch action.Type {
	case "place_ships":
		if state.Phase != "setup" {
			return fmt.Errorf("can only place ships during setup phase")
		}
		if pd.ShipsReady {
			return fmt.Errorf("ships already placed")
		}
		return e.validateShipPlacement(action)

	case "fire":
		if state.Phase != "play" {
			return fmt.Errorf("can only fire during play phase")
		}
		if state.CurrentTurn != player {
			return fmt.Errorf("not your turn")
		}
		return e.validateFire(pd, action)

	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

func (e *Engine) ApplyAction(state *engines.GameState, player engines.PlayerID, action engines.Action) (*engines.ActionResult, error) {
	pd := getPlayerData(state, player)

	switch action.Type {
	case "place_ships":
		return e.applyPlaceShips(state, player, pd, action)
	case "fire":
		return e.applyFire(state, player, pd, action)
	default:
		return nil, fmt.Errorf("unknown action type: %s", action.Type)
	}
}

func (e *Engine) GetPlayerView(state *engines.GameState, player engines.PlayerID) *engines.PlayerView {
	pd := getPlayerData(state, player)
	if pd == nil {
		return nil
	}

	var availableActions []string
	yourTurn := false

	switch state.Phase {
	case "setup":
		if !pd.ShipsReady {
			availableActions = []string{"place_ships"}
			yourTurn = true
		}
	case "play":
		if state.CurrentTurn == player {
			availableActions = []string{"fire"}
			yourTurn = true
		}
	}

	// Ship initial letters for display
	shipChars := map[int]string{0: "C", 1: "B", 2: "R", 3: "S", 4: "D"} // carrier, battleship, cruiser, submarine, destroyer

	// Own board: compress each row to a single string
	ownBoard := make([]string, boardSize)
	for r := range boardSize {
		row := make([]byte, boardSize)
		for c := range boardSize {
			switch pd.Board.Grid[r][c] {
			case CellEmpty:
				row[c] = '.'
			case CellShip:
				if ch, ok := shipChars[pd.Board.ShipMap[r][c]]; ok {
					row[c] = ch[0]
				} else {
					row[c] = 'S'
				}
			case CellHit:
				row[c] = 'H'
			case CellMiss:
				row[c] = 'M'
			}
		}
		ownBoard[r] = string(row)
	}

	// Opponent board: compress each row to a single string
	opponentBoard := make([]string, boardSize)
	for r := range boardSize {
		row := make([]byte, boardSize)
		for c := range boardSize {
			switch pd.ShotBoard[r][c] {
			case CellHit:
				row[c] = 'X'
			case CellMiss:
				row[c] = 'O'
			default:
				row[c] = '.'
			}
		}
		opponentBoard[r] = string(row)
	}

	// Ship status
	shipStatus := make([]map[string]any, len(pd.Board.Ships))
	for i, s := range pd.Board.Ships {
		shipStatus[i] = map[string]any{
			"name":   s.Name,
			"size":   shipDefs[i].Size,
			"placed": s.Placed,
			"sunk":   s.Placed && s.Hits >= shipDefs[i].Size,
			"hits":   s.Hits,
		}
	}

	return &engines.PlayerView{
		Phase:            state.Phase,
		YourTurn:         yourTurn,
		Simultaneous:     state.Phase == "setup",
		Board: map[string]any{
			"own":      ownBoard,
			"opponent": opponentBoard,
		},
		AvailableActions: availableActions,
		TurnNumber:       state.TurnNumber,
		GameSpecific: map[string]any{
			"ships":      shipStatus,
			"board_size": boardSize,
		},
	}
}

func (e *Engine) CheckGameOver(state *engines.GameState) *engines.GameResult {
	if state.Phase != "play" {
		return nil
	}

	for _, player := range state.Players {
		pd := getPlayerData(state, player)
		if pd == nil {
			continue
		}
		allSunk := true
		for i, ship := range pd.Board.Ships {
			if !ship.Placed || ship.Hits < shipDefs[i].Size {
				allSunk = false
				break
			}
		}
		if allSunk {
			// This player's ships are all sunk, opponent wins
			var winner engines.PlayerID
			for _, p := range state.Players {
				if p != player {
					winner = p
					break
				}
			}
			state.Phase = "finished"
			return &engines.GameResult{
				Finished: true,
				Winner:   winner,
				Reason:   fmt.Sprintf("All of %s's ships have been sunk", string(player)),
			}
		}
	}
	return nil
}

// --- Internal helpers ---

func (e *Engine) validateShipPlacement(action engines.Action) error {
	shipsRaw, ok := action.Data["ships"]
	if !ok {
		return fmt.Errorf("missing ships data")
	}

	ships, ok := shipsRaw.([]any)
	if !ok {
		return fmt.Errorf("ships must be an array")
	}

	if len(ships) != len(shipDefs) {
		return fmt.Errorf("must place exactly %d ships", len(shipDefs))
	}

	// Track which ship names are placed
	placed := make(map[string]bool)
	// Track occupied cells
	occupied := make(map[[2]int]bool)

	for _, shipRaw := range ships {
		shipMap, ok := shipRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid ship format")
		}

		name, _ := shipMap["name"].(string)
		startStr, _ := shipMap["start"].(string)
		endStr, _ := shipMap["end"].(string)

		if name == "" || startStr == "" || endStr == "" {
			return fmt.Errorf("ship must have name, start, and end")
		}

		// Find ship definition
		var shipDef *ShipDef
		for _, sd := range shipDefs {
			if sd.Name == name {
				sd := sd
				shipDef = &sd
				break
			}
		}
		if shipDef == nil {
			return fmt.Errorf("unknown ship: %s", name)
		}

		if placed[name] {
			return fmt.Errorf("duplicate ship: %s", name)
		}

		startRow, startCol, err := parseCoord(startStr)
		if err != nil {
			return fmt.Errorf("invalid start coordinate: %s", startStr)
		}
		endRow, endCol, err := parseCoord(endStr)
		if err != nil {
			return fmt.Errorf("invalid end coordinate: %s", endStr)
		}

		// Must be horizontal or vertical
		if startRow != endRow && startCol != endCol {
			return fmt.Errorf("ship %s must be horizontal or vertical", name)
		}

		// Calculate cells
		cells := shipCells(startRow, startCol, endRow, endCol)
		if len(cells) != shipDef.Size {
			return fmt.Errorf("ship %s must be %d cells, got %d", name, shipDef.Size, len(cells))
		}

		// Check bounds and overlaps
		for _, c := range cells {
			if c[0] < 0 || c[0] >= boardSize || c[1] < 0 || c[1] >= boardSize {
				return fmt.Errorf("ship %s out of bounds", name)
			}
			if occupied[c] {
				return fmt.Errorf("ship %s overlaps with another ship", name)
			}
			occupied[c] = true
		}

		placed[name] = true
	}

	return nil
}

func (e *Engine) validateFire(pd *PlayerData, action engines.Action) error {
	targetStr, ok := action.Data["target"].(string)
	if !ok || targetStr == "" {
		return fmt.Errorf("missing target coordinate")
	}

	row, col, err := parseCoord(targetStr)
	if err != nil {
		return fmt.Errorf("invalid target: %s", targetStr)
	}

	if row < 0 || row >= boardSize || col < 0 || col >= boardSize {
		return fmt.Errorf("target out of bounds")
	}

	if pd.ShotBoard[row][col] != CellEmpty {
		return fmt.Errorf("already fired at %s", targetStr)
	}

	return nil
}

func (e *Engine) applyPlaceShips(state *engines.GameState, _ engines.PlayerID, pd *PlayerData, action engines.Action) (*engines.ActionResult, error) {
	ships := action.Data["ships"].([]any)

	for _, shipRaw := range ships {
		shipMap := shipRaw.(map[string]any)
		name := shipMap["name"].(string)
		startStr := shipMap["start"].(string)
		endStr := shipMap["end"].(string)

		startRow, startCol, _ := parseCoord(startStr)
		endRow, endCol, _ := parseCoord(endStr)

		cells := shipCells(startRow, startCol, endRow, endCol)

		// Find ship index
		shipIdx := -1
		for i, s := range pd.Board.Ships {
			if s.Name == name {
				pd.Board.Ships[i].Cells = cells
				pd.Board.Ships[i].Placed = true
				shipIdx = i
				break
			}
		}

		// Mark grid
		for _, c := range cells {
			pd.Board.Grid[c[0]][c[1]] = CellShip
			pd.Board.ShipMap[c[0]][c[1]] = shipIdx
		}
	}

	pd.ShipsReady = true

	// Check if both players have placed ships
	allReady := true
	for _, p := range state.Players {
		opd := getPlayerData(state, p)
		if !opd.ShipsReady {
			allReady = false
			break
		}
	}

	if allReady {
		state.Phase = "play"
		state.TurnNumber = 1
		state.CurrentTurn = state.Players[0]
	}

	return &engines.ActionResult{
		Success: true,
		Message: "Ships placed successfully",
		Data: map[string]any{
			"game_started": allReady,
		},
	}, nil
}

func (e *Engine) applyFire(state *engines.GameState, player engines.PlayerID, pd *PlayerData, action engines.Action) (*engines.ActionResult, error) {
	targetStr := action.Data["target"].(string)
	row, col, _ := parseCoord(targetStr)

	// Find opponent
	var opponent engines.PlayerID
	for _, p := range state.Players {
		if p != player {
			opponent = p
			break
		}
	}

	opd := getPlayerData(state, opponent)

	var resultMsg string
	var resultData map[string]any

	if opd.Board.Grid[row][col] == CellShip {
		opd.Board.Grid[row][col] = CellHit
		pd.ShotBoard[row][col] = CellHit

		// Find which ship was hit
		var hitShip string
		var sunk bool
		for i, ship := range opd.Board.Ships {
			for _, c := range ship.Cells {
				if c[0] == row && c[1] == col {
					opd.Board.Ships[i].Hits++
					hitShip = ship.Name
					if opd.Board.Ships[i].Hits >= shipDefs[i].Size {
						sunk = true
					}
					break
				}
			}
			if hitShip != "" {
				break
			}
		}

		if sunk {
			resultMsg = fmt.Sprintf("Hit and sunk %s!", hitShip)
			resultData = map[string]any{"result": "sunk", "ship": hitShip, "target": targetStr}
		} else {
			resultMsg = "Hit!"
			resultData = map[string]any{"result": "hit", "target": targetStr}
		}
	} else {
		opd.Board.Grid[row][col] = CellMiss
		pd.ShotBoard[row][col] = CellMiss
		resultMsg = "Miss"
		resultData = map[string]any{"result": "miss", "target": targetStr}
	}

	// Advance turn
	state.TurnNumber++
	state.CurrentTurn = opponent

	return &engines.ActionResult{
		Success: true,
		Message: resultMsg,
		Data:    resultData,
	}, nil
}

func getPlayerData(state *engines.GameState, player engines.PlayerID) *PlayerData {
	if pd, ok := state.Data[string(player)].(*PlayerData); ok {
		return pd
	}
	return nil
}

func parseCoord(s string) (row, col int, err error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) < 2 || len(s) > 3 {
		return 0, 0, fmt.Errorf("invalid coordinate: %s", s)
	}

	letter := s[0]
	if letter < 'A' || letter > 'J' {
		return 0, 0, fmt.Errorf("column must be A-J")
	}
	col = int(letter - 'A')

	num, err := strconv.Atoi(s[1:])
	if err != nil || num < 1 || num > 10 {
		return 0, 0, fmt.Errorf("row must be 1-10")
	}
	row = num - 1

	return row, col, nil
}

func (e *Engine) GetSpectatorView(state *engines.GameState) map[string]any {
	views := make(map[string]any)
	shipStatus := make(map[string]any)
	for _, pid := range state.Players {
		p := string(pid)
		pv := e.GetPlayerView(state, pid)
		views[p] = pv
		if gs, ok := pv.GameSpecific["ships"]; ok {
			switch ships := gs.(type) {
			case []map[string]any:
				total := len(ships)
				sunk := 0
				for _, ship := range ships {
					if isSunk, ok := ship["sunk"].(bool); ok && isSunk {
						sunk++
					}
				}
				shipStatus[p] = map[string]any{"sunk": sunk, "total": total}
			case []any:
				total := len(ships)
				sunk := 0
				for _, s := range ships {
					if ship, ok := s.(map[string]any); ok {
						if isSunk, ok := ship["sunk"].(bool); ok && isSunk {
							sunk++
						}
					}
				}
				shipStatus[p] = map[string]any{"sunk": sunk, "total": total}
			}
		}
	}

	result := map[string]any{
		"views":       views,
		"ship_status": shipStatus,
	}

	if len(state.ActionLog) > 0 {
		lastEntry := state.ActionLog[len(state.ActionLog)-1]
		if lastEntry.Action.Type == "fire" {
			if target, ok := lastEntry.Action.Data["target"]; ok {
				result["last_action"] = map[string]any{
					"player": string(lastEntry.Player),
					"target": target,
				}
			}
		}
	}

	return result
}

func shipCells(r1, c1, r2, c2 int) [][2]int {
	var cells [][2]int
	if r1 == r2 {
		// Horizontal
		minC, maxC := c1, c2
		if minC > maxC {
			minC, maxC = maxC, minC
		}
		for c := minC; c <= maxC; c++ {
			cells = append(cells, [2]int{r1, c})
		}
	} else {
		// Vertical
		minR, maxR := r1, r2
		if minR > maxR {
			minR, maxR = maxR, minR
		}
		for r := minR; r <= maxR; r++ {
			cells = append(cells, [2]int{r, c1})
		}
	}
	return cells
}
