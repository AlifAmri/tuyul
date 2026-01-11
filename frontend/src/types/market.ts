export interface Coin {
  pair_id: string;
  base_currency: string;
  quote_currency: string;
  current_price: number;
  high_24h: number;
  low_24h: number;
  open_24h: number;
  volume_24h: number;
  volume_idr: number;
  change_24h: number;
  best_bid: number;
  best_ask: number;
  bid_volume: number;
  ask_volume: number;
  gap_percentage: number;
  spread: number;
  pump_score: number;
  volatility_1m: number;
  timeframes: Timeframes;
  last_update: string;
}

export interface TimeframeData {
  open: number;
  high: number;
  low: number;
  trx: number;
}

export interface Timeframes {
  '1m': TimeframeData;
  '5m': TimeframeData;
  '15m': TimeframeData;
  '30m': TimeframeData;
}

// Response from /api/v1/market/summary
export interface MarketSummaryResponse {
  markets: Coin[];
  count: number;
  last_update: string;
}

// Response from /api/v1/market/pump-scores
export interface PumpScoresResponse {
  scores: Coin[];
  count: number;
}

// Response from /api/v1/market/gaps
export interface GapsResponse {
  gaps: Coin[];
  count: number;
}

export type SortKey = 'pump_score' | 'gap_percentage' | 'volume_idr' | 'change_24h' | 'current_price';
export type SortOrder = 'asc' | 'desc';

export interface TimeframeRecord {
  pair_id: string;
  timeframe: '1m' | '5m' | '15m' | '30m';
  data: TimeframeData;
}
