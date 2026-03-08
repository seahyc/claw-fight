import { readFileSync, writeFileSync, mkdirSync, existsSync } from "fs";
import { join } from "path";
import { homedir } from "os";

export interface Session {
  player_id: string;
  player_name?: string;
  match_id?: string;
  server_url?: string;
}

const BASE_DIR = join(homedir(), ".claw-fight");
const sessionName = process.env.CLAW_FIGHT_SESSION;
const SESSION_DIR = sessionName ? join(BASE_DIR, "sessions", sessionName) : BASE_DIR;
const SESSION_FILE = join(SESSION_DIR, "session.json");

export function loadSession(): Session | null {
  try {
    const data = readFileSync(SESSION_FILE, "utf-8");
    return JSON.parse(data) as Session;
  } catch {
    return null;
  }
}

export function saveSession(session: Session): void {
  if (!existsSync(SESSION_DIR)) {
    mkdirSync(SESSION_DIR, { recursive: true });
  }
  writeFileSync(SESSION_FILE, JSON.stringify(session, null, 2) + "\n");
}

export function requireSession(): Session {
  const session = loadSession();
  if (!session) {
    output({ error: "No session found. Run 'claw-fight register --name YOUR_NAME' first." });
    process.exit(1);
  }
  return session;
}

export function getServerUrl(opts: { server?: string }): string {
  const session = loadSession();
  return opts.server || process.env.CLAW_FIGHT_SERVER || session?.server_url || "http://localhost:7429";
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
