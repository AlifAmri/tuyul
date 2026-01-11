import { useState, useEffect, useRef, useMemo, useCallback, memo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { marketService } from '@/api/services/market';
import { Coin } from '@/types/market';
import { formatIDR, formatPercent } from '@/utils/formatters';
import { cn } from '@/utils/cn';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { NoDataEmptyState } from '@/components/common/EmptyState';
import { useWebSocket } from '@/contexts/WebSocketContext';
import { WebSocketMessage, MarketUpdateMessage } from '@/types/websocket';
import { useMarketStore } from '@/stores/marketStore';

type TabType = 'all' | 'pumps' | 'gaps';

export default function MarketAnalysisPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { lastMessage } = useWebSocket();
  const { coins: storeCoins, updateCoins } = useMarketStore();
  
  // Initialize activeTab from URL params or default to 'all'
  const tabFromUrl = searchParams.get('tab') as TabType | null;
  const [activeTab, setActiveTab] = useState<TabType>(
    tabFromUrl && ['all', 'pumps', 'gaps'].includes(tabFromUrl) ? tabFromUrl : 'all'
  );
  
  // Track if tab change is from user click (to prevent useEffect interference)
  const isUserTabChange = useRef(false);
  
  // Sync activeTab with URL params only when URL changes externally (not from user clicks)
  useEffect(() => {
    if (isUserTabChange.current) {
      isUserTabChange.current = false;
      return; // Ignore this effect if change was from user click
    }
    
    const tabFromUrl = searchParams.get('tab') as TabType | null;
    if (tabFromUrl && ['all', 'pumps', 'gaps'].includes(tabFromUrl) && tabFromUrl !== activeTab) {
      setActiveTab(tabFromUrl);
    }
  }, [searchParams, activeTab]);
  
  // Update URL when tab changes (but not from URL param changes)
  const handleTabChange = useCallback((tab: TabType) => {
    isUserTabChange.current = true; // Mark as user-initiated change
    setActiveTab(tab);
    if (tab !== 'all') {
      setSearchParams({ tab }, { replace: true });
    } else {
      setSearchParams({}, { replace: true });
    }
  }, [setSearchParams]);
  
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedPair, setSelectedPair] = useState<Coin | null>(null);
  const [priceChanges, setPriceChanges] = useState<Record<string, 'up' | 'down' | null>>({});
  const priceTimeouts = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

  // Fetch market data based on active tab (NO refetchInterval - WebSocket only)
  const { data: allMarkets, isLoading: allLoading } = useQuery({
    queryKey: ['markets', 'summary'],
    queryFn: () => marketService.getSummary(),
    enabled: activeTab === 'all',
  });

  const { data: pumpScores, isLoading: pumpsLoading } = useQuery({
    queryKey: ['markets', 'pump-scores'],
    queryFn: () => marketService.getPumpScores(),
    enabled: activeTab === 'pumps',
  });

  const { data: gaps, isLoading: gapsLoading } = useQuery({
    queryKey: ['markets', 'gaps'],
    queryFn: () => marketService.getGaps(),
    enabled: activeTab === 'gaps',
  });

  // Initialize store with API data when it loads
  useEffect(() => {
    if (allMarkets?.markets) {
      updateCoins(allMarkets.markets);
    }
  }, [allMarkets, updateCoins]);

  useEffect(() => {
    if (pumpScores?.scores) {
      updateCoins(pumpScores.scores);
    }
  }, [pumpScores, updateCoins]);

  useEffect(() => {
    if (gaps?.gaps) {
      updateCoins(gaps.gaps);
    }
  }, [gaps, updateCoins]);

  // WebSocket listener for real-time market updates and blink animations
  // Batch state updates to reduce re-renders
  useEffect(() => {
    if (!lastMessage) return;

    const message = lastMessage as WebSocketMessage;
    
    if (message.type === 'market_update' && message.payload) {
      const marketMessage = message as MarketUpdateMessage;
      const payload = marketMessage.payload;
      
      // Handle both single coin and array
      const coinsToAnimate: Coin[] = Array.isArray(payload) ? payload : [payload];
      
      // Batch state updates
      const updates: Record<string, 'up' | 'down'> = {};
      coinsToAnimate.forEach((coin: Coin) => {
        updates[coin.pair_id] = coin.change_24h > 0 ? 'up' : 'down';
        
        // Clear existing timeout
        if (priceTimeouts.current[coin.pair_id]) {
          clearTimeout(priceTimeouts.current[coin.pair_id]);
        }
        
        // Set new timeout
        priceTimeouts.current[coin.pair_id] = setTimeout(() => {
          setPriceChanges(prev => {
            const next = { ...prev };
            delete next[coin.pair_id];
            return next;
          });
        }, 500);
      });
      
      // Single state update for all coins
      setPriceChanges(prev => ({ ...prev, ...updates }));
    }
  }, [lastMessage]);

  // Memoize helper functions
  const calculateChange = useCallback((coin: Coin, timeframe: '1m' | '5m' | '15m' | '30m') => {
    const tfData = coin.timeframes?.[timeframe];
    if (!tfData?.open) return 0;
    return ((coin.current_price - tfData.open) / tfData.open) * 100;
  }, []);

  const getPumpScoreColor = useCallback((score: number) => {
    if (score >= 100) return 'text-red-500';
    if (score >= 50) return 'text-orange-500';
    if (score >= 20) return 'text-yellow-500';
    return 'text-gray-400';
  }, []);

  // Memoize merge and sort function
  const mergeAndSortMarketData = useCallback((apiData: Coin[] | undefined, sortBy: 'change_24h' | 'pump_score' | 'gap_percentage'): Coin[] => {
    // Start with API data or empty array
    const baseData = apiData || [];
    
    // Create a map for fast lookups
    const mergedMap = new Map<string, Coin>();
    
    // Add API data first
    baseData.forEach(coin => {
      mergedMap.set(coin.pair_id, coin);
    });
    
    // Override with WebSocket updates if available
    storeCoins.forEach((wsCoin) => {
      mergedMap.set(wsCoin.pair_id, wsCoin);
    });
    
    // Convert to array and sort
    const merged = Array.from(mergedMap.values());
    
    switch (sortBy) {
      case 'change_24h':
        return merged.sort((a, b) => (b.change_24h || 0) - (a.change_24h || 0));
      case 'pump_score':
        return merged.sort((a, b) => (b.pump_score || 0) - (a.pump_score || 0));
      case 'gap_percentage':
        return merged.sort((a, b) => (b.gap_percentage || 0) - (a.gap_percentage || 0));
      default:
        return merged;
    }
  }, [storeCoins]);

  // Memoize current data based on active tab
  const currentData = useMemo(() => {
    if (activeTab === 'all') {
      return mergeAndSortMarketData(allMarkets?.markets, 'change_24h');
    } else if (activeTab === 'pumps') {
      return mergeAndSortMarketData(pumpScores?.scores, 'pump_score');
    } else {
      return mergeAndSortMarketData(gaps?.gaps, 'gap_percentage');
    }
  }, [activeTab, allMarkets?.markets, pumpScores?.scores, gaps?.gaps, mergeAndSortMarketData]);

  const isLoading = allLoading || pumpsLoading || gapsLoading;

  // Memoize filtered markets with debounced search
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState(searchQuery);
  
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery);
    }, 300); // 300ms debounce
    
    return () => clearTimeout(timer);
  }, [searchQuery]);

  const filteredMarkets = useMemo(() => {
    if (!debouncedSearchQuery.trim()) {
      return currentData;
    }
    const query = debouncedSearchQuery.toLowerCase();
    return currentData.filter((coin: Coin) =>
      coin.pair_id.toLowerCase().includes(query) ||
      coin.base_currency.toLowerCase().includes(query)
    );
  }, [currentData, debouncedSearchQuery]);

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
              {/* Tabs */}
              <div className="flex gap-2">
                <button
                  onClick={() => handleTabChange('all')}
                  className={cn(
                    'px-4 py-2 rounded-lg font-medium transition-all',
                    activeTab === 'all'
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                  )}
                >
                  All Markets
                </button>
                <button
                  onClick={() => handleTabChange('pumps')}
                  className={cn(
                    'px-4 py-2 rounded-lg font-medium transition-all',
                    activeTab === 'pumps'
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                  )}
                >
                  Pump Scores
                </button>
                <button
                  onClick={() => handleTabChange('gaps')}
                  className={cn(
                    'px-4 py-2 rounded-lg font-medium transition-all',
                    activeTab === 'gaps'
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                  )}
                >
                  Gaps
                </button>
              </div>

              {/* Search */}
              <div className="relative">
                <input
                  type="text"
                  placeholder="Search pair..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="w-full md:w-64 px-4 py-2 pl-10 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                />
                <svg
                  className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-500"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
              </div>
            </div>
          </div>
        </div>

        {/* Market Table */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            {isLoading ? (
              <div className="flex justify-center py-12">
                <LoadingSpinner />
              </div>
            ) : (
              <div className="overflow-x-auto custom-scrollbar">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-gray-800">
                      <th className="px-6 py-4 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">
                        Pair
                      </th>
                      <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                        Price
                      </th>
                      {activeTab === 'gaps' ? (
                        <>
                          <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                            Best Bid
                          </th>
                          <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                            Best Ask
                          </th>
                        </>
                      ) : (
                        <>
                          <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                            1m
                          </th>
                          <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                            5m
                          </th>
                          <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                            15m
                          </th>
                        </>
                      )}
                      <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                        24h
                      </th>
                      <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                        Volume (IDR)
                      </th>
                      <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                        Pump Score
                      </th>
                      <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                        Gap (%)
                      </th>
                      <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                        Actions
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredMarkets.length === 0 ? (
                      <tr>
                        <td colSpan={activeTab === 'gaps' ? 9 : 10} className="px-6 py-12">
                          <NoDataEmptyState />
                        </td>
                      </tr>
                    ) : (
                      filteredMarkets.map((coin: Coin) => (
                        <MarketRow
                          key={coin.pair_id}
                          coin={coin}
                          activeTab={activeTab}
                          priceChange={priceChanges[coin.pair_id]}
                          onSelect={handleSelectPair}
                          onNavigate={navigate}
                          calculateChange={calculateChange}
                          getPumpScoreColor={getPumpScoreColor}
                        />
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>

        {/* Pair Detail Modal */}
        {selectedPair && (
          <PairDetailModal
            pair={selectedPair}
            onClose={() => setSelectedPair(null)}
            calculateChange={calculateChange}
            getPumpScoreColor={getPumpScoreColor}
          />
        )}
      </div>
    </div>
  );
}

// Memoized Market Row Component
interface MarketRowProps {
  coin: Coin;
  activeTab: TabType;
  priceChange: 'up' | 'down' | null;
  onSelect: (coin: Coin) => void;
  onNavigate: (path: string) => void;
  calculateChange: (coin: Coin, timeframe: '1m' | '5m' | '15m' | '30m') => number;
  getPumpScoreColor: (score: number) => string;
}

const MarketRow = memo(({ coin, activeTab, priceChange, onSelect, onNavigate, calculateChange, getPumpScoreColor }: MarketRowProps) => {
  const change1m = useMemo(() => calculateChange(coin, '1m'), [coin, calculateChange]);
  const change5m = useMemo(() => calculateChange(coin, '5m'), [coin, calculateChange]);
  const change15m = useMemo(() => calculateChange(coin, '15m'), [coin, calculateChange]);

  const handleRowClick = useCallback(() => {
    onSelect(coin);
  }, [coin, onSelect]);

  const handleActionClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    if (activeTab === 'gaps') {
      onNavigate(`/bots?create=true&type=market_maker&pair=${coin.pair_id}`);
    }
  }, [activeTab, coin.pair_id, onNavigate]);

  return (
    <tr
      onClick={handleRowClick}
      className="border-b border-gray-800/50 hover:bg-gray-900/50 cursor-pointer transition-colors"
    >
      <td className="px-6 py-4">
        <div>
          <p className="font-semibold text-white">
            {coin.base_currency.toUpperCase()}/{coin.quote_currency.toUpperCase()}
          </p>
          <p className="text-xs text-gray-500">{coin.pair_id}</p>
        </div>
      </td>
      <td className="px-6 py-4 text-right">
        <p className={cn(
          "font-semibold text-white transition-all duration-300",
          priceChange === 'up' && "animate-blink-green",
          priceChange === 'down' && "animate-blink-red"
        )}>
          {formatIDR(coin.current_price)}
        </p>
      </td>
      {activeTab === 'gaps' ? (
        <>
          <td className="px-6 py-4 text-right">
            <p className="font-medium text-green-400">
              {formatIDR(coin.best_bid)}
            </p>
          </td>
          <td className="px-6 py-4 text-right">
            <p className="font-medium text-red-400">
              {formatIDR(coin.best_ask)}
            </p>
          </td>
        </>
      ) : (
        <>
          <td className="px-6 py-4 text-right">
            <div
              title={coin.timeframes?.['1m'] ? `O: ${formatIDR(coin.timeframes['1m'].open)} | H: ${formatIDR(coin.timeframes['1m'].high)} | L: ${formatIDR(coin.timeframes['1m'].low)} | Trx: ${coin.timeframes['1m'].trx}` : 'No data'}
              className="cursor-help"
            >
              <span className={cn(
                'font-medium',
                change1m >= 0 ? 'text-green-500' : 'text-red-500'
              )}>
                {formatPercent(change1m)}
              </span>
            </div>
          </td>
          <td className="px-6 py-4 text-right">
            <div
              title={coin.timeframes?.['5m'] ? `O: ${formatIDR(coin.timeframes['5m'].open)} | H: ${formatIDR(coin.timeframes['5m'].high)} | L: ${formatIDR(coin.timeframes['5m'].low)} | Trx: ${coin.timeframes['5m'].trx}` : 'No data'}
              className="cursor-help"
            >
              <span className={cn(
                'font-medium',
                change5m >= 0 ? 'text-green-500' : 'text-red-500'
              )}>
                {formatPercent(change5m)}
              </span>
            </div>
          </td>
          <td className="px-6 py-4 text-right">
            <div
              title={coin.timeframes?.['15m'] ? `O: ${formatIDR(coin.timeframes['15m'].open)} | H: ${formatIDR(coin.timeframes['15m'].high)} | L: ${formatIDR(coin.timeframes['15m'].low)} | Trx: ${coin.timeframes['15m'].trx}` : 'No data'}
              className="cursor-help"
            >
              <span className={cn(
                'font-medium',
                change15m >= 0 ? 'text-green-500' : 'text-red-500'
              )}>
                {formatPercent(change15m)}
              </span>
            </div>
          </td>
        </>
      )}
      <td className="px-6 py-4 text-right">
        <span className={cn(
          'font-medium',
          coin.change_24h >= 0 ? 'text-green-500' : 'text-red-500'
        )}>
          {formatPercent(coin.change_24h)}
        </span>
      </td>
      <td className="px-6 py-4 text-right">
        <p className="text-gray-300">{formatIDR(coin.volume_idr)}</p>
      </td>
      <td className="px-6 py-4 text-right">
        <span className={cn('font-bold', getPumpScoreColor(coin.pump_score))}>
          {coin.pump_score.toFixed(1)}
        </span>
      </td>
      <td className="px-6 py-4 text-right">
        <span className="text-gray-300">{formatPercent(coin.gap_percentage)}</span>
      </td>
      <td className="px-6 py-4 text-right">
        {activeTab === 'gaps' ? (
          <button
            onClick={handleActionClick}
            className="px-3 py-1 bg-primary-600 hover:bg-primary-700 text-white text-sm rounded-lg transition-colors"
          >
            Create Bot
          </button>
        ) : (
          <button
            onClick={(e) => {
              e.stopPropagation();
              // TODO: Navigate to copilot with pair pre-filled
            }}
            className="px-3 py-1 bg-primary-600 hover:bg-primary-700 text-white text-sm rounded-lg transition-colors"
          >
            Trade
          </button>
        )}
      </td>
    </tr>
  );
});

MarketRow.displayName = 'MarketRow';

// Memoized Pair Detail Modal Component
interface PairDetailModalProps {
  pair: Coin;
  onClose: () => void;
  calculateChange: (coin: Coin, timeframe: '1m' | '5m' | '15m' | '30m') => number;
  getPumpScoreColor: (score: number) => string;
}

const PairDetailModal = memo(({ pair, onClose, calculateChange, getPumpScoreColor }: PairDetailModalProps) => {
  return (
    <div
      className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-8 max-w-2xl w-full border border-gray-800"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
        <div className="relative z-10">
          {/* Header */}
          <div className="flex items-center justify-between mb-6">
            <div>
              <h2 className="text-2xl font-bold text-white">
                {pair.base_currency.toUpperCase()}/{pair.quote_currency.toUpperCase()}
              </h2>
              <p className="text-gray-400 text-sm">{pair.pair_id}</p>
            </div>
            <button
              onClick={onClose}
              className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
            >
              <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          {/* Price Info */}
          <div className="grid grid-cols-2 gap-4 mb-6">
            <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
              <p className="text-gray-400 text-sm mb-1">Current Price</p>
              <p className="text-2xl font-bold text-white">{formatIDR(pair.current_price)}</p>
            </div>
            <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
              <p className="text-gray-400 text-sm mb-1">24h Change</p>
              <p className={cn(
                'text-2xl font-bold',
                pair.change_24h >= 0 ? 'text-green-500' : 'text-red-500'
              )}>
                {formatPercent(pair.change_24h)}
              </p>
            </div>
          </div>

          {/* Timeframe Details */}
          <div className="space-y-3 mb-6">
            <h3 className="text-lg font-semibold text-white">Timeframe Analysis</h3>
            {(['1m', '5m', '15m', '30m'] as const).map((tf) => {
              const tfData = pair.timeframes?.[tf];
              const change = calculateChange(pair, tf);
              
              return (
                <div key={tf} className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-gray-400 font-medium">{tf.toUpperCase()}</span>
                    <span className={cn(
                      'font-bold',
                      change >= 0 ? 'text-green-500' : 'text-red-500'
                    )}>
                      {formatPercent(change)}
                    </span>
                  </div>
                  {tfData && (
                    <div className="grid grid-cols-4 gap-2 text-sm">
                      <div>
                        <p className="text-gray-500">Open</p>
                        <p className="text-white font-medium">{formatIDR(tfData.open)}</p>
                      </div>
                      <div>
                        <p className="text-gray-500">High</p>
                        <p className="text-white font-medium">{formatIDR(tfData.high)}</p>
                      </div>
                      <div>
                        <p className="text-gray-500">Low</p>
                        <p className="text-white font-medium">{formatIDR(tfData.low)}</p>
                      </div>
                      <div>
                        <p className="text-gray-500">Trx</p>
                        <p className="text-white font-medium">{tfData.trx}</p>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {/* Additional Stats */}
          <div className="grid grid-cols-3 gap-4">
            <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
              <p className="text-gray-400 text-sm mb-1">Pump Score</p>
              <p className={cn('text-xl font-bold', getPumpScoreColor(pair.pump_score))}>
                {pair.pump_score.toFixed(1)}
              </p>
            </div>
            <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
              <p className="text-gray-400 text-sm mb-1">Gap</p>
              <p className="text-xl font-bold text-white">{formatPercent(pair.gap_percentage)}</p>
            </div>
            <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
              <p className="text-gray-400 text-sm mb-1">Volume (IDR)</p>
              <p className="text-xl font-bold text-white">{formatIDR(pair.volume_idr)}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
});

PairDetailModal.displayName = 'PairDetailModal';
