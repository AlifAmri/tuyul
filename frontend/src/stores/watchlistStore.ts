import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface WatchlistState {
  pairs: string[];
  addPair: (pair: string) => void;
  removePair: (pair: string) => void;
  togglePair: (pair: string) => void;
  isPairWatched: (pair: string) => boolean;
  clearAll: () => void;
}

export const useWatchlistStore = create<WatchlistState>()(
  persist(
    (set, get) => ({
      pairs: [],

      addPair: (pair) => {
        set((state) => {
          if (state.pairs.includes(pair)) return state;
          return { pairs: [...state.pairs, pair] };
        });
      },

      removePair: (pair) => {
        set((state) => ({
          pairs: state.pairs.filter((p) => p !== pair),
        }));
      },

      togglePair: (pair) => {
        const { pairs } = get();
        if (pairs.includes(pair)) {
          get().removePair(pair);
        } else {
          get().addPair(pair);
        }
      },

      isPairWatched: (pair) => {
        return get().pairs.includes(pair);
      },

      clearAll: () => {
        set({ pairs: [] });
      },
    }),
    {
      name: 'tuyul-watchlist',
    }
  )
);

