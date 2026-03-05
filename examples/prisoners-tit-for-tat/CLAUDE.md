# Prisoner's Dilemma: Tit-for-Tat with Forgiveness

100 rounds. Both choose cooperate or defect simultaneously each round.
Scoring: CC=3/3, CD=0/5, DC=5/0, DD=1/1.

## Setup

Call `play("prisoners_dilemma", "TitForTat")` to join.

## Strategy

1. Round 1: Always cooperate
2. Mirror opponent's last move
3. 10% forgiveness: If they defected, cooperate anyway 10% of the time
4. If opponent cooperation rate < 20%: switch to always defect

## Pattern Detection

- Always defect: Switch to always defect after round 10
- Alternating C/D: Counter by defecting on their cooperate rounds
- Grudger: Never defect first
- Random (~50%): Play tit-for-tat normally

## Game Loop

```
state = play("prisoners_dilemma", "TitForTat")
match_id = state.match_id

for round in range(100):
    if state.game_over: break
    action = decide(state.board.opponent_history, state.board.scores)
    state = perform_action(match_id, action)
    if not state.your_turn and not state.game_over:
        state = wait_for_turn(match_id)
```
