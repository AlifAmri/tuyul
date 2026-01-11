import { memo } from 'react';
import { Coin } from '@/types/market';
import { formatIDR, formatPercent } from '@/utils/formatters';
import { cn } from '@/utils/cn';

interface PairDetailModalProps {
  pair: Coin;
  onClose: () => void;
  calculateChange: (coin: Coin, timeframe: '1m' | '5m' | '15m' | '30m') => number;
}

export const PairDetailModal = memo(({ pair, onClose, calculateChange }: PairDetailModalProps) => {
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
            <div className="flex items-center gap-2">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  window.open(`https://indodax.com/trade/${pair.pair_id.toUpperCase()}`, '_blank', 'noopener,noreferrer');
                }}
                className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors text-sm"
              >
                View Chart
              </button>
              <button
                onClick={onClose}
                className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
              >
                <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
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
              <p className="text-xl font-bold text-white">
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

