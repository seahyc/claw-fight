import type { Server } from "@modelcontextprotocol/sdk/server/index.js";
import type { GameClient } from "./client.js";

interface ToolDef {
  name: string;
  description: string;
  inputSchema: Record<string, any>;
}

export const toolDefinitions: ToolDef[] = [
  {
    name: "play",
    description:
      "Join or create a game match. Smart entry point: with a code, joins that specific match; without, joins an open match or creates a new one and waits. Blocks until matched with an opponent and the game starts. Returns the match ID, spectator URL, and initial game state.",
    inputSchema: {
      type: "object" as const,
      properties: {
        game_type: {
          type: "string",
          description:
            "Game to play: 'battleship', 'poker', or 'prisoners_dilemma'",
        },
        name: {
          type: "string",
          description: "Display name for your agent (e.g. 'DeepBlue')",
        },
        code: {
          type: "string",
          description:
            "Challenge code to join a specific match. If omitted, auto-matches.",
        },
      },
      required: ["game_type"],
    },
  },
  {
    name: "perform_action",
    description:
      "Submit your move in a match. Returns the result of the action and the updated game state. If the action is invalid, returns an error - retry with a corrected action.",
    inputSchema: {
      type: "object" as const,
      properties: {
        match_id: {
          type: "string",
          description: "The match ID",
        },
        action_type: {
          type: "string",
          description:
            "Action type (game-specific, e.g. 'fire', 'place_ships', 'check', 'bet', 'cooperate', 'defect')",
        },
        action_data: {
          type: "object",
          description:
            "Action-specific data (e.g. {target:'B5'} for fire, {amount:100} for bet)",
        },
      },
      required: ["match_id", "action_type"],
    },
  },
  {
    name: "wait_for_turn",
    description:
      "Block until it's your turn. Returns the current game state (board, available actions) or game over result. Use this after perform_action says it's the opponent's turn.",
    inputSchema: {
      type: "object" as const,
      properties: {
        match_id: {
          type: "string",
          description: "The match ID to wait for",
        },
      },
      required: ["match_id"],
    },
  },
  {
    name: "get_rules",
    description:
      "Get rules for a specific game type, or list all available games if game_type is omitted. Call this before playing to understand valid actions and win conditions.",
    inputSchema: {
      type: "object" as const,
      properties: {
        game_type: {
          type: "string",
          description:
            "Game type to get rules for. Omit to list all available games.",
        },
      },
      required: [],
    },
  },
  {
    name: "get_game_state",
    description:
      "Non-blocking check of current game state. Returns board, phase, available actions, and turn info.",
    inputSchema: {
      type: "object" as const,
      properties: {
        match_id: {
          type: "string",
          description: "The match ID",
        },
      },
      required: ["match_id"],
    },
  },
  {
    name: "create_match",
    description:
      "Create a match with a challenge code to share with a specific opponent (e.g. via hotline). The opponent joins using play(code=...). For auto-matching, use play() instead.",
    inputSchema: {
      type: "object" as const,
      properties: {
        game_type: {
          type: "string",
          description: "Game type to create",
        },
        name: {
          type: "string",
          description: "Display name for your agent",
        },
      },
      required: ["game_type"],
    },
  },
];

export async function handleToolCall(
  server: Server,
  client: GameClient,
  name: string,
  args: Record<string, any>,
  progressToken?: string | number
): Promise<{ content: Array<{ type: "text"; text: string }>; isError?: boolean }> {
  try {
    switch (name) {
      case "play": {
        const gameType = args.game_type as string;
        const playerName = args.name as string | undefined;
        const code = args.code as string | undefined;

        // Re-register with name if provided
        if (playerName) {
          await client.register(playerName);
        }

        let matchId: string;
        let spectatorUrl: string;

        if (code) {
          // Join specific match by code
          client.send({ type: "join_match", code });
          const joined = await client.waitForMessage("match_joined", 15000);
          matchId = joined.match_id;
          spectatorUrl = joined.spectator_url;
        } else {
          // Smart flow: find_match (server checks open matches, then queues)
          client.send({ type: "find_match", game_type: gameType });

          // Progress while waiting for opponent
          const progressInterval = progressToken
            ? setInterval(() => {
                server.notification({
                  method: "notifications/progress",
                  params: {
                    progressToken: progressToken!,
                    progress: 0,
                    total: 0,
                    message: "Searching for opponent...",
                  },
                });
              }, 10000)
            : undefined;

          try {
            const found = await client.waitForMessage("match_found", 300000);
            matchId = found.match_id;
            spectatorUrl = found.spectator_url || `/match/${matchId}`;
          } finally {
            if (progressInterval) clearInterval(progressInterval);
          }
        }

        // Signal ready
        client.send({ type: "ready", match_id: matchId });

        // Wait for game to start and get initial state
        // Progress while waiting for game start
        const startProgress = progressToken
          ? setInterval(() => {
              server.notification({
                method: "notifications/progress",
                params: {
                  progressToken: progressToken!,
                  progress: 0,
                  total: 0,
                  message: "Waiting for game to start...",
                },
              });
            }, 10000)
          : undefined;

        try {
          // Wait for your_turn (initial game state) or game_start then your_turn
          const state = await client.waitForTurn(matchId, (msg) => {
            if (progressToken) {
              server.notification({
                method: "notifications/progress",
                params: {
                  progressToken: progressToken!,
                  progress: 0,
                  total: 0,
                  message: msg,
                },
              });
            }
          });

          return text({
            match_id: matchId,
            spectator_url: spectatorUrl,
            phase: state.phase,
            your_turn: state.your_turn,
            simultaneous: state.simultaneous,
            board: state.board,
            available_actions: state.available_actions,
            turn_number: state.turn_number,
            game_specific: state.game_specific,
          });
        } finally {
          if (startProgress) clearInterval(startProgress);
        }
      }

      case "perform_action": {
        client.send({
          type: "action",
          match_id: args.match_id,
          action_type: args.action_type,
          action_data: args.action_data || {},
        });

        const response = await client.waitForMessage("action_result", 30000);

        if (!response.success) {
          return {
            content: [
              {
                type: "text",
                text: JSON.stringify(
                  {
                    success: false,
                    message: response.message,
                    data: response.data,
                  },
                  null,
                  2
                ),
              },
            ],
            isError: true,
          };
        }

        // After a successful action, wait briefly for the next your_turn or game_over
        try {
          const next = await client.waitForTurn(args.match_id, () => {});

          if (next.type === "game_over") {
            return text({
              success: true,
              message: response.message,
              data: response.data,
              game_over: true,
              winner: next.winner,
              draw: next.draw,
              scores: next.scores,
              reason: next.reason,
            });
          }

          return text({
            success: true,
            message: response.message,
            data: response.data,
            your_turn: next.your_turn,
            phase: next.phase,
            board: next.board,
            available_actions: next.available_actions,
            turn_number: next.turn_number,
            game_specific: next.game_specific,
          });
        } catch {
          // If we can't get next state quickly, just return the action result
          return text({
            success: true,
            message: response.message,
            data: response.data,
          });
        }
      }

      case "wait_for_turn": {
        const progressCallback = (msg: string) => {
          if (progressToken) {
            server.notification({
              method: "notifications/progress",
              params: {
                progressToken: progressToken!,
                progress: 0,
                total: 0,
                message: msg,
              },
            });
          }
        };

        const response = await client.waitForTurn(
          args.match_id,
          progressCallback
        );

        if (response.type === "game_over") {
          return text({
            game_over: true,
            winner: response.winner,
            draw: response.draw,
            scores: response.scores,
            reason: response.reason,
          });
        }

        return text({
          phase: response.phase,
          your_turn: response.your_turn,
          simultaneous: response.simultaneous,
          board: response.board,
          available_actions: response.available_actions,
          turn_number: response.turn_number,
          game_specific: response.game_specific,
        });
      }

      case "get_rules": {
        if (args.game_type) {
          client.send({ type: "get_rules", game_type: args.game_type });
          const response = await client.waitForMessage("rules", 10000);
          return text({ game_type: response.game_type, rules: response.rules });
        } else {
          client.send({ type: "list_games" });
          const response = await client.waitForMessage("games_list", 10000);
          return text({
            games: response.games,
            open_matches: response.open_matches,
          });
        }
      }

      case "get_game_state": {
        client.send({ type: "get_state", match_id: args.match_id });
        const response = await client.waitForMessage("game_state", 10000);
        return text({
          phase: response.phase,
          your_turn: response.your_turn,
          simultaneous: response.simultaneous,
          board: response.board,
          available_actions: response.available_actions,
          turn_number: response.turn_number,
          game_specific: response.game_specific,
        });
      }

      case "create_match": {
        if (args.name) {
          await client.register(args.name);
        }
        client.send({
          type: "create_match",
          game_type: args.game_type,
        });
        const response = await client.waitForMessage("match_created", 10000);
        return text({
          match_id: response.match_id,
          code: response.code,
          spectator_url: response.spectator_url,
          message:
            "Match created. Share the code with your opponent. They join with: play(game_type, code='" +
            response.code +
            "')",
        });
      }

      default:
        return {
          content: [{ type: "text", text: `Unknown tool: ${name}` }],
          isError: true,
        };
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return {
      content: [{ type: "text", text: `Error: ${message}` }],
      isError: true,
    };
  }
}

function text(data: any) {
  return {
    content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }],
  };
}
