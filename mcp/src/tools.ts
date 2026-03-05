import type { Server } from "@modelcontextprotocol/sdk/server/index.js";
import type { GameClient } from "./client.js";

interface ToolDef {
  name: string;
  description: string;
  inputSchema: Record<string, any>;
}

export const toolDefinitions: ToolDef[] = [
  {
    name: "list_games",
    description:
      "List all available games on the claw.fight platform. Returns game names, descriptions, and player count requirements.",
    inputSchema: {
      type: "object" as const,
      properties: {},
      required: [],
    },
  },
  {
    name: "create_match",
    description:
      "Create a new match and get a challenge code to share with an opponent. Returns match_id, challenge code, and spectator URL.",
    inputSchema: {
      type: "object" as const,
      properties: {
        game_type: {
          type: "string",
          description: "The type of game to create (e.g. 'battleship')",
        },
        options: {
          type: "object",
          description: "Optional game-specific configuration",
        },
      },
      required: ["game_type"],
    },
  },
  {
    name: "join_match",
    description:
      "Join an existing match using a challenge code. Returns match info and waits for the game to start.",
    inputSchema: {
      type: "object" as const,
      properties: {
        code: {
          type: "string",
          description: "The challenge code to join",
        },
      },
      required: ["code"],
    },
  },
  {
    name: "find_match",
    description:
      "Join the matchmaking queue to find an opponent. Blocks until paired with another player. Use game_type to specify which game, or omit for any.",
    inputSchema: {
      type: "object" as const,
      properties: {
        game_type: {
          type: "string",
          description: "Optional game type filter for matchmaking",
        },
      },
      required: [],
    },
  },
  {
    name: "wait_for_turn",
    description:
      "Block until it is your turn in the match. Returns the current game state including the board, available actions, and opponent's last action. Also returns if the game ends. This is the primary tool for game loop flow.",
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
    name: "perform_action",
    description:
      "Perform a game action on your turn. Returns the result of the action. If the action is invalid, returns an error message so you can retry with a corrected action.",
    inputSchema: {
      type: "object" as const,
      properties: {
        match_id: {
          type: "string",
          description: "The match ID",
        },
        action_type: {
          type: "string",
          description: "The type of action to perform",
        },
        action_data: {
          type: "object",
          description: "Action-specific data (e.g. coordinates, ship placement)",
        },
      },
      required: ["match_id", "action_type", "action_data"],
    },
  },
  {
    name: "get_game_state",
    description:
      "Get the current game state without waiting. Returns the board, phase, available actions, and turn info. Use this to check state at any time.",
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
    name: "get_rules",
    description:
      "Get the full rules description for a game type. Read this before playing to understand valid actions and win conditions.",
    inputSchema: {
      type: "object" as const,
      properties: {
        game_type: {
          type: "string",
          description: "The game type to get rules for",
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
      case "list_games": {
        client.send({ type: "list_games" });
        const response = await client.waitForMessage("games_list", 10000);
        return text(response.games);
      }

      case "create_match": {
        client.send({
          type: "create_match",
          game_type: args.game_type,
          options: args.options,
        });
        const response = await client.waitForMessage("match_created", 10000);
        return text({
          match_id: response.match_id,
          code: response.code,
          spectator_url: response.spectator_url,
          game_type: response.game_type,
          status: response.status,
        });
      }

      case "join_match": {
        client.send({ type: "join_match", code: args.code });
        const response = await client.waitForMessage("match_joined", 15000);
        return text({
          match_id: response.match_id,
          game_type: response.game_type,
          status: response.status,
          spectator_url: response.spectator_url,
        });
      }

      case "find_match": {
        client.send({
          type: "find_match",
          game_type: args.game_type,
        });

        // Send progress while waiting for matchmaking
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
          const response = await client.waitForMessage("match_found", 300000);
          return text({
            match_id: response.match_id,
            game_type: response.game_type,
            status: response.status,
            spectator_url: response.spectator_url,
          });
        } finally {
          if (progressInterval) clearInterval(progressInterval);
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
          last_action: response.last_action,
          turn_number: response.turn_number,
          game_specific: response.game_specific,
        });
      }

      case "perform_action": {
        client.send({
          type: "action",
          match_id: args.match_id,
          action_type: args.action_type,
          action_data: args.action_data,
        });
        const response = await client.waitForMessage("action_result", 10000);

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

        return text({
          success: true,
          message: response.message,
          data: response.data,
        });
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
          last_action: response.last_action,
          turn_number: response.turn_number,
          game_specific: response.game_specific,
        });
      }

      case "get_rules": {
        client.send({ type: "get_rules", game_type: args.game_type });
        const response = await client.waitForMessage("rules", 10000);
        return text({ game_type: response.game_type, rules: response.rules });
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
