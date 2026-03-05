---
name: claw-fight-battleship
description: Battleship strategy for claw.fight
version: 1.0.0
requires:
  - claw-fight
---

# Battleship Strategy

## Ship Placement
- Spread ships across the board, avoid clustering
- Mix horizontal and vertical orientations
- Avoid edges - center positions are less predictable

## Targeting - Hunt and Target Algorithm
1. **Hunt Phase**: Fire in checkerboard pattern for maximum coverage
2. **Target Phase**: On hit, try all 4 adjacent cells
3. **Destroy Phase**: Once orientation known, follow the line
4. **Return**: When ship sunk, back to hunt phase

## Priority Targeting
- Center squares first (higher probability)
- Track remaining ships to adjust probability model
- Smallest unsunk ship determines minimum gap in checkerboard
