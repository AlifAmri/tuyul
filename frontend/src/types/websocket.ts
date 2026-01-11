import { BotConfig, BotSummary, Position } from './bot';
import { Coin } from './market';
import { Trade } from './trade';

export type WebSocketMessageType =
  | 'market_update'
  | 'pump_signal'
  | 'order_update'
  | 'bot_status'
  | 'bot_update'
  | 'stop_loss_triggered'
  | 'position_update'
  | 'bot_pnl_update'
  | 'position_open'
  | 'position_close'
  | 'bot_started'
  | 'bot_stopped'
  | 'bot_updated'
  | 'bot_stats'
  | 'mm_orders'
  | 'order_placed'
  | 'order_filled'
  | 'order_cancelled'
  | 'order_rejected'
  | 'subscribed'
  | 'error';

export interface WebSocketMessage {
  type: WebSocketMessageType;
  payload?: unknown;
  bot_id?: number;
  user_id?: string;
  data?: unknown;
}

export interface MarketUpdateMessage extends WebSocketMessage {
  type: 'market_update';
  payload: Coin | Coin[]; // Handles both single coin (backward compat) and array (batched)
}

export interface PumpSignalMessage extends WebSocketMessage {
  type: 'pump_signal';
  payload: {
    pair: string;
    score: number;
    message: string;
  };
}

export interface OrderUpdateMessage extends WebSocketMessage {
  type: 'order_update';
  payload: {
    order_id: string;
    status: string;
    filled_qty: number;
  };
}

export interface BotStatusMessage extends WebSocketMessage {
  type: 'bot_status';
  payload: {
    bot_id: number;
    status: 'starting' | 'running' | 'stopped' | 'error';
  };
}

export interface StopLossTriggerMessage extends WebSocketMessage {
  type: 'stop_loss_triggered';
  payload: Trade;
}

export interface PositionUpdateMessage extends WebSocketMessage {
  type: 'position_update';
  payload: Position;
}

export interface BotPnLUpdateMessage extends WebSocketMessage {
  type: 'bot_pnl_update';
  payload: {
    bot_id: number;
    profit: number;
  };
}

export interface PositionOpenMessage extends WebSocketMessage {
  type: 'position_open';
  bot_id: number;
  data: Position;
}

export interface PositionCloseMessage extends WebSocketMessage {
  type: 'position_close';
  bot_id: number;
  data: {
    position: Position;
    summary: BotSummary;
  };
}

export interface BotStatsMessage extends WebSocketMessage {
  type: 'bot_stats';
  bot_id: number;
  data: {
    bot: BotConfig;
    summary: BotSummary;
  };
}

export interface BotUpdateMessage extends WebSocketMessage {
  type: 'bot_update';
  payload: {
    bot_id: number;
    status?: 'starting' | 'running' | 'stopped' | 'error';
    buy_price?: number; // Current bid price
    sell_price?: number; // Current ask price
    spread_percent?: number; // Current spread percentage
    balances?: Record<string, number>;
    total_trades?: number;
    winning_trades?: number;
    total_profit_idr?: number;
    win_rate?: number;
  };
}
