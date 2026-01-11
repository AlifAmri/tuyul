import { memo } from 'react';
import { useNavigate } from 'react-router-dom';
import { Coin } from '@/types/market';
import { NoDataEmptyState } from '@/components/common/EmptyState';
import { MarketRow } from './MarketRow';

type TabType = 'all' | 'pumps' | 'gaps';

interface MarketTableProps {
  coins: Coin[];
  activeTab: TabType;
  priceChanges: Record<string, 'up' | 'down' | null>;
  onSelectPair: (coin: Coin) => void;
  calculateChange: (coin: Coin, timeframe: '1m' | '5m' | '15m' | '30m') => number;
}

export const MarketTable = memo(({ 
  coins, 
  activeTab, 
  priceChanges, 
  onSelectPair,
  calculateChange
}: MarketTableProps) => {
  const navigate = useNavigate();

  return (
    <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl overflow-hidden border border-gray-800">
      <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
      <div className="relative z-10">
        <div className="overflow-x-auto custom-scrollbar">
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-800">
                <th className="px-6 py-4 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">
                  Pair
                </th>
                {activeTab === 'pumps' && (
                  <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                    Pump Score
                  </th>
                )}
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
                {activeTab !== 'pumps' && (
                  <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                    Pump Score
                  </th>
                )}
                <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                  Gap (%)
                </th>
                <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {coins.length === 0 ? (
                <tr>
                  <td colSpan={activeTab === 'gaps' ? 9 : activeTab === 'pumps' ? 10 : 10} className="px-6 py-12">
                    <NoDataEmptyState />
                  </td>
                </tr>
              ) : (
                coins.map((coin: Coin) => (
                  <MarketRow
                    key={coin.pair_id}
                    coin={coin}
                    activeTab={activeTab}
                    priceChange={priceChanges[coin.pair_id]}
                    onSelect={onSelectPair}
                    onNavigate={navigate}
                    calculateChange={calculateChange}
                  />
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
});

MarketTable.displayName = 'MarketTable';

