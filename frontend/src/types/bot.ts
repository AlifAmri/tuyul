export type BotType = 'market_maker' | 'pump_hunter';
export type BotStatus = 'stopped' | 'starting' | 'running' | 'error';

export interface PumpHunterEntryRules {
  min_pump_score: number;
  min_timeframes_positive: number;
  min_24h_volume_idr: number;
  min_price_idr: number;
  excluded_pairs: string[];
  allowed_pairs: string[];
}

export interface PumpHunterExitRules {
  target_profit_percent: number;
  stop_loss_percent: number;
  trailing_stop_enabled: boolean;
  trailing_stop_percent: number;
  max_hold_minutes: number;
  exit_on_pump_score_drop: boolean;
  pump_score_drop_threshold: number;
}

export interface PumpHunterRiskManagement {
  max_position_idr: number;
  max_concurrent_positions: number;
  daily_loss_limit_idr: number;
  cooldown_after_loss_minutes: number;
  min_balance_idr: number;
}

export interface BotConfig {
  id: number;
  user_id: string;
  name: string;
  type: BotType;
  pair: string;
  is_paper_trading: boolean;
  api_key_id?: number | null;

  // Market Maker parameters
  initial_balance_idr: number;
  order_size_idr: number;
  min_gap_percent: number;
  reposition_threshold_percent: number;
  max_loss_idr: number;

  // Virtual balances
  balances: Record<string, number>;

  // Pump Hunter parameters
  entry_rules?: PumpHunterEntryRules;
  exit_rules?: PumpHunterExitRules;
  risk_management?: PumpHunterRiskManagement;

  // Statistics
  total_trades: number;
  winning_trades: number;
  total_profit_idr: number;
  win_rate?: number; // Calculated field for UI

  // Status
  status: BotStatus;
  error_message?: string;

  created_at: string;
  updated_at: string;
}

export interface BotConfigRequest {
  name: string;
  type: BotType;
  pair: string;
  is_paper_trading: boolean;
  api_key_id?: number | null;
  initial_balance_idr: number;
  order_size_idr: number;
  min_gap_percent: number;
  reposition_threshold_percent: number;
  max_loss_idr: number;
  entry_rules?: PumpHunterEntryRules;
  exit_rules?: PumpHunterExitRules;
  risk_management?: PumpHunterRiskManagement;
}

export interface UpdateBotRequest extends Partial<BotConfigRequest> {}

export interface Position {
  id: number;
  bot_config_id: number;
  user_id: string;
  pair: string;
  status: 'open' | 'closed';
  entry_price: number;
  entry_quantity: number;
  entry_amount_idr: number;
  entry_order_id?: string;
  entry_pump_score?: number;
  entry_at: string;
  exit_price?: number;
  exit_quantity?: number;
  exit_amount_idr?: number;
  exit_order_id?: string;
  exit_at?: string;
  highest_price?: number;
  lowest_price?: number;
  profit_idr: number;
  profit_percent: number;
  close_reason?: string;
  is_paper_trade: boolean;
  created_at: string;
  updated_at: string;
  // Legacy fields for backward compatibility
  bot_id?: number; // Maps to bot_config_id
  entry_amount?: number; // Maps to entry_amount_idr
  exit_amount?: number; // Maps to exit_amount_idr
  closed_at?: string; // Maps to exit_at
}

export interface Order {
  id: number;
  user_id: string;
  parent_id: number;
  parent_type: 'bot' | 'trade' | 'position';
  order_id: string; // Indodax order ID
  pair: string;
  side: 'buy' | 'sell';
  status: 'pending' | 'open' | 'filled' | 'cancelled' | 'failed' | 'stopped';
  price: number;
  amount: number;
  filled_amount: number;
  is_paper_trade: boolean;
  created_at: string;
  updated_at: string;
  filled_at?: string;
}

export interface BotSummary {
  bot_id: number;
  type: string;
  status: string;
  pair?: string;
  total_trades: number;
  winning_trades: number;
  losing_trades: number;
  win_rate: number;
  total_profit_idr: number;
  average_profit: number;
  uptime: string;
  last_trade_at?: string;
  balance_idr?: number;
  balance_coin?: number;
  balance_coin_name?: string;
  open_positions?: number; // Added for convenience info
  profit_factor?: number; // Added for convenience info
}
