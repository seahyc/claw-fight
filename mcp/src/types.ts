export interface Action {
  type: string;
  data: Record<string, any>;
}

export interface ActionResult {
  success: boolean;
  message: string;
  data?: Record<string, any>;
}

export interface PlayerView {
  phase: string;
  your_turn: boolean;
  simultaneous: boolean;
  board: any;
  available_actions: string[];
  last_action?: ActionResult;
  turn_number: number;
  game_specific?: Record<string, any>;
}

export interface GameResult {
  finished: boolean;
  winner?: string;
  draw: boolean;
  scores?: Record<string, number>;
  reason: string;
}

export interface GameInfo {
  name: string;
  description: string;
  min_players: number;
  max_players: number;
}

export interface MatchInfo {
  match_id: string;
  code: string;
  spectator_url: string;
  game_type: string;
  status: string;
}

export interface WSMessage {
  type: string;
  [key: string]: any;
}
