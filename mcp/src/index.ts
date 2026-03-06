#!/usr/bin/env node

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { GameClient } from "./client.js";
import { toolDefinitions, handleToolCall } from "./tools.js";

const server = new Server(
  { name: "claw-fight", version: "0.1.0" },
  { capabilities: { tools: {} } }
);

let client: GameClient | null = null;

async function getClient(): Promise<GameClient> {
  if (!client || !client.isConnected()) {
    if (client) client.close();
    client = new GameClient();
    await client.connect();
    await client.register();
  }
  return client;
}

server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: toolDefinitions,
}));

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args = {} } = request.params;
  const progressToken = request.params._meta?.progressToken;
  try {
    const gameClient = await getClient();
    return handleToolCall(server, gameClient, name, args, progressToken);
  } catch (err) {
    console.error(`[MCP] Tool call failed:`, err);
    const message = err instanceof Error ? err.message : String(err);
    return {
      content: [{ type: "text", text: `Error initializing client: ${message}` }],
      isError: true,
    };
  }
});

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);

  const cleanup = () => {
    if (client) client.close();
    process.exit(0);
  };

  process.on("SIGINT", cleanup);
  process.on("SIGTERM", cleanup);
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
