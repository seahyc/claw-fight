#!/usr/bin/env node
import { Command } from "commander";
import { fetchApi } from "./api.js";
import { loadSession, saveSession, requireSession, getServerUrl, output, sanitizeEvent } from "./session.js";
import WebSocket from "ws";

const program = new Command();

program
  .name("claw-fight")
  .description("CLI for the claw-fight game platform")
  .version("0.1.0")
  .option("--server <url>", "Game server URL (or set CLAW_FIGHT_SERVER)");

program
  .command("register")
  .description("Register a player and store session")
  .requiredOption("--name <name>", "Player name")
  .action(async (opts) => {
    const serverUrl = getServerUrl(program.opts());
    try {
      const res = await fetchApi(serverUrl, "POST", "/api/register", {
        player_name: opts.name,
      }) as { player_id: string; player_name: string };
      saveSession({
        player_id: res.player_id,
        player_name: res.player_name,
        server_url: serverUrl,
      });
      output({ player_id: res.player_id, player_name: res.player_name, session_saved: true });
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
    const session = requireSession();
    const serverUrl = getServerUrl(program.opts());
    try {
      let res: any;
      if (opts.create) {
        res = await fetchApi(serverUrl, "POST", "/api/match", {
          game_type: opts.game,
          player_id: session.player_id,
        });
        res.action = "created";
        res.status = "waiting";
        res.next_step = `Share code ${res.code} with your opponent. Then run: claw-fight listen --timeout 300`;
      } else if (opts.code) {
        res = await fetchApi(serverUrl, "POST", "/api/match/join", {
          code: opts.code,
          player_id: session.player_id,
        });
        // Signal ready
        await fetchApi(serverUrl, "POST", `/api/match/${res.match_id}/ready`, {
          player_id: session.player_id,
        });
        res.action = "joined";
        res.status = "matched";
        res.next_step = "Opponent matched! Run: claw-fight listen --timeout 300";
      } else {
        res = await fetchApi(serverUrl, "POST", "/api/match/find", {
          game_type: opts.game,
          player_id: session.player_id,
        });
        if (res.status === "matched") {
          await fetchApi(serverUrl, "POST", `/api/match/${res.match_id}/ready`, {
            player_id: session.player_id,
          });
          res.action = "joined";
          res.next_step = "Opponent matched! Run: claw-fight listen --timeout 300";
        } else {
          res.action = "created";
          res.next_step = `Waiting for opponent. Share code ${res.code} or run: claw-fight listen --timeout 300`;
        }
      }
      // Update session with match_id
      session.match_id = res.match_id;
      saveSession(session);
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("status")
  .description("Get current game state (non-blocking)")
  .option("--match <id>", "Match ID (uses session if omitted)")
  .action(async (opts) => {
    const session = requireSession();
    const serverUrl = getServerUrl(program.opts());
    const matchId = opts.match || session.match_id;
    if (!matchId) {
      output({ error: "No match ID. Join a match first or pass --match ID." });
      process.exit(1);
    }
    try {
      const res = await fetchApi(serverUrl, "GET", `/api/match/${matchId}/state?player_id=${session.player_id}`);
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
  .option("--match <id>", "Match ID (uses session if omitted)")
  .action(async (actionType, opts) => {
    const session = requireSession();
    const serverUrl = getServerUrl(program.opts());
    const matchId = opts.match || session.match_id;
    if (!matchId) {
      output({ error: "No match ID. Join a match first or pass --match ID." });
      process.exit(1);
    }
    try {
      const actionData = JSON.parse(opts.data);
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/action`, {
        player_id: session.player_id,
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
  .option("--match <id>", "Match ID (uses session if omitted)")
  .action(async (opts) => {
    const session = requireSession();
    const serverUrl = getServerUrl(program.opts());
    const matchId = opts.match || session.match_id;
    const timeout = Math.min(Math.max(parseInt(opts.timeout) || 300, 1), 300);

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
        // Register with player_id
        ws.send(JSON.stringify({ type: "register", player_id: session.player_id }));
      });

      let registered = false;

      ws.on("message", (data) => {
        const msg = JSON.parse(data.toString());

        if (msg.type === "registered") {
          registered = true;
          // Send listen request
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
  .option("--match <id>", "Match ID (uses session if omitted)")
  .action(async (message, opts) => {
    const session = requireSession();
    const serverUrl = getServerUrl(program.opts());
    const matchId = opts.match || session.match_id;
    if (!matchId) {
      output({ error: "No match ID. Join a match first or pass --match ID." });
      process.exit(1);
    }
    try {
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/chat`, {
        player_id: session.player_id,
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
  .option("--match <id>", "Match ID (uses session if omitted)")
  .action(async (opts) => {
    const session = requireSession();
    const serverUrl = getServerUrl(program.opts());
    const matchId = opts.match || session.match_id;
    if (!matchId) {
      output({ error: "No match ID." });
      process.exit(1);
    }
    try {
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/quit`, {
        player_id: session.player_id,
      });
      session.match_id = undefined;
      saveSession(session);
      output(res);
    } catch (err) {
      output({ error: err instanceof Error ? err.message : String(err) });
      process.exit(1);
    }
  });

program
  .command("end")
  .description("End the current match entirely")
  .option("--match <id>", "Match ID (uses session if omitted)")
  .action(async (opts) => {
    const session = requireSession();
    const serverUrl = getServerUrl(program.opts());
    const matchId = opts.match || session.match_id;
    if (!matchId) {
      output({ error: "No match ID." });
      process.exit(1);
    }
    try {
      const res = await fetchApi(serverUrl, "POST", `/api/match/${matchId}/end`, {
        player_id: session.player_id,
      });
      session.match_id = undefined;
      saveSession(session);
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

program.parse();
