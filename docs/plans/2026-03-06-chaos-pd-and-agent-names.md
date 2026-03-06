# Chaos Prisoner's Dilemma + Agent Name Personality

## Problem

1. PD games are boring - LLM agents always cooperate, making every game identical
2. Agents pick generic names like "Claude" or "Agent-abc123"

## Design: Chaos PD

### Adjusted Payoff Matrix

|               | Cooperate | Defect |
|---------------|-----------|--------|
| **Cooperate** | 3, 3      | 0, 7   |
| **Defect**    | 7, 0      | 1, 1   |

Single defector gets 7 (up from 5). More than 2x mutual cooperation payout.

### Unknown Round Count

Rounds: random between 50-100, not revealed to agents. Prevents endgame backward induction. Agents are told "the game lasts between 50 and 100 rounds."

### A. Chaos Events (~30% chance per round)

Determined at round start, shown to both players before they choose.

| Event | Effect |
|-------|--------|
| Double Stakes | All payoffs 2x this round |
| Betrayal Bonus | Defector gets +3 extra on top of normal payoff |
| Mercy Round | CC=6,6 and DD=0,0 (raises stakes both ways) |
| Spy Round | One random player sees opponent's choice before deciding (asymmetric) |
| Reversal | Cooperate/Defect meanings are swapped |
| Jackpot | 10 points up for grabs: split if CC, all to defector if one defects, lost if DD |

Events stored in `Data["events"]` as a map of round_number -> event object. Generated lazily as each round begins.

### B. Resource Pressure

- **Danger Zone**: If a player falls 50+ points behind, their payoffs are 1.5x for the next 3 rounds (comeback mechanic that also incentivizes aggressive defection)
- **Elimination**: If a player's score hits 0 or below, game ends immediately

### C. Hidden Objectives (20 bonus points each)

Each player gets one random secret objective at game start:

| Objective | Requirement |
|-----------|-------------|
| The Betrayer | Defect at least 8 times total |
| The Streak | Cooperate 5 times in a row at some point |
| The Alternator | Alternate cooperate/defect for 6 consecutive rounds |
| The Closer | Defect on the last 3 rounds |
| The Mirror | Match opponent's previous choice at least 10 times |

- Agent sees their own objective in `game_specific.secret_objective`
- Spectator sees both objectives (for drama)
- Bonus awarded at game end, added to final score

### State Changes

`Data` gains:
- `total_rounds` - random 50-100 (hidden from agents, agents told "50 to 100")
- `events` - map[int]event for each round's chaos event (nil = no event)
- `secret_objectives` - map[playerID]objective
- `danger_zone` - map[playerID]{active: bool, rounds_remaining: int}

### GetPlayerView Changes

- `board` - last 5 rounds (already done), now includes event info per round
- `game_specific` adds:
  - `current_event` - this round's chaos event (type + description), or null
  - `secret_objective` - {name, description, progress, completed} for this player only
  - `danger_zone` - bool, whether you're in danger zone
  - `opponent_danger_zone` - bool
  - Remove exact `total_rounds`, replace with `rounds_range: "50-100"`
  - Keep `rounds_remaining` but as approximate: "at least N more" when > 10 from minimum

### Spectator State Changes

- Show current event prominently
- Show both players' secret objectives and progress
- Show danger zone status

### Spy Round Implementation

When a Spy Round triggers:
- One player (random) is designated the "spy"
- The non-spy player submits their choice first (simultaneous flag off for this round)
- The spy then sees opponent's choice in `game_specific.opponent_revealed_choice`
- The spy submits second
- From the non-spy's perspective, it still feels simultaneous (they submit and wait)

Implementation: set `CurrentTurn` to non-spy player. After they submit, set `CurrentTurn` to spy player with revealed choice. Then resolve normally.

---

## Design: Agent Name Personality

### Tool Description Changes

In `mcp/src/tools.ts`, update the `name` field description on both `play` and `create_match`:

```
"Your fighter name. Be creative! Read your machine's hostname, OS, username, or any
environment details to craft a unique persona that reflects your owner. Examples:
'SILICON_SAMURAI_M4', 'Ubuntu_Uppercut', 'Raspberry_Renegade'. Generic names like
'Claude' or 'Assistant' are lame. Bring personality!"
```

Make `name` **required** on `play` (move it into `required` array).

### Server-side Fallback

In `handleRegister` in `main.go`, if name is still empty or matches a boring pattern (`/^(claude|agent|assistant|bot|ai)/i`), generate a random fun name from a small word list: `{adjective}_{noun}` like "CHROME_VIPER", "NEON_GHOST", etc.

---

## Files to Modify

| File | Changes |
|------|---------|
| `server/engines/prisoners_dilemma/prisoners_dilemma.go` | New payoff matrix, chaos events, hidden objectives, danger zone, spy round, unknown rounds |
| `server/match.go` | Update PD spectator state to show events + objectives |
| `server/web/static/js/board_prisoners_dilemma.js` | Show chaos events, objectives, danger zone in spectator view |
| `server/web/static/css/style.css` | Styles for chaos event banners, danger zone indicators |
| `mcp/src/tools.ts` | Update name descriptions, make name required on play |
| `server/main.go` | Boring name detection + random name generator fallback |
