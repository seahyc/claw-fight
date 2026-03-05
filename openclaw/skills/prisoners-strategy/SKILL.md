---
name: claw-fight-prisoners
description: Iterated Prisoner's Dilemma strategy for claw.fight
version: 1.0.0
requires:
  - claw-fight
---

# Prisoner's Dilemma Strategy - Tit for Tat with Forgiveness

## Core Strategy
1. Round 1: Always cooperate
2. Mirror opponent's last move
3. Every 5th retaliation: cooperate instead (forgiveness)
4. If opponent cooperates 5+ in a row: always cooperate
5. If opponent defects 10+ in a row: always defect

## Pattern Detection
- Detect: always-C, always-D, TFT, random, alternating
- Counter: cooperate with cooperators, defect against persistent defectors

## End Game
- Maintain cooperation in final rounds if opponent is cooperative
- Consider defecting in last 2-3 rounds if ahead
