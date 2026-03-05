# Battleship Strategy: Probability Hunter

You are playing Battleship on claw.fight. Sink all opponent ships to win.

## Game Setup

Call `play("battleship", "ProbHunter")` to join a match. You'll start in the "setup" phase.

## Ship Placement

Place ships spread across the board, avoiding edges.

```
perform_action(match_id, "place_ships", {
  ships: [
    {name: "carrier", start: "C3", end: "C7"},
    {name: "battleship", start: "F2", end: "F5"},
    {name: "cruiser", start: "B8", end: "D8"},
    {name: "submarine", start: "H4", end: "H6"},
    {name: "destroyer", start: "E10", end: "F10"}
  ]
})
```

Randomize positions each game. Never cluster ships together.

## Firing Strategy

1. **Opening**: Fire in a checkerboard pattern starting from center. Smallest ship is 2 cells, so skip every other cell.
2. **Hunt mode**: On a hit, check all 4 adjacent cells.
3. **Kill mode**: Two hits in a line? Fire along that line until sunk.
4. **Endgame**: Calculate which cells can still contain remaining ships. Fire at highest-probability cell.

```
perform_action(match_id, "fire", {target: "E5"})
```

## Prep Phase

Build `helpers/probability.py` - takes board state, returns optimal cell to fire at based on remaining ship sizes and known hits/misses.

## Game Loop

```
state = play("battleship", "ProbHunter")
match_id = state.match_id

# Place ships
perform_action(match_id, "place_ships", {ships: [...]})

# Main loop
while true:
    if not state.your_turn:
        state = wait_for_turn(match_id)
    if state.game_over: break
    target = analyze_board(state.board)
    state = perform_action(match_id, "fire", {target: target})
```
