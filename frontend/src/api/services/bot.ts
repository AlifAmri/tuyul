import { BotConfig, BotConfigRequest, BotSummary, Position, Order, UpdateBotRequest } from '@/types/bot';
import apiClient from '../client';

export interface BotsListResponse {
  bots: BotConfig[];
  count: number;
}

export interface PositionsListResponse {
  positions: Position[];
  count: number;
}

export interface OrdersListResponse {
  orders: Order[];
  count: number;
}

export const botService = {
  // List user's bots
  getBots: async (type?: string, status?: string): Promise<BotsListResponse> => {
    const bots = await apiClient.get('/api/v1/bots', {
      params: { type, status },
    });
    // Backend returns array directly, wrap it in expected format
    return { bots: Array.isArray(bots) ? bots : [], count: Array.isArray(bots) ? bots.length : 0 };
  },

  // Get bot details
  getBot: async (id: number): Promise<BotConfig> => {
    return await apiClient.get(`/api/v1/bots/${id}`);
  },

  // Create new bot
  createBot: async (data: BotConfigRequest): Promise<BotConfig> => {
    return await apiClient.post('/api/v1/bots', data);
  },

  // Update bot configuration (bot must be stopped)
  updateBot: async (id: number, data: UpdateBotRequest): Promise<BotConfig> => {
    return await apiClient.put(`/api/v1/bots/${id}`, data);
  },

  // Delete bot (must be stopped first)
  deleteBot: async (id: number): Promise<void> => {
    await apiClient.delete(`/api/v1/bots/${id}`);
  },

  // Start bot
  startBot: async (id: number): Promise<void> => {
    await apiClient.post(`/api/v1/bots/${id}/start`);
  },

  // Stop bot
  stopBot: async (id: number): Promise<void> => {
    await apiClient.post(`/api/v1/bots/${id}/stop`);
  },

  // Get bot P&L summary
  getBotSummary: async (id: number): Promise<BotSummary> => {
    return await apiClient.get(`/api/v1/bots/${id}/summary`);
  },

  // List bot positions (Pump Hunter only)
  getBotPositions: async (id: number, status?: 'open' | 'closed'): Promise<PositionsListResponse> => {
    const response = await apiClient.get(`/api/v1/bots/${id}/positions`, {
      params: { status },
    });
    // Handle both array response and object response
    if (Array.isArray(response)) {
      return { positions: response, count: response.length };
    }
    // If response has positions property, ensure it has count too
    if (response && typeof response === 'object' && 'positions' in response) {
      const positionsData = response as { positions?: Position[]; count?: number };
      return {
        positions: positionsData.positions || [],
        count: positionsData.count ?? (positionsData.positions?.length || 0),
      };
    }
    // Fallback: wrap in expected format
    return { positions: [], count: 0 };
  },

  // List bot orders (Both Market Maker and Pump Hunter)
  getBotOrders: async (id: number): Promise<Order[]> => {
    return await apiClient.get(`/api/v1/bots/${id}/orders`);
  },
};
