import { create } from 'zustand';
import { Coin } from '@/types/market';

interface MarketState {
  // Map of pair_id -> Coin for fast lookups
  coins: Map<string, Coin>;
  
  // Actions
  updateCoin: (coin: Coin) => void;
  updateCoins: (coins: Coin[]) => void; // For batched updates
  getCoin: (pairId: string) => Coin | undefined;
  getAllCoins: () => Coin[];
  clearCoins: () => void;
  
  // Helper to get coins as array sorted by different criteria
  getCoinsSortedByPumpScore: (limit?: number) => Coin[];
  getCoinsSortedByGap: (limit?: number) => Coin[];
  getCoinsSortedByVolume: (limit?: number) => Coin[];
  getCoinsSortedByChange24h: (limit?: number) => Coin[];
}

export const useMarketStore = create<MarketState>((set, get) => ({
  coins: new Map(),
  
  updateCoin: (coin) => {
    set((state) => {
      const newCoins = new Map(state.coins);
      newCoins.set(coin.pair_id, coin);
      return { coins: newCoins };
    });
  },
  
  updateCoins: (coins) => {
    set((state) => {
      const newCoins = new Map(state.coins);
      coins.forEach(coin => {
        newCoins.set(coin.pair_id, coin);
      });
      return { coins: newCoins };
    });
  },
  
  getCoin: (pairId) => {
    return get().coins.get(pairId);
  },
  
  getAllCoins: () => {
    return Array.from(get().coins.values());
  },
  
  clearCoins: () => {
    set({ coins: new Map() });
  },
  
  getCoinsSortedByPumpScore: (limit) => {
    const coins = get().getAllCoins();
    const sorted = coins.sort((a, b) => (b.pump_score || 0) - (a.pump_score || 0));
    return limit ? sorted.slice(0, limit) : sorted;
  },
  
  getCoinsSortedByGap: (limit) => {
    const coins = get().getAllCoins();
    const sorted = coins.sort((a, b) => (b.gap_percentage || 0) - (a.gap_percentage || 0));
    return limit ? sorted.slice(0, limit) : sorted;
  },
  
  getCoinsSortedByVolume: (limit) => {
    const coins = get().getAllCoins();
    const sorted = coins.sort((a, b) => (b.volume_idr || 0) - (a.volume_idr || 0));
    return limit ? sorted.slice(0, limit) : sorted;
  },
  
  getCoinsSortedByChange24h: (limit) => {
    const coins = get().getAllCoins();
    const sorted = coins.sort((a, b) => (b.change_24h || 0) - (a.change_24h || 0));
    return limit ? sorted.slice(0, limit) : sorted;
  },
}));

