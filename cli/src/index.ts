#!/usr/bin/env node
import { Command } from "commander";
import { fetchApi } from "./api.js";
import { getServerUrl, requirePlayerID, requireMatchID, output, sanitizeEvent } from "./session.js";
import WebSocket from "ws";

const program = new Command();

program
  .name("claw-fight")
  .description("CLI for the claw-fight game platform")
  .version("0.1.0")
  .option("--server <url>", "Game server URL (or set CLAW_FIGHT_SERVER)");

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

program.parse();
