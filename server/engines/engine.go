package engines

type PlayerID string

type Action struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type ActionResult struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type PlayerView struct {
	Phase            string         `json:"phase"`
	YourTurn         bool           `json:"your_turn"`
	Simultaneous     bool           `json:"simultaneous"`
	Board            any            `json:"board"`
	AvailableActions []string       `json:"available_actions"`
	LastAction       *ActionResult  `json:"last_action,omitempty"`
	TurnNumber       int            `json:"turn_number"`
	GameSpecific     map[string]any `json:"game_specific,omitempty"`
}

type GameResult struct {
	Finished bool             `json:"finished"`
	Winner   PlayerID         `json:"winner,omitempty"`
	Draw     bool             `json:"draw"`
	Scores   map[PlayerID]int `json:"scores,omitempty"`
	Reason   string           `json:"reason"`
}

type GameState struct {
	Phase       string
	TurnNumber  int
	Data        map[string]any
	Players     []PlayerID
	CurrentTurn PlayerID
	ActionLog   []ActionLogEntry
}

type ActionLogEntry struct {
	Player PlayerID
	Action Action
	Result ActionResult
	Seq    int
}

type GameEngine interface {
	Name() string
	MinPlayers() int
	MaxPlayers() int
	InitGame(players []PlayerID, options map[string]any) (*GameState, error)
	ValidateAction(state *GameState, player PlayerID, action Action) error
	ApplyAction(state *GameState, player PlayerID, action Action) (*ActionResult, error)
	GetPlayerView(state *GameState, player PlayerID) *PlayerView
	CheckGameOver(state *GameState) *GameResult
	DescribeRules() string
	GetSpectatorView(state *GameState) map[string]any
}
