import { memo, useMemo, useCallback } from 'react';
import { Coin } from '@/types/market';
import { formatIDR, formatPercent } from '@/utils/formatters';
import { cn } from '@/utils/cn';
import { PumpScoreGauge } from './PumpScoreGauge';

type TabType = 'all' | 'pumps' | 'gaps';

interface MarketRowProps {
  coin: Coin;
  activeTab: TabType;
  priceChange: 'up' | 'down' | null;
  onSelect: (coin: Coin) => void;
  onNavigate: (path: string) => void;
  calculateChange: (coin: Coin, timeframe: '1m' | '5m' | '15m' | '30m') => number;
  getPumpScoreColor: (score: number) => string;
}

export const MarketRow = memo(({ coin, activeTab, priceChange, onSelect, onNavigate, calculateChange, getPumpScoreColor }: MarketRowProps) => {
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
      {activeTab === 'pumps' && (
        <td className="px-6 py-4">
          <div className="flex justify-end">
            <PumpScoreGauge 
              score={coin.pump_score} 
              className="max-w-[120px]"
              showValue={false}
            />
          </div>
        </td>
      )}
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
      {activeTab !== 'pumps' && (
        <td className="px-6 py-4">
          <div className="flex justify-end">
            <PumpScoreGauge 
              score={coin.pump_score} 
              className="max-w-[120px]"
              showValue={false}
            />
          </div>
        </td>
      )}
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

