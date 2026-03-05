import WebSocket from "ws";
import type { WSMessage } from "./types.js";

function generatePlayerName(): string {
  const suffix = Math.floor(Math.random() * 10000)
    .toString()
    .padStart(4, "0");
  return `Agent-${suffix}`;
}

interface PendingWaiter {
  type: string;
  resolve: (msg: WSMessage) => void;
  reject: (err: Error) => void;
  timer?: ReturnType<typeof setTimeout>;
}

export class GameClient {
  private ws: WebSocket | null = null;
  private serverUrl: string;
  private playerName: string;
  private playerId: string | null = null;
  private handlers: Array<(msg: WSMessage) => void> = [];
  private waiters: PendingWaiter[] = [];
  private messageQueue: WSMessage[] = [];
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private closed = false;

  constructor() {
    this.serverUrl =
      process.env.CLAW_FIGHT_SERVER || "ws://localhost:8080/ws";
    this.playerName = generatePlayerName();
  }

  async connect(): Promise<void> {
    if (this.ws?.readyState === WebSocket.OPEN) return;

    return new Promise((resolve, reject) => {
      this.ws = new WebSocket(this.serverUrl);

      this.ws.on("open", () => {
        this.reconnectAttempts = 0;
        resolve();
      });

      this.ws.on("message", (data) => {
        const msg = JSON.parse(data.toString()) as WSMessage;
        this.handleMessage(msg);
      });

      this.ws.on("close", () => {
        if (!this.closed) {
          this.tryReconnect();
        }
      });

      this.ws.on("error", (err) => {
        if (this.ws?.readyState !== WebSocket.OPEN) {
          reject(err);
        }
      });
    });
  }

  private handleMessage(msg: WSMessage): void {
    // Check waiters first
    const waiterIndex = this.waiters.findIndex((w) => w.type === msg.type);
    if (waiterIndex >= 0) {
      const waiter = this.waiters[waiterIndex];
      this.waiters.splice(waiterIndex, 1);
      if (waiter.timer) clearTimeout(waiter.timer);
      waiter.resolve(msg);
      return;
    }

    // Notify handlers
    for (const handler of this.handlers) {
      handler(msg);
    }

    // Queue for later consumption
    this.messageQueue.push(msg);
  }

  private async tryReconnect(): Promise<void> {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) return;

    this.reconnectAttempts++;
    const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 30000);
    await new Promise((r) => setTimeout(r, delay));

    try {
      await this.connect();
      if (this.playerId) {
        await this.register(this.playerName);
      }
    } catch {
      // reconnect failed, will retry on next attempt
    }
  }

  async register(playerName?: string): Promise<string> {
    if (playerName) this.playerName = playerName;

    this.send({ type: "register", player_name: this.playerName });
    const response = await this.waitForMessage("registered", 10000);
    this.playerId = response.player_id;
    return this.playerId!;
  }

  send(message: WSMessage): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error("WebSocket is not connected");
    }
    this.ws.send(JSON.stringify(message));
  }

  waitForMessage(type: string, timeout = 60000): Promise<WSMessage> {
    // Check queue first
    const queueIndex = this.messageQueue.findIndex((m) => m.type === type);
    if (queueIndex >= 0) {
      const msg = this.messageQueue[queueIndex];
      this.messageQueue.splice(queueIndex, 1);
      return Promise.resolve(msg);
    }

    return new Promise((resolve, reject) => {
      const waiter: PendingWaiter = { type, resolve, reject };

      if (timeout > 0) {
        waiter.timer = setTimeout(() => {
          const idx = this.waiters.indexOf(waiter);
          if (idx >= 0) this.waiters.splice(idx, 1);
          reject(new Error(`Timeout waiting for message type: ${type}`));
        }, timeout);
      }

      this.waiters.push(waiter);
    });
  }

  async waitForTurn(
    matchId: string,
    progressCallback: (msg: string) => void
  ): Promise<WSMessage> {
    // Check queue for existing your_turn or game_over messages
    for (let i = 0; i < this.messageQueue.length; i++) {
      const msg = this.messageQueue[i];
      if (
        (msg.type === "your_turn" || msg.type === "game_over") &&
        msg.match_id === matchId
      ) {
        this.messageQueue.splice(i, 1);
        return msg;
      }
    }

    return new Promise((resolve, reject) => {
      const interval = setInterval(() => {
        progressCallback("Waiting for opponent...");
      }, 10000);

      const handler = (msg: WSMessage) => {
        if (
          (msg.type === "your_turn" || msg.type === "game_over") &&
          msg.match_id === matchId
        ) {
          clearInterval(interval);
          this.removeHandler(handler);
          resolve(msg);
        }
      };

      this.handlers.push(handler);

      // Also check queued messages that might arrive during setup
      for (let i = 0; i < this.messageQueue.length; i++) {
        const msg = this.messageQueue[i];
        if (
          (msg.type === "your_turn" || msg.type === "game_over") &&
          msg.match_id === matchId
        ) {
          this.messageQueue.splice(i, 1);
          clearInterval(interval);
          this.removeHandler(handler);
          resolve(msg);
          return;
        }
      }
    });
  }

  onMessage(handler: (msg: WSMessage) => void): void {
    this.handlers.push(handler);
  }

  private removeHandler(handler: (msg: WSMessage) => void): void {
    const idx = this.handlers.indexOf(handler);
    if (idx >= 0) this.handlers.splice(idx, 1);
  }

  getPlayerId(): string | null {
    return this.playerId;
  }

  getPlayerName(): string {
    return this.playerName;
  }

  close(): void {
    this.closed = true;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    // Reject all pending waiters
    for (const waiter of this.waiters) {
      if (waiter.timer) clearTimeout(waiter.timer);
      waiter.reject(new Error("Client closed"));
    }
    this.waiters = [];
  }
}
