export type TradeStatus = 'pending' | 'filled' | 'completed' | 'cancelled' | 'stopped';

export interface Trade {
  id: number;
  user_id: string;
  pair: string;
  status: TradeStatus;
  buy_price: number;
  buy_amount: number;
  buy_order_id: string;
  sell_price: number;
  sell_amount: number;
  sell_order_id: string;
  target_profit: number;
  stop_loss: number;
  is_paper_trade: boolean;
  stop_loss_triggered: boolean;
  created_at: string;
  updated_at: string;
}

export interface TradeRequest {
  pair: string;
  buying_price: number;
  volume_idr: number;
  target_profit: number;
  stop_loss: number;
  is_paper_trade: boolean;
}

export interface ManualSellRequest {
  price?: number; // If not provided, use market price
}
