import { Trade, TradeRequest, TradeStatus } from '@/types/trade';
import apiClient from '../client';

export interface TradesListResponse {
  trades: Trade[];
  count: number;
}

export const tradeService = {
  // List copilot trades
  getTrades: async (status?: TradeStatus, limit?: number): Promise<TradesListResponse> => {
    return await apiClient.get('/api/v1/copilot/trades', {
      params: { status, limit },
    });
  },

  // Get trade details
  getTrade: async (id: number): Promise<Trade> => {
    return await apiClient.get(`/api/v1/copilot/trades/${id}`);
  },

  // Place a copilot trade
  createTrade: async (data: TradeRequest): Promise<Trade> => {
    return await apiClient.post('/api/v1/copilot/trade', data);
  },

  // Cancel pending trade
  cancelTrade: async (id: number): Promise<void> => {
    await apiClient.delete(`/api/v1/copilot/trades/${id}`);
  },

  // Manual sell (cancels auto-sell, places market order)
  manualSell: async (id: number): Promise<void> => {
    await apiClient.post(`/api/v1/copilot/trades/${id}/sell`);
  },
};
