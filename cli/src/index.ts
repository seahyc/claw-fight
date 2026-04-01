#!/usr/bin/env node
import { Command } from "commander";
import os from "os";
import { fetchApi } from "./api.js";
import { getServerUrl, requirePlayerID, requireMatchID, output, sanitizeEvent } from "./session.js";
import WebSocket from "ws";
import { createRequire } from "module";
const _require = createRequire(import.meta.url);
const pkg = _require("../package.json") as { version: string };

const program = new Command();

program
  .name("claw-fight")
  .description("CLI for the claw-fight game platform")
  .version("0.1.0")
  .option("--server <url>", "Game server URL (or set CLAW_FIGHT_SERVER)");

program
  .command("play [game_type]")
  .description("Quick start: register + join a game in one step. Without game_type, lists available games.")
  .option("--name <name>", "Player name (default: auto-generated)")
  .option("--create", "Create a private match and print the challenge code")
  .option("--code <code>", "Join a match by challenge code")
  .action(async (gameType, opts) => {
    const serverUrl = getServerUrl(program.opts());
    try {
      // If no game type, list available games
      if (!gameType && !opts.code) {
        const games = await fetchApi(serverUrl, "GET", "/api/games") as Array<{ name: string; rules: string }>;
        output({
          available_games: games.map((g: { name: string; rules: string }) => ({
            name: g.name,
            command: `npx claw-fight play ${g.name}`,
          })),
          hint: "Pick a game and run the command above",
        });
        return;
      }

      // Auto-register if no player ID set
      let playerID = process.env.CLAW_FIGHT_PLAYER_ID;
      if (!playerID) {
        const playerName = opts.name || `AGENT_${os.hostname().replace(/\./g, "_").toUpperCase()}`;
        const regRes = await fetchApi(serverUrl, "POST", "/api/register", {
          player_name: playerName,
        }) as { player_id: string; player_name: string };
        playerID = regRes.player_id;
      }

      // Join or create match
      let res: any;
      if (opts.create) {
        res = await fetchApi(serverUrl, "POST", "/api/match", {
          game_type: gameType || "battleship",
          player_id: playerID,
        });
        res.action = "created";
        res.status = "waiting";
      } else if (opts.code) {
        res = await fetchApi(serverUrl, "POST", "/api/match/join", {
          code: opts.code,
          player_id: playerID,
        });
        await fetchApi(serverUrl, "POST", `/api/match/${res.match_id}/ready`, {
          player_id: playerID,
        });
        res.action = "joined";
        res.status = "matched";
      } else {
        res = await fetchApi(serverUrl, "POST", "/api/match/find", {
          game_type: gameType,
          player_id: playerID,
        });
        if (res.status === "matched") {
          await fetchApi(serverUrl, "POST", `/api/match/${res.match_id}/ready`, {
            player_id: playerID,
          });
          res.action = "joined";
        } else {
          res.action = "created";
        }
      }

      res.player_id = playerID;
      res.env_setup = `export CLAW_FIGHT_PLAYER_ID=${playerID} CLAW_FIGHT_MATCH_ID=${res.match_id}`;
      if (res.code && res.action === "created") {
        res.share = `Share this code with your opponent: ${res.code}`;
      }
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("register")
  .description("Register a player and print player_id")
  .requiredOption("--name <name>", "Player name")
  .action(async (opts) => {
    const serverUrl = getServerUrl(program.opts());
    try {
      const res = await fetchApi(serverUrl, "POST", "/api/register", {
        player_name: opts.name,
      }) as { player_id: string; player_name: string };
      output({
        player_id: res.player_id,
        player_name: res.player_name,
        next_step: `export CLAW_FIGHT_PLAYER_ID=${res.player_id}`,
      });
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("join")
  .description("Join or create a match via matchmaking or challenge code")
  .option("--game <type>", "Game type: battleship, poker, prisoners_dilemma", "battleship")
  .option("--code <code>", "Challenge code to join a specific match")
  .option("--create", "Create a new private match and print the challenge code (don't auto-matchmake)")
  .action(async (opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    try {
      let res: any;
      if (opts.create) {
        res = await fetchApi(serverUrl, "POST", "/api/match", {
          game_type: opts.game,
          player_id: playerID,
        });
        res.action = "created";
        res.status = "waiting";
        res.next_step = `export CLAW_FIGHT_MATCH_ID=${res.match_id} # then share code ${res.code} with your opponent`;
      } else if (opts.code) {
        res = await fetchApi(serverUrl, "POST", "/api/match/join", {
          code: opts.code,
          player_id: playerID,
        });
        await fetchApi(serverUrl, "POST", `/api/match/${res.match_id}/ready`, {
          player_id: playerID,
        });
        res.action = "joined";
        res.status = "matched";
        res.next_step = `export CLAW_FIGHT_MATCH_ID=${res.match_id}`;
      } else {
        res = await fetchApi(serverUrl, "POST", "/api/match/find", {
          game_type: opts.game,
          player_id: playerID,
        });
        if (res.status === "matched") {
          await fetchApi(serverUrl, "POST", `/api/match/${res.match_id}/ready`, {
            player_id: playerID,
          });
          res.action = "joined";
          res.next_step = `export CLAW_FIGHT_MATCH_ID=${res.match_id}`;
        } else {
          res.action = "created";
          res.next_step = `export CLAW_FIGHT_MATCH_ID=${res.match_id} # waiting for opponent, share code ${res.code}`;
        }
      }
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("status")
  .description("Get current game state (non-blocking)")
  .option("--match <id>", "Match ID (or set CLAW_FIGHT_MATCH_ID)")
  .action(async (opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    const matchId = requireMatchID(opts);
    try {
      const res = await fetchApi(serverUrl, "GET", `/api/match/${matchId}/state?player_id=${playerID}`);
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("action <type>")
  .description("Submit a move in the current match")
  .option("--data <json>", "Action data as JSON", "{}")
  .option("--match <id>", "Match ID (or set CLAW_FIGHT_MATCH_ID)")
  .action(async (actionType, opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    const matchId = requireMatchID(opts);
    try {
      const actionData = JSON.parse(opts.data);
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/action`, {
        player_id: playerID,
        action_type: actionType,
        action_data: actionData,
      });
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("listen")
  .description("Blocking wait for game events via WebSocket")
  .option("--timeout <seconds>", "Max seconds to wait", "300")
  .option("--match <id>", "Match ID (or set CLAW_FIGHT_MATCH_ID)")
  .action(async (opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    const matchId = requireMatchID(opts);
    const timeout = Math.min(Math.max(parseInt(opts.timeout) || 300, 1), 300);

    // Check current state first — if it's already our turn, return immediately
    // (events sent before WS connects are lost, so this catches missed turns)
    try {
      const state = await fetchApi(serverUrl, "GET", `/api/match/${matchId}/state?player_id=${playerID}`) as Record<string, unknown>;
      if (state.your_turn === true || state.phase === "finished") {
        output({ events: [sanitizeEvent(state)] });
        process.exit(0);
      }
    } catch (err) {
      // 404 = match not found (waiting for opponent) — continue to WS listen
      // Other errors = also fall through to WS, which will retry or timeout
    }

    const wsUrl = serverUrl.replace(/^http/, "ws") + "/ws";

    try {
      const ws = new WebSocket(wsUrl);

      const cleanup = () => {
        try { ws.close(); } catch {}
      };

      const failTimer = setTimeout(() => {
        output({ events: [] });
        cleanup();
        process.exit(0);
      }, (timeout + 15) * 1000);

      ws.on("open", () => {
        ws.send(JSON.stringify({ type: "register", player_id: playerID }));
      });

      ws.on("message", (data) => {
        const msg = JSON.parse(data.toString());

        if (msg.type === "registered") {
          ws.send(JSON.stringify({
            type: "listen",
            match_id: matchId,
            types: ["your_turn", "game_over", "action_result", "opponent_action", "chat", "match_ended", "match_found"],
            timeout,
          }));
          return;
        }

        if (msg.type === "events") {
          clearTimeout(failTimer);
          const events = (msg.events || []).map((e: Record<string, unknown>) => sanitizeEvent(e));
          output({ events });
          cleanup();
          process.exit(0);
        }

        if (msg.type === "error") {
          clearTimeout(failTimer);
          output({ error: msg.message });
          cleanup();
          process.exit(1);
        }
      });

      ws.on("error", (err) => {
        clearTimeout(failTimer);
        output({ error: `WebSocket error: ${err.message}` });
        process.exit(1);
      });

      ws.on("close", () => {
        clearTimeout(failTimer);
      });
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("chat <message>")
  .description("Send an in-game chat message")
  .option("--match <id>", "Match ID (or set CLAW_FIGHT_MATCH_ID)")
  .action(async (message, opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    const matchId = requireMatchID(opts);
    try {
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/chat`, {
        player_id: playerID,
        message,
      });
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("quit")
  .description("Leave the current match")
  .option("--match <id>", "Match ID (or set CLAW_FIGHT_MATCH_ID)")
  .action(async (opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    const matchId = requireMatchID(opts);
    try {
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/quit`, {
        player_id: playerID,
      });
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("end")
  .description("End the current match entirely")
  .option("--match <id>", "Match ID (or set CLAW_FIGHT_MATCH_ID)")
  .action(async (opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    const matchId = requireMatchID(opts);
    try {
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/end`, {
        player_id: playerID,
      });
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("rules [game_type]")
  .description("Get game rules")
  .action(async (gameType) => {
    const serverUrl = getServerUrl(program.opts());
    try {
      if (gameType) {
        const res = await fetchApi(serverUrl, "GET", `/api/game/${gameType}/rules`);
        output(res);
      } else {
        const res = await fetchApi(serverUrl, "GET", "/api/games");
        output(res);
      }
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

// ---------------------------------------------------------------------------
// Feature 1: Auto-update check
// ---------------------------------------------------------------------------

async function checkForUpdates() {
  // Skip if running via npx (always latest)
  if (process.env.npm_execpath?.includes('npx') || process.env._?.includes('npx')) return;

  try {
    const current = pkg.version;
    const res = await fetch('https://registry.npmjs.org/claw-fight/latest', { signal: AbortSignal.timeout(3000) });
    if (!res.ok) return;
    const data = await res.json() as { version: string };
    const latest = data.version;
    if (latest === current) return;

    process.stderr.write(`Updating claw-fight ${current} → ${latest}...\n`);
    const { execSync } = await import('child_process');
    execSync('npm install -g claw-fight@latest', { stdio: 'inherit' });

    // Re-exec with updated version
    execSync(process.argv.slice(1).join(' '), { stdio: 'inherit' });
    process.exit(0);
  } catch {
    // Silent fail - don't block CLI on network errors
  }
}

// ---------------------------------------------------------------------------
// Feature 2: `next` command
// ---------------------------------------------------------------------------

function parseAction(doArg: string): { type: string; data: Record<string, unknown> } {
  const parts = doArg.trim().split(/\s+/);
  const cmd = parts[0].toLowerCase();
  const rest = parts.slice(1);

  switch (cmd) {
    case 'fold':
    case 'check':
    case 'call':
    case 'all_in':
      return { type: cmd, data: {} };

    case 'raise':
    case 'bet':
      return { type: cmd, data: { amount: parseInt(rest[0], 10) } };

    case 'fire':
      return { type: 'fire', data: { target: rest[0] } };

    case 'mark':
      return { type: 'mark', data: { position: parseInt(rest[0], 10) } };

    case 'cooperate':
      return { type: 'choose', data: { choice: 'cooperate' } };

    case 'defect':
      return { type: 'choose', data: { choice: 'defect' } };

    case 'place_ships': {
      // parse "carrier:A1-A5 battleship:C3-F3 ..."
      const ships: Record<string, string> = {};
      for (const token of rest) {
        const [name, coords] = token.split(':');
        if (name && coords) ships[name] = coords;
      }
      return { type: 'place_ships', data: { ships } };
    }

    default:
      return { type: cmd, data: {} };
  }
}

function renderBoard(event: Record<string, unknown>): string {
  const lines: string[] = [];
  const gameType = (event.game_type as string | undefined) || '';
  const phase = (event.phase as string | undefined) || '';
  const gs = (event.game_specific as Record<string, unknown> | undefined) || {};

  const formatGameType = (t: string) => {
    if (t === 'prisoners_dilemma') return "PRISONER'S DILEMMA";
    return t.replace(/_/g, ' ').toUpperCase();
  };

  // Build header
  let header = formatGameType(gameType);
  if (phase === 'setup') {
    header += '  •  Setup phase';
  } else if (phase === 'play' || phase === 'fire') {
    const turn = gs.turn_number as number | undefined;
    const hand = gs.hand_number as number | undefined;
    const handTotal = gs.total_hands as number | undefined;
    const roundNum = gs.round as number | undefined;
    const subPhase = gs.sub_phase as string | undefined;
    if (hand !== undefined && handTotal !== undefined) {
      header += `  •  Hand ${hand}/${handTotal}`;
    } else if (roundNum !== undefined) {
      header += `  •  Round ${roundNum}`;
    } else if (turn !== undefined) {
      header += `  •  Turn ${turn}`;
    }
    if (subPhase) header += `  •  ${subPhase.charAt(0).toUpperCase() + subPhase.slice(1)}`;
  }

  lines.push(header);
  lines.push('');

  const playerRole = (event.player_role as string | undefined) || 'p1';
  const opponentRole = playerRole === 'p1' ? 'p2' : 'p1';

  if (gameType === 'poker') {
    const community = gs.community_cards as string[] | undefined;
    const hand = gs.hand as string[] | undefined;
    const pot = gs.pot as number | undefined;
    const chips = gs.chips as Record<string, number> | undefined;
    const playerNames = gs.player_names as Record<string, string> | undefined;

    if (community && community.length > 0) {
      lines.push(`  Community: ${community.join('  ')}`);
    }
    if (hand && hand.length > 0) {
      lines.push(`  Your hand: ${hand.join('  ')}`);
    }

    if (pot !== undefined || chips) {
      const parts: string[] = [];
      if (pot !== undefined) parts.push(`Pot: ${pot}`);
      if (chips) {
        const myChips = chips[playerRole];
        const oppChips = chips[opponentRole];
        const myLabel = playerNames?.[playerRole] || playerRole.toUpperCase();
        const oppLabel = playerNames?.[opponentRole] || opponentRole.toUpperCase();
        if (myChips !== undefined) parts.push(`You (${myLabel}): ${myChips} chips`);
        if (oppChips !== undefined) parts.push(`Opponent (${oppLabel}): ${oppChips} chips`);
      }
      lines.push(`  ${parts.join('  |  ')}`);
    }
  } else if (gameType === 'battleship') {
    if (phase === 'setup') {
      lines.push('  Place your 5 ships. Ships: carrier(5), battleship(4), cruiser(3), submarine(3), destroyer(2)');
      lines.push('  Coordinates: A1-J10. Ships must be horizontal or vertical.');
    } else {
      // Show hit/miss summary from board if available
      const board = event.board as Record<string, unknown> | undefined;
      if (board) {
        const myHits = board.opponent_hits as string[] | undefined;  // hits on opponent's board
        const oppHits = board.my_hits as string[] | undefined;       // hits on our board
        if (myHits && myHits.length > 0) {
          lines.push(`  Your hits: ${myHits.join(', ')}`);
        }
        if (oppHits && oppHits.length > 0) {
          lines.push(`  Opponent hits on you: ${oppHits.join(', ')}`);
        }
      }
      lines.push('  Available: fire at any unrevealed coordinate (A1-J10)');
    }
  } else if (gameType === 'prisoners_dilemma') {
    const scores = gs.scores as Record<string, number> | undefined;
    if (scores) {
      const myScore = scores[playerRole];
      const oppScore = scores[opponentRole];
      if (myScore !== undefined && oppScore !== undefined) {
        lines.push(`  Scores: You ${myScore}  |  Opponent ${oppScore}`);
      }
    }
  }

  lines.push('');

  // Generate runnable action commands
  const availableActions = (event.available_actions as string[] | undefined) || [];
  const actionsWithParams = new Set(['bet', 'raise', 'fire', 'mark']);

  if (gameType === 'battleship' && phase === 'setup') {
    lines.push('  npx claw-fight next --do "place_ships carrier:A1-A5 battleship:C3-F3 cruiser:E5-E7 submarine:G1-G3 destroyer:I9-I10"');
  } else {
    for (const action of availableActions) {
      if (actionsWithParams.has(action)) {
        if (action === 'fire') {
          lines.push(`  npx claw-fight next --do "fire B5"`);
          lines.push(`  npx claw-fight next --do "fire C7"`);
        } else if (action === 'bet' || action === 'raise') {
          lines.push(`  npx claw-fight next --do "${action} <amount>"`);
        } else if (action === 'mark') {
          lines.push(`  npx claw-fight next --do "mark <position>"`);
        }
      } else {
        lines.push(`  npx claw-fight next --do "${action}"`);
      }
    }
  }

  return lines.join('\n');
}

program
  .command('next')
  .description('Wait for your turn and print the board state with runnable action commands')
  .option('--match <id>', 'Match ID (or set CLAW_FIGHT_MATCH_ID)')
  .option('--do <action>', 'Submit an action first, then poll for next turn')
  .action(async (opts) => {
    const playerID = requirePlayerID();
    const serverUrl = getServerUrl(program.opts());
    const matchId = opts.match || process.env.CLAW_FIGHT_MATCH_ID;

    if (!matchId) {
      process.stderr.write(
        'Error: No match ID. Set CLAW_FIGHT_MATCH_ID or pass --match <id>.\n' +
        'To start a game: npx claw-fight play <game_type>\n'
      );
      process.exit(1);
    }

    // Submit action if --do was provided
    if (opts.do) {
      const parsed = parseAction(opts.do);
      try {
        await fetchApi(serverUrl, 'POST', `/api/match/${matchId}/action`, {
          player_id: playerID,
          action_type: parsed.type,
          action_data: parsed.data,
        });
        process.stderr.write(`✓ ${opts.do}\n`);
      } catch (err) {
        process.stderr.write(`Error submitting action: ${err instanceof Error ? err.message : String(err)}\n`);
        process.exit(1);
      }
    }

    process.stderr.write('Waiting for your turn...\n');

    // Poll loop
    // 6-minute upper timeout once game is active; no upper timeout waiting for opponent to join
    let gameActive = false;
    const SIX_MINUTES = 6 * 60 * 1000;
    const startTime = Date.now();

    while (true) {
      // Check 6-min timeout if game is active
      if (gameActive && Date.now() - startTime > SIX_MINUTES) {
        process.stderr.write('Timeout: no turn received within 6 minutes.\n');
        process.exit(1);
      }

      let data: Record<string, unknown>;
      try {
        data = await fetchApi(
          serverUrl,
          'GET',
          `/api/match/${matchId}/poll?player_id=${playerID}&timeout=55`
        ) as Record<string, unknown>;
      } catch (err) {
        process.stderr.write(`Poll error: ${err instanceof Error ? err.message : String(err)}\n`);
        process.exit(1);
      }

      const eventType = data.type as string | undefined;

      // Empty / timeout — retry silently
      if (!eventType || eventType === 'timeout' || eventType === 'no_event') {
        continue;
      }

      // Silently skip these event types
      if (eventType === 'opponent_action' || eventType === 'chat') {
        continue;
      }

      // Match found / game started — mark active and continue polling
      if (eventType === 'match_found' || eventType === 'game_started') {
        gameActive = true;
        continue;
      }

      if (eventType === 'your_turn') {
        gameActive = true;
        const board = renderBoard(data);
        process.stdout.write(board + '\n');
        process.exit(0);
      }

      if (eventType === 'game_over' || eventType === 'match_ended') {
        const winner = data.winner as string | undefined;
        const reason = data.reason as string | undefined;
        const yourRole = data.player_role as string | undefined;

        let result: string;
        if (!winner) {
          result = 'Draw';
        } else if (yourRole && winner === yourRole) {
          result = 'You win!';
        } else {
          result = 'Opponent wins';
        }

        process.stdout.write(`GAME OVER  —  ${result}\n`);
        if (reason) process.stdout.write(`  ${reason}\n`);
        process.stdout.write(`  Replay: https://clawfight.live/match/${matchId}\n`);
        process.exit(1);
      }

      // Unknown event — continue polling
    }
  });

await checkForUpdates();
program.parse();
