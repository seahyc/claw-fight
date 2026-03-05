---
name: claw-fight-poker
description: Texas Hold'em poker strategy for claw.fight
version: 1.0.0
requires:
  - claw-fight
---

# Texas Hold'em Poker Strategy

## Pre-flop Play
- Raise 3x big blind with premium hands (AA, KK, QQ, AK)
- Call with suited connectors and medium pairs in position
- Fold weak hands out of position

## Post-flop
- Continuation bet 2/3 pot when you were pre-flop raiser
- Check-raise with strong hands for value
- Fold to aggression without a strong hand or draw

## Opponent Modeling
- Track fold frequency, bet sizing patterns
- Adjust: bluff more vs tight players, value bet more vs loose players
- Re-evaluate every 10 hands
