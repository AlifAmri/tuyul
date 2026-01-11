import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import { marketService } from '@/api/services/market';
import { Coin } from '@/types/market';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { useWebSocket } from '@/contexts/WebSocketContext';
import { WebSocketMessage, MarketUpdateMessage } from '@/types/websocket';
import { useMarketStore } from '@/stores/marketStore';
import { MarketTabs } from '@/components/market/MarketTabs';
import { MarketSearch } from '@/components/market/MarketSearch';
import { MarketTable } from '@/components/market/MarketTable';
import { PairDetailModal } from '@/components/market/PairDetailModal';

export default function AllMarketsPage() {
  const { lastMessage } = useWebSocket();
  const { coins: storeCoins, updateCoins } = useMarketStore();
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedPair, setSelectedPair] = useState<Coin | null>(null);
  const [priceChanges, setPriceChanges] = useState<Record<string, 'up' | 'down' | null>>({});
  const priceTimeouts = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

  // Fetch only all markets data
  const { data: allMarkets, isLoading } = useQuery({
    queryKey: ['markets', 'summary'],
    queryFn: () => marketService.getSummary(),
  });

  // Initialize store with API data when it loads
  useEffect(() => {
    if (allMarkets?.markets) {
      updateCoins(allMarkets.markets);
    }
  }, [allMarkets, updateCoins]);

  // WebSocket listener for real-time market updates and blink animations
  useEffect(() => {
    if (!lastMessage) return;

    const message = lastMessage as WebSocketMessage;
    
    if (message.type === 'market_update' && message.payload) {
      const marketMessage = message as MarketUpdateMessage;
      const payload = marketMessage.payload;
      
      const coinsToAnimate: Coin[] = Array.isArray(payload) ? payload : [payload];
      
      const updates: Record<string, 'up' | 'down'> = {};
      coinsToAnimate.forEach((coin: Coin) => {
        updates[coin.pair_id] = coin.change_24h > 0 ? 'up' : 'down';
        
        if (priceTimeouts.current[coin.pair_id]) {
          clearTimeout(priceTimeouts.current[coin.pair_id]);
        }
        
        priceTimeouts.current[coin.pair_id] = setTimeout(() => {
          setPriceChanges(prev => {
            const next = { ...prev };
            delete next[coin.pair_id];
            return next;
          });
        }, 500);
      });
      
      setPriceChanges(prev => ({ ...prev, ...updates }));
    }
  }, [lastMessage]);

  // Memoize helper functions
  const calculateChange = useCallback((coin: Coin, timeframe: '1m' | '5m' | '15m' | '30m') => {
    const tfData = coin.timeframes?.[timeframe];
    if (!tfData?.open) return 0;
    return ((coin.current_price - tfData.open) / tfData.open) * 100;
  }, []);


  // Merge and sort data
  const mergeAndSortMarketData = useCallback((apiData: Coin[] | undefined): Coin[] => {
    const baseData = apiData || [];
    const mergedMap = new Map<string, Coin>();
    
    baseData.forEach(coin => {
      mergedMap.set(coin.pair_id, coin);
    });
    
    storeCoins.forEach((wsCoin) => {
      mergedMap.set(wsCoin.pair_id, wsCoin);
    });
    
    const merged = Array.from(mergedMap.values());
    return merged.sort((a, b) => (b.change_24h || 0) - (a.change_24h || 0));
  }, [storeCoins]);

  const currentData = useMemo(() => {
    return mergeAndSortMarketData(allMarkets?.markets);
  }, [allMarkets?.markets, mergeAndSortMarketData]);

  // Filter markets by search query
  const filteredMarkets = useMemo(() => {
    if (!searchQuery.trim()) {
      return currentData;
    }
    const query = searchQuery.toLowerCase();
    return currentData.filter((coin: Coin) =>
      coin.pair_id.toLowerCase().includes(query) ||
      coin.base_currency.toLowerCase().includes(query)
    );
  }, [currentData, searchQuery]);

  const handleSelectPair = useCallback((coin: Coin) => {
    setSelectedPair(coin);
  }, []);

  return (
    <div className="max-w-[1800px] mx-auto">
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Market Analysis</h1>
            <p className="text-gray-500 dark:text-gray-400 mt-1">
              Real-time market data with pump scores and gap analysis
            </p>
          </div>
        </div>

        {/* Tabs & Search */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
              <MarketTabs />
              <MarketSearch value={searchQuery} onChange={setSearchQuery} />
            </div>
          </div>
        </div>

        {/* Market Table */}
        {isLoading ? (
          <div className="flex justify-center py-12">
            <LoadingSpinner />
          </div>
        ) : (
          <MarketTable
            coins={filteredMarkets}
            activeTab="all"
            priceChanges={priceChanges}
            onSelectPair={handleSelectPair}
            calculateChange={calculateChange}
          />
        )}

        {/* Pair Detail Modal */}
        {selectedPair && (
          <PairDetailModal
            pair={selectedPair}
            onClose={() => setSelectedPair(null)}
            calculateChange={calculateChange}
          />
        )}
      </div>
    </div>
  );
}

