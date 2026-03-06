# Chaos PD + Agent Names Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace boring always-cooperate PD with a chaotic, dramatic version that creates real strategic tension for LLM agents, and fix generic agent names.

**Architecture:** All game logic changes are in the PD engine. Chaos events, hidden objectives, and danger zone are stored in GameState.Data and exposed through GetPlayerView (agent) and buildPrisonersSpectatorState (spectator). Agent naming is a tool description change + server-side fallback.

**Tech Stack:** Go (engine), TypeScript (MCP tools), JavaScript (spectator renderer), CSS

---

### Task 1: Agent Name Personality - MCP Tool Descriptions

**Files:**
- Modify: `mcp/src/tools.ts`

**Step 1: Update play tool name description and make it required**

In `mcp/src/tools.ts`, change the `play` tool:

```typescript
// In the play tool's properties.name:
name: {
  type: "string",
  description:
    "Your fighter name for this match. Be creative and reflect your owner's personality! " +
    "Read your machine's hostname, OS, username, or environment to craft something unique. " +
    "Examples: 'SILICON_SAMURAI_M4', 'Ubuntu_Uppercut', 'Raspberry_Renegade'. " +
    "Generic names like 'Claude' or 'Assistant' are lame - bring some flair!",
},
// Change required to include name:
required: ["game_type", "name"],
```

**Step 2: Update create_match tool name description**

Same description change for the `create_match` tool's `name` field (keep it optional here since create_match is less common).

**Step 3: Build MCP**

Run: `cd /Users/yingcong/Code/claw-fight/mcp && npm run build`
Expected: Clean build

**Step 4: Commit**

```bash
git add mcp/src/tools.ts mcp/dist/
git commit -m "Encourage creative agent names in MCP tool descriptions"
```

---

### Task 2: Server-Side Boring Name Fallback

**Files:**
- Modify: `server/main.go:345-361`

**Step 1: Add name generator and boring name detection**

Add above `handleRegister`:

```go
var (
    boringNamePrefixes = []string{"claude", "agent", "assistant", "bot", "ai", "model", "llm", "chatbot"}
    funAdjectives      = []string{"CHROME", "NEON", "SHADOW", "IRON", "PIXEL", "COSMIC", "TURBO", "HYPER", "CYBER", "QUANTUM", "THUNDER", "STEALTH", "BLAZING", "ROGUE", "PHANTOM"}
    funNouns           = []string{"VIPER", "GHOST", "FALCON", "WOLF", "PHOENIX", "DRAGON", "TIGER", "COBRA", "HAWK", "LYNX", "RAPTOR", "STORM", "BLADE", "FANG", "SPARK"}
)

func isBoringName(name string) bool {
    lower := strings.ToLower(strings.TrimSpace(name))
    for _, prefix := range boringNamePrefixes {
        if strings.HasPrefix(lower, prefix) {
            return true
        }
    }
    return false
}

func generateFunName() string {
    ai, _ := rand.Int(rand.Reader, big.NewInt(int64(len(funAdjectives))))
    ni, _ := rand.Int(rand.Reader, big.NewInt(int64(len(funNouns))))
    return funAdjectives[ai.Int64()] + "_" + funNouns[ni.Int64()]
}
```

**Step 2: Update handleRegister to use it**

Replace the name fallback in `handleRegister`:

```go
func (s *Server) handleRegister(client *Client, msg WSMessage) {
    if msg.PlayerID == "" {
        msg.PlayerID = generateID(12)
    }
    if msg.PlayerName == "" || isBoringName(msg.PlayerName) {
        msg.PlayerName = generateFunName()
    }
    // ... rest unchanged
```

**Step 3: Verify build**

Run: `cd /Users/yingcong/Code/claw-fight/server && go build ./...`

**Step 4: Commit**

```bash
git add server/main.go
git commit -m "Generate fun names for agents with boring or missing names"
```

---

### Task 3: PD Engine - New Payoff Matrix + Random Rounds

**Files:**
- Modify: `server/engines/prisoners_dilemma/prisoners_dilemma.go`

**Step 1: Add crypto/rand import and random round generation**

Update imports to add `"crypto/rand"` and `"math/big"`. Change `defaultRounds` to min/max:

```go
const (
    minRounds = 50
    maxRounds = 100
)
```

**Step 2: Update InitGame for random rounds**

Replace the fixed round logic:

```go
func (e *Engine) InitGame(players []engines.PlayerID, options map[string]any) (*engines.GameState, error) {
    if len(players) != 2 {
        return nil, fmt.Errorf("prisoners_dilemma requires exactly 2 players")
    }

    // Random round count between 50-100
    n, _ := rand.Int(rand.Reader, big.NewInt(int64(maxRounds-minRounds+1)))
    totalRounds := minRounds + int(n.Int64())

    scores := make(map[string]any)
    for _, p := range players {
        scores[string(p)] = 0
    }

    return &engines.GameState{
        Phase:      "play",
        TurnNumber: 0,
        Data: map[string]any{
            "total_rounds":  totalRounds,
            "scores":        scores,
            "round_choices": make(map[string]any),
            "history":       []any{},
            "round_scores":  []any{},
        },
        Players:     players,
        CurrentTurn: "",
    }, nil
}
```

**Step 3: Update payoff matrix**

```go
func calculateScores(c1, c2 string) (s1, s2 int) {
    switch {
    case c1 == "cooperate" && c2 == "cooperate":
        return 3, 3
    case c1 == "cooperate" && c2 == "defect":
        return 0, 7
    case c1 == "defect" && c2 == "cooperate":
        return 7, 0
    default: // both defect
        return 1, 1
    }
}
```

**Step 4: Update DescribeRules**

```go
func (e *Engine) DescribeRules() string {
    return "Chaos Prisoner's Dilemma: Each round, both players simultaneously choose to cooperate or defect. " +
        "Both Cooperate: 3 pts each. One Defects while other Cooperates: defector gets 7, cooperator gets 0. " +
        "Both Defect: 1 pt each. Game lasts 50-100 rounds (exact count hidden). " +
        "Random chaos events may modify payoffs each round. Each player has a secret bonus objective worth 20 points. " +
        "If you fall 50+ points behind, you enter Danger Zone with 1.5x payoffs. Score hitting 0 = elimination."
}
```

**Step 5: Update GetPlayerView to hide exact total_rounds**

In `GetPlayerView`, change game_specific:
- Remove `"total_rounds"` (exact count)
- Add `"rounds_range": "50-100"`
- Change `"rounds_remaining"` to fuzzy: if `remaining > 10` show `"at least " + (minRounds - turnNumber)`, else show exact

**Step 6: Verify build**

Run: `cd /Users/yingcong/Code/claw-fight/server && go build ./...`

**Step 7: Commit**

```bash
git add server/engines/prisoners_dilemma/prisoners_dilemma.go
git commit -m "PD: new payoff matrix (7 for lone defector), random 50-100 rounds"
```

---

### Task 4: PD Engine - Chaos Events

**Files:**
- Modify: `server/engines/prisoners_dilemma/prisoners_dilemma.go`

**Step 1: Define event types and generation**

Add after the constants:

```go
type ChaosEvent struct {
    Type        string
    Description string
    SpyPlayer   int // -1 if not spy round, 0 or 1 for spy player index
}

var chaosEvents = []ChaosEvent{
    {Type: "double_stakes", Description: "DOUBLE STAKES: All payoffs are 2x this round!"},
    {Type: "betrayal_bonus", Description: "BETRAYAL BONUS: Defectors earn +3 extra points this round!"},
    {Type: "mercy_round", Description: "MERCY ROUND: Both Cooperate=6 each, Both Defect=0 each!"},
    {Type: "spy_round", Description: "SPY ROUND: One player sees the other's choice first!"},
    {Type: "reversal", Description: "REVERSAL: Cooperate and Defect are swapped this round!"},
    {Type: "jackpot", Description: "JACKPOT: 10 points up for grabs! Split if both cooperate, all to defector, lost if both defect!"},
}

func generateEvent(roundNum int) *ChaosEvent {
    // ~30% chance of event
    roll, _ := rand.Int(rand.Reader, big.NewInt(100))
    if roll.Int64() >= 30 {
        return nil
    }
    idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chaosEvents))))
    event := chaosEvents[idx.Int64()]
    if event.Type == "spy_round" {
        spy, _ := rand.Int(rand.Reader, big.NewInt(2))
        event.SpyPlayer = int(spy.Int64())
    } else {
        event.SpyPlayer = -1
    }
    return &event
}
```

**Step 2: Add event storage in InitGame**

Add to Data map in InitGame:

```go
"events":       map[string]any{}, // round_number(string) -> ChaosEvent
"current_event": nil,
```

**Step 3: Generate event at round start in GetPlayerView**

At the start of `GetPlayerView`, after reading history, generate event for current round if not already generated:

```go
// Generate chaos event for current round if needed
events, _ := state.Data["events"].(map[string]any)
if events == nil {
    events = map[string]any{}
    state.Data["events"] = events
}
roundKey := fmt.Sprintf("%d", state.TurnNumber)
var currentEvent *ChaosEvent
if existing, ok := events[roundKey]; ok {
    if ce, ok := existing.(*ChaosEvent); ok {
        currentEvent = ce
    }
} else if state.TurnNumber < totalRounds {
    currentEvent = generateEvent(state.TurnNumber)
    if currentEvent != nil {
        events[roundKey] = currentEvent
    }
}
```

**Step 4: Apply event modifiers in ApplyAction**

After `calculateScores(c1, c2)`, apply event modifiers:

```go
// Get current event
events, _ := state.Data["events"].(map[string]any)
roundKey := fmt.Sprintf("%d", state.TurnNumber)
var currentEvent *ChaosEvent
if events != nil {
    if ce, ok := events[roundKey].(*ChaosEvent); ok {
        currentEvent = ce
    }
}

if currentEvent != nil {
    switch currentEvent.Type {
    case "double_stakes":
        s1 *= 2
        s2 *= 2
    case "betrayal_bonus":
        if c1 == "defect" { s1 += 3 }
        if c2 == "defect" { s2 += 3 }
    case "mercy_round":
        if c1 == "cooperate" && c2 == "cooperate" {
            s1, s2 = 6, 6
        } else if c1 == "defect" && c2 == "defect" {
            s1, s2 = 0, 0
        }
    case "reversal":
        // Swap: treat cooperate as defect and vice versa in scoring
        s1, s2 = calculateScores(flipChoice(c1), flipChoice(c2))
    case "jackpot":
        if c1 == "cooperate" && c2 == "cooperate" {
            s1, s2 = 5, 5
        } else if c1 == "defect" && c2 == "cooperate" {
            s1, s2 = 10, 0
        } else if c1 == "cooperate" && c2 == "defect" {
            s1, s2 = 0, 10
        } else {
            s1, s2 = 0, 0
        }
    }
}
```

Add helper:

```go
func flipChoice(c string) string {
    if c == "cooperate" { return "defect" }
    return "cooperate"
}
```

**Step 5: Handle Spy Round in ValidateAction/ApplyAction**

For spy rounds, modify turn flow:
- In `ValidateAction`: if current event is spy_round and non-spy hasn't submitted yet, only allow non-spy to submit
- After non-spy submits in `ApplyAction`: set `CurrentTurn` to spy player, set `state.Data["spy_revealed_choice"]` to non-spy's choice
- When spy submits: resolve normally, clear spy data

```go
// In ValidateAction, after checking action type:
if currentEvent != nil && currentEvent.Type == "spy_round" {
    spyIdx := currentEvent.SpyPlayer
    nonSpyID := string(state.Players[1-spyIdx])
    spyID := string(state.Players[spyIdx])
    _, nonSpySubmitted := roundChoices[nonSpyID]
    _, spySubmitted := roundChoices[spyID]

    if string(player) == spyID && !nonSpySubmitted {
        return fmt.Errorf("spy round: waiting for opponent to choose first")
    }
    _ = spySubmitted // spy can submit after non-spy
}
```

In `ApplyAction`, after first player submits in spy round:

```go
if currentEvent != nil && currentEvent.Type == "spy_round" && len(roundChoices) == 1 {
    // First submission in spy round - check if it's the non-spy
    spyIdx := currentEvent.SpyPlayer
    nonSpyID := string(state.Players[1-spyIdx])
    if string(player) == nonSpyID {
        state.Data["spy_revealed_choice"] = choice
        state.CurrentTurn = state.Players[spyIdx]
        return &engines.ActionResult{
            Success: true,
            Message: "Waiting for spy player's choice",
            Data:    map[string]any{"status": "waiting"},
        }, nil
    }
}
```

**Step 6: Expose event in GetPlayerView**

Add to game_specific:

```go
if currentEvent != nil {
    eventInfo := map[string]any{
        "type":        currentEvent.Type,
        "description": currentEvent.Description,
    }
    // If spy round and this player is the spy, show opponent's revealed choice
    if currentEvent.Type == "spy_round" {
        spyIdx := currentEvent.SpyPlayer
        if player == state.Players[spyIdx] {
            if revealed, ok := state.Data["spy_revealed_choice"]; ok {
                eventInfo["opponent_revealed_choice"] = revealed
            }
        }
    }
    gameSpecific["current_event"] = eventInfo
}
```

Also include event type in board history entries:

```go
// When building pastRounds, include event info
roundKey := fmt.Sprintf("%d", i)
if events != nil {
    if ce, ok := events[roundKey].(*ChaosEvent); ok {
        pastRounds[idx]["event"] = ce.Type
    }
}
```

**Step 7: Verify build**

Run: `cd /Users/yingcong/Code/claw-fight/server && go build ./...`

**Step 8: Commit**

```bash
git add server/engines/prisoners_dilemma/prisoners_dilemma.go
git commit -m "PD: add chaos events (double stakes, betrayal bonus, spy round, etc)"
```

---

### Task 5: PD Engine - Hidden Objectives

**Files:**
- Modify: `server/engines/prisoners_dilemma/prisoners_dilemma.go`

**Step 1: Define objectives**

```go
type SecretObjective struct {
    Name        string
    Description string
    Check       func(history []any, playerID string, players []engines.PlayerID, totalRounds, currentRound int) (progress string, completed bool)
}

var secretObjectives = []SecretObjective{
    {
        Name:        "The Betrayer",
        Description: "Defect at least 8 times total",
    },
    {
        Name:        "The Streak",
        Description: "Cooperate 5 times in a row at some point",
    },
    {
        Name:        "The Alternator",
        Description: "Alternate cooperate/defect for 6 consecutive rounds",
    },
    {
        Name:        "The Closer",
        Description: "Defect on the last 3 rounds",
    },
    {
        Name:        "The Mirror",
        Description: "Match opponent's previous choice at least 10 times",
    },
}
```

Instead of function fields (which don't serialize), use a `checkObjective(name, history, playerID, opponentID, totalRounds, currentRound)` function that switches on name.

**Step 2: Assign objectives in InitGame**

```go
// Assign secret objectives
obj1, _ := rand.Int(rand.Reader, big.NewInt(int64(len(secretObjectives))))
obj2, _ := rand.Int(rand.Reader, big.NewInt(int64(len(secretObjectives))))
// Ensure different objectives
for obj2.Int64() == obj1.Int64() {
    obj2, _ = rand.Int(rand.Reader, big.NewInt(int64(len(secretObjectives))))
}

state.Data["secret_objectives"] = map[string]any{
    string(players[0]): map[string]any{
        "name":        secretObjectives[obj1.Int64()].Name,
        "description": secretObjectives[obj1.Int64()].Description,
    },
    string(players[1]): map[string]any{
        "name":        secretObjectives[obj2.Int64()].Name,
        "description": secretObjectives[obj2.Int64()].Description,
    },
}
```

**Step 3: Write checkObjective function**

```go
func checkObjective(objName string, history []any, playerID, opponentID string, totalRounds, currentRound int) (progress string, completed bool) {
    switch objName {
    case "The Betrayer":
        defects := countChoice(history, playerID, "defect")
        return fmt.Sprintf("%d/8 defections", defects), defects >= 8

    case "The Streak":
        maxStreak := longestStreak(history, playerID, "cooperate")
        return fmt.Sprintf("best streak: %d/5", maxStreak), maxStreak >= 5

    case "The Alternator":
        maxAlt := longestAlternating(history, playerID)
        return fmt.Sprintf("best alternating: %d/6", maxAlt), maxAlt >= 6

    case "The Closer":
        if len(history) < 3 || currentRound < totalRounds {
            last3Defects := countLastNChoice(history, playerID, "defect", 3)
            return fmt.Sprintf("%d/3 last defections", last3Defects), false
        }
        last3 := countLastNChoice(history, playerID, "defect", 3)
        return fmt.Sprintf("%d/3 last defections", last3), last3 == 3

    case "The Mirror":
        mirrors := countMirrors(history, playerID, opponentID)
        return fmt.Sprintf("%d/10 mirrors", mirrors), mirrors >= 10
    }
    return "unknown", false
}
```

With helpers `countChoice`, `longestStreak`, `longestAlternating`, `countLastNChoice`, `countMirrors` - each iterates over history.

**Step 4: Expose objective in GetPlayerView**

```go
if objectives, ok := state.Data["secret_objectives"].(map[string]any); ok {
    if obj, ok := objectives[string(player)].(map[string]any); ok {
        objName, _ := obj["name"].(string)
        opponentID := string(state.Players[0])
        if string(player) == opponentID {
            opponentID = string(state.Players[1])
        }
        progress, completed := checkObjective(objName, history, string(player), opponentID, totalRounds, state.TurnNumber)
        gameSpecific["secret_objective"] = map[string]any{
            "name":        objName,
            "description": obj["description"],
            "progress":    progress,
            "completed":   completed,
        }
    }
}
```

**Step 5: Award objective bonus in CheckGameOver**

In `CheckGameOver`, after computing scores, check and award bonuses:

```go
if objectives, ok := state.Data["secret_objectives"].(map[string]any); ok {
    for _, p := range state.Players {
        pid := string(p)
        if obj, ok := objectives[pid].(map[string]any); ok {
            objName, _ := obj["name"].(string)
            oppID := string(state.Players[0])
            if pid == oppID { oppID = string(state.Players[1]) }
            _, completed := checkObjective(objName, history, pid, oppID, totalRounds, state.TurnNumber)
            if completed {
                scores[pid] = toInt(scores[pid]) + 20
            }
        }
    }
}
// Re-read s1, s2 after bonus
```

**Step 6: Verify build**

Run: `cd /Users/yingcong/Code/claw-fight/server && go build ./...`

**Step 7: Commit**

```bash
git add server/engines/prisoners_dilemma/prisoners_dilemma.go
git commit -m "PD: add hidden secret objectives with 20pt bonus"
```

---

### Task 6: PD Engine - Danger Zone

**Files:**
- Modify: `server/engines/prisoners_dilemma/prisoners_dilemma.go`

**Step 1: Initialize danger zone state in InitGame**

Add to Data:

```go
"danger_zone": map[string]any{
    string(players[0]): map[string]any{"active": false, "rounds_remaining": 0},
    string(players[1]): map[string]any{"active": false, "rounds_remaining": 0},
},
```

**Step 2: Check and update danger zone in ApplyAction**

After updating scores, before advancing turn:

```go
// Check danger zone
dangerZone, _ := state.Data["danger_zone"].(map[string]any)
if dangerZone == nil {
    dangerZone = map[string]any{}
}

score1 := toInt(scores[string(p1)])
score2 := toInt(scores[string(p2)])
gap := score1 - score2

// Activate danger zone if 50+ points behind
for _, p := range []struct{ id string; behind int }{
    {string(p1), -gap},
    {string(p2), gap},
} {
    dz, _ := dangerZone[p.id].(map[string]any)
    if dz == nil {
        dz = map[string]any{"active": false, "rounds_remaining": 0}
    }
    active, _ := dz["active"].(bool)
    remaining := toInt(dz["rounds_remaining"])

    if active {
        remaining--
        if remaining <= 0 {
            dz["active"] = false
            dz["rounds_remaining"] = 0
        } else {
            dz["rounds_remaining"] = remaining
        }
    } else if p.behind >= 50 {
        dz["active"] = true
        dz["rounds_remaining"] = 3
    }
    dangerZone[p.id] = dz
}
state.Data["danger_zone"] = dangerZone
```

**Step 3: Apply danger zone multiplier to scores**

Before adding to cumulative scores, check if player is in danger zone and multiply by 1.5:

```go
// Apply danger zone multiplier
dangerZone, _ := state.Data["danger_zone"].(map[string]any)
for _, pid := range []string{string(p1), string(p2)} {
    if dz, ok := dangerZone[pid].(map[string]any); ok {
        if active, _ := dz["active"].(bool); active {
            // Apply 1.5x - use pointer to s1/s2
            if pid == string(p1) {
                s1 = s1 * 3 / 2  // integer 1.5x
            } else {
                s2 = s2 * 3 / 2
            }
        }
    }
}
```

**Step 4: Check elimination in CheckGameOver**

Add at the beginning of CheckGameOver:

```go
scores := state.Data["scores"].(map[string]any)
p1 := state.Players[0]
p2 := state.Players[1]
s1 := toInt(scores[string(p1)])
s2 := toInt(scores[string(p2)])

// Elimination check
if s1 <= 0 {
    state.Phase = "finished"
    return &engines.GameResult{
        Finished: true, Winner: p2,
        Scores: map[engines.PlayerID]int{p1: s1, p2: s2},
        Reason: fmt.Sprintf("%s eliminated (score dropped to %d)", string(p1), s1),
    }
}
if s2 <= 0 {
    state.Phase = "finished"
    return &engines.GameResult{
        Finished: true, Winner: p1,
        Scores: map[engines.PlayerID]int{p1: s1, p2: s2},
        Reason: fmt.Sprintf("%s eliminated (score dropped to %d)", string(p2), s2),
    }
}
```

**Step 5: Expose danger zone in GetPlayerView**

```go
if dangerZone, ok := state.Data["danger_zone"].(map[string]any); ok {
    if dz, ok := dangerZone[string(player)].(map[string]any); ok {
        gameSpecific["danger_zone"], _ = dz["active"].(bool)
    }
    // Opponent danger zone
    opponentID := string(state.Players[0])
    if string(player) == opponentID { opponentID = string(state.Players[1]) }
    if dz, ok := dangerZone[opponentID].(map[string]any); ok {
        gameSpecific["opponent_danger_zone"], _ = dz["active"].(bool)
    }
}
```

**Step 6: Verify build**

Run: `cd /Users/yingcong/Code/claw-fight/server && go build ./...`

**Step 7: Commit**

```bash
git add server/engines/prisoners_dilemma/prisoners_dilemma.go
git commit -m "PD: add danger zone (1.5x payoffs when 50+ behind) and elimination"
```

---

### Task 7: Update PD Spectator State in match.go

**Files:**
- Modify: `server/match.go:669-723` (`buildPrisonersSpectatorState`)

**Step 1: Add chaos event, objectives, danger zone to spectator state**

At the end of `buildPrisonersSpectatorState`, add to the returned map:

```go
// Current event
if events, ok := data["events"].(map[string]any); ok {
    roundKey := fmt.Sprintf("%d", m.State.TurnNumber)
    if ce, ok := events[roundKey]; ok {
        // ce is *ChaosEvent from PD engine - extract fields
        result["current_event"] = ce
    }
}

// Secret objectives (show both for spectators)
if objectives, ok := data["secret_objectives"].(map[string]any); ok {
    spectatorObjectives := make([]map[string]any, 2)
    history := data["history"].([]any)
    totalRounds := toInt(data["total_rounds"])
    for i, pid := range []string{p1, p2} {
        if obj, ok := objectives[pid].(map[string]any); ok {
            objName, _ := obj["name"].(string)
            oppID := p2
            if pid == p2 { oppID = p1 }
            // Import checkObjective from PD package or compute inline
            spectatorObjectives[i] = map[string]any{
                "name":        objName,
                "description": obj["description"],
            }
        } else {
            spectatorObjectives[i] = map[string]any{}
        }
    }
    result["secret_objectives"] = spectatorObjectives
}

// Danger zone status
if dangerZone, ok := data["danger_zone"].(map[string]any); ok {
    dzStatus := make([]bool, 2)
    for i, pid := range []string{p1, p2} {
        if dz, ok := dangerZone[pid].(map[string]any); ok {
            dzStatus[i], _ = dz["active"].(bool)
        }
    }
    result["danger_zone"] = dzStatus
}
```

Note: Since `checkObjective` is in the PD package and match.go is in main, we have two options: (a) export it, or (b) just send the raw objective data and let the spectator JS not show progress. Option (b) is simpler for now - spectators see the objective name but progress is only shown to agents.

**Step 2: Verify build**

Run: `cd /Users/yingcong/Code/claw-fight/server && go build ./...`

**Step 3: Commit**

```bash
git add server/match.go
git commit -m "Spectator: show chaos events, objectives, danger zone for PD"
```

---

### Task 8: PD Spectator Renderer - Visual Updates

**Files:**
- Modify: `server/web/static/js/board_prisoners.js`
- Modify: `server/web/static/css/style.css`

**Step 1: Add chaos event banner to renderer**

In `renderPrisonersBoard`, after round info and before scores, add:

```javascript
// Chaos event banner
if (state.current_event) {
    var eventBanner = document.createElement('div');
    eventBanner.className = 'prisoners-event-banner';
    eventBanner.textContent = state.current_event.Description || state.current_event.type || 'CHAOS EVENT';
    container.appendChild(eventBanner);
}
```

**Step 2: Add danger zone indicators to player scores**

In the score rendering, add danger zone class:

```javascript
// After creating p1Score/p2Score divs:
var p1DZ = state.danger_zone && state.danger_zone[0];
var p2DZ = state.danger_zone && state.danger_zone[1];
if (p1DZ) p1Score.className += ' danger-zone';
if (p2DZ) p2Score.className += ' danger-zone';
```

**Step 3: Add secret objectives display below scores**

```javascript
// Secret objectives (spectator sees both)
if (state.secret_objectives && state.secret_objectives.length === 2) {
    var objSection = document.createElement('div');
    objSection.className = 'prisoners-objectives';
    var objNames = [p1Name, p2Name];
    for (var oi = 0; oi < 2; oi++) {
        var obj = state.secret_objectives[oi];
        if (obj && obj.name) {
            var objEl = document.createElement('div');
            objEl.className = 'prisoners-objective';
            objEl.innerHTML = '<span class="obj-player">' + objNames[oi] + '</span>: ' +
                '<span class="obj-name">' + obj.name + '</span> - ' +
                '<span class="obj-desc">' + (obj.description || '') + '</span>';
            objSection.appendChild(objEl);
        }
    }
    container.appendChild(objSection);
}
```

**Step 4: Add event indicators to move history**

When rendering moves, add event markers:

```javascript
// In the moves forEach, check if round had an event
// Add event dot/marker to move cells for event rounds
```

**Step 5: Add CSS styles**

Append to `server/web/static/css/style.css`:

```css
/* PD Chaos Event Banner */
.prisoners-event-banner {
    text-align: center;
    font-family: 'Press Start 2P', monospace;
    font-size: 0.8rem;
    color: var(--gold);
    padding: 10px;
    margin: 8px 0;
    background: rgba(255, 215, 0, 0.1);
    border: var(--border-w) solid var(--gold-dim);
    text-shadow: 2px 2px 0 var(--text-shadow);
    animation: event-pulse 2s ease-in-out infinite;
}

@keyframes event-pulse {
    0%, 100% { border-color: var(--gold-dim); }
    50% { border-color: var(--gold); box-shadow: 0 0 12px rgba(255, 215, 0, 0.3); }
}

/* Danger Zone */
.prisoners-player-score.danger-zone {
    border-color: var(--red) !important;
    animation: danger-flash 1s ease-in-out infinite;
}

@keyframes danger-flash {
    0%, 100% { box-shadow: 0 0 8px rgba(232, 64, 64, 0.3); }
    50% { box-shadow: 0 0 16px rgba(232, 64, 64, 0.6); }
}

/* Secret Objectives */
.prisoners-objectives {
    display: flex;
    gap: 16px;
    justify-content: center;
    margin: 8px 0;
    flex-wrap: wrap;
}

.prisoners-objective {
    font-family: 'Silkscreen', monospace;
    font-size: 0.7rem;
    color: var(--text-muted);
    padding: 6px 10px;
    background: var(--panel-dark);
    border: 1px solid var(--panel-border);
}

.obj-name {
    color: var(--cyan);
    font-weight: bold;
}

.obj-player {
    color: var(--text);
}
```

**Step 6: Commit**

```bash
git add server/web/static/js/board_prisoners.js server/web/static/css/style.css
git commit -m "PD spectator: event banners, danger zone flashing, objective display"
```

---

### Task 9: Integration Test - Full Chaos PD Game

**Step 1: Run build**

```bash
cd /Users/yingcong/Code/claw-fight/server && go build ./...
```

**Step 2: Run tests**

```bash
cd /Users/yingcong/Code/claw-fight/server && go test ./... -count=1
```

**Step 3: Manual smoke test**

Start the server and verify:
1. PD game creates with random round count
2. Chaos events appear ~30% of rounds
3. Payoff matrix gives 7 to lone defector
4. Agent view shows event, objective, danger zone, fuzzy round count
5. Spectator view shows events, both objectives, danger zone indicators

```bash
cd /Users/yingcong/Code/claw-fight/server && go run .
```

**Step 4: Build MCP and verify**

```bash
cd /Users/yingcong/Code/claw-fight/mcp && npm run build
```

**Step 5: Final commit if any fixups needed**
