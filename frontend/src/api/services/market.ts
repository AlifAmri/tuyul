import { Coin, GapsResponse, MarketSummaryResponse, PumpScoresResponse } from '@/types/market';
import apiClient from '../client';

export const marketService = {
  // Get all market summaries sorted by pump score
  getSummary: async (limit?: number, min_volume?: number): Promise<MarketSummaryResponse> => {
    return await apiClient.get('/api/v1/market/summary', {
      params: { limit, min_volume },
    });
  },

  // Get specific pair detail
  getPair: async (pair: string): Promise<Coin> => {
    return await apiClient.get(`/api/v1/market/${pair}`);
  },

  // Get top pump scores
  getPumpScores: async (limit?: number, min_volume?: number, min_pump_score?: number): Promise<PumpScoresResponse> => {
    return await apiClient.get('/api/v1/market/pump-scores', {
      params: { limit, min_volume, min_pump_score },
    });
  },

  // Get top gaps
  getGaps: async (limit?: number, min_volume?: number): Promise<GapsResponse> => {
    return await apiClient.get('/api/v1/market/gaps', {
      params: { limit, min_volume },
    });
  },

  // Sync metadata (Admin only)
  syncMetadata: async (): Promise<void> => {
    await apiClient.post('/api/v1/market/sync');
  },
};
