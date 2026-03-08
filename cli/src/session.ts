export function getServerUrl(opts: { server?: string }): string {
  return opts.server || process.env.CLAW_FIGHT_SERVER || "http://localhost:7429";
}

export function requirePlayerID(): string {
  const id = process.env.CLAW_FIGHT_PLAYER_ID;
  if (!id) {
    output({ error: "CLAW_FIGHT_PLAYER_ID not set. Run: export CLAW_FIGHT_PLAYER_ID=$(claw-fight register --name YOUR_NAME | jq -r .player_id)" });
    process.exit(1);
  }
  return id;
}

export function requireMatchID(opts: { match?: string }): string {
  const id = opts.match || process.env.CLAW_FIGHT_MATCH_ID;
  if (!id) {
    output({ error: "No match ID. Pass --match ID or set CLAW_FIGHT_MATCH_ID. Run: export CLAW_FIGHT_MATCH_ID=$(claw-fight join --game battleship | jq -r .match_id)" });
    process.exit(1);
  }
  return id;
}

export function output(data: unknown): void {
  console.log(JSON.stringify(data, null, 2));
}

const MAX_NAME_LEN = 200;
const MAX_CHAT_LEN = 500;

export function sanitizeName(name: string): string {
  const truncated = name.slice(0, MAX_NAME_LEN);
  return `[UNTRUSTED PLAYER DATA] ${truncated} [/UNTRUSTED PLAYER DATA]`;
}

export function sanitizeChat(message: string): string {
  const truncated = message.slice(0, MAX_CHAT_LEN);
  return `[UNTRUSTED PLAYER DATA] ${truncated} [/UNTRUSTED PLAYER DATA]`;
}

export function sanitizeEvent(event: Record<string, unknown>): Record<string, unknown> {
  const copy = { ...event };
  if (typeof copy.from === "string") {
    copy.from = sanitizeName(copy.from);
  }
  if (typeof copy.player_name === "string") {
    copy.player_name = sanitizeName(copy.player_name);
  }
  if (typeof copy.message === "string" && copy.type === "chat") {
    copy.message = sanitizeChat(copy.message);
  }
  return copy;
}
