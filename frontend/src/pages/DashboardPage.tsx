import { useQuery } from '@tanstack/react-query';
import { marketService } from '@/api/services/market';
import { authService } from '@/api/services/auth';
import { apiKeyService } from '@/api/services/apiKey';
import { useAuthStore } from '@/stores/authStore';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { formatIDR, formatPercent } from '@/utils/formatters';
import { Link } from 'react-router-dom';

export function DashboardPage() {
  // Fetch gaps (limit 5)
  const { data: gaps, isLoading: gapsLoading } = useQuery({
    queryKey: ['market-gaps', 5],
    queryFn: () => marketService.getGaps(5),
    retry: 1,
  });

  // Fetch hot targets with min_pump_score 500
  const { data: hotTargets, isLoading: hotTargetsLoading } = useQuery({
    queryKey: ['market-pump-scores', 'hot-targets', 500],
    queryFn: () => marketService.getPumpScores(undefined, undefined, 500),
    retry: 1,
  });

  // Fetch user stats (aggregated from backend)
  const { data: stats, isLoading: statsLoading, error: statsError } = useQuery({
    queryKey: ['user-stats'],
    queryFn: () => authService.getStats(),
    refetchInterval: 60000, // Refresh every 1 minute
    retry: 1,
  });

  // Get API key from user store
  const { user } = useAuthStore();
  const apiKey = user?.api_key;

  // Fetch account balance (only if user has API key, no refetch)
  const { data: accountInfo } = useQuery({
    queryKey: ['api-keys', 'account-info'],
    queryFn: () => apiKeyService.getAccountInfo(),
    enabled: !!apiKey,
    refetchOnWindowFocus: false,
    refetchOnMount: false,
    refetchOnReconnect: false,
  });

  // Show error state if backend is unavailable
  if (statsError) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="text-center">
          <div className="w-16 h-16 bg-red-100 dark:bg-red-900/30 rounded-full flex items-center justify-center mx-auto mb-4">
            <svg className="w-8 h-8 text-red-600 dark:text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <h2 className="text-xl font-bold text-gray-900 dark:text-white mb-2">Unable to Connect</h2>
          <p className="text-gray-600 dark:text-gray-400 mb-4">
            Cannot connect to the backend server. Please ensure the server is running.
          </p>
          <button
            onClick={() => window.location.reload()}
            className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Dashboard</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">
          🧚 Master, I've been working hard! Here's what I've collected for you today...
        </p>
      </div>

      {/* API Key Warning Banner */}
      {!apiKey && (
        <div data-api-key-banner className="relative bg-gradient-to-br from-yellow-950 via-black to-yellow-900/50 rounded-2xl p-6 overflow-hidden border border-yellow-800">
          <div className="absolute inset-0 bg-gradient-to-br from-yellow-500/10 via-transparent to-transparent rounded-2xl pointer-events-none" />
            <div className="relative z-10 flex items-start gap-4">
              <div className="flex items-center justify-center flex-shrink-0">
                <img src="/tuyul-crying.png" alt="Crying Tuyul" className="w-24 h-auto" />
              </div>
            <div className="flex-1">
              <h3 className="text-lg font-bold text-white mb-2">I Need Your Key!</h3>
              <p className="text-gray-300 mb-4">
                I need your Indodax API key to steal real profits for you!
                <br />
                Right now, I can only practice with fake money (paper trading). Give me the key and I'll start collecting real coins! 💰
              </p>
              <Link
                to="/settings/api-keys"
                className="inline-flex items-center gap-2 px-4 py-2 bg-yellow-600 hover:bg-yellow-700 text-white rounded-lg font-medium transition-colors"
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                </svg>
                Give Me the Key! 🔑
              </Link>
            </div>
            <button
              onClick={() => {
                const banner = document.querySelector('[data-api-key-banner]');
                if (banner) banner.remove();
              }}
              className="p-2 hover:bg-yellow-800/50 rounded-lg transition-colors"
              aria-label="Dismiss"
            >
              <svg className="w-5 h-5 text-gray-400 hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>
      )}

      {/* Stats Cards */}
      <div className={`grid grid-cols-1 md:grid-cols-2 ${apiKey ? 'lg:grid-cols-5' : 'lg:grid-cols-4'} gap-6`}>
        {/* Active Bots */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600 dark:text-gray-400">🤖 My Helpers</p>
                <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                  {statsLoading ? '...' : stats?.active_bots || 0}
                </p>
              </div>
              <div className="w-12 h-12 bg-primary-100 dark:bg-primary-900/30 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-primary-600 dark:text-primary-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                </svg>
              </div>
            </div>
          </div>
        </div>

        {/* Total Profit */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-green-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600 dark:text-gray-400">💰 Stolen Profits</p>
                <p className={`text-2xl font-bold mt-1 ${(stats?.total_profit_idr || 0) >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                  {statsLoading ? '...' : formatIDR(stats?.total_profit_idr || 0)}
                </p>
              </div>
              <div className="w-12 h-12 bg-green-100 dark:bg-green-900/30 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6" />
                </svg>
              </div>
            </div>
          </div>
        </div>

        {/* Avg Win Rate */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-blue-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600 dark:text-gray-400">🎯 Success Rate</p>
                <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                  {statsLoading ? '...' : formatPercent(stats?.avg_win_rate || 0, 1)}
                </p>
              </div>
              <div className="w-12 h-12 bg-blue-100 dark:bg-blue-900/30 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-blue-600 dark:text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                </svg>
              </div>
            </div>
          </div>
        </div>

        {/* Total Trades */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-purple-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600 dark:text-gray-400">⚡ Missions Done</p>
                <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                  {statsLoading ? '...' : stats?.total_trades || 0}
                </p>
              </div>
              <div className="w-12 h-12 bg-purple-100 dark:bg-purple-900/30 rounded-lg flex items-center justify-center">
                <svg className="w-6 h-6 text-purple-600 dark:text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
                </svg>
              </div>
            </div>
          </div>
        </div>

        {/* IDR Balance - Only show if user has API key */}
        {apiKey && (
          <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
            <div className="absolute inset-0 bg-gradient-to-br from-yellow-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
            <div className="relative z-10">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-600 dark:text-gray-400">💵 My Balance</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white mt-1">
                    {accountInfo?.balance?.idr ? formatIDR(parseFloat(accountInfo.balance.idr)) : '...'}
                  </p>
                </div>
                <div className="w-12 h-12 bg-yellow-100 dark:bg-yellow-900/30 rounded-lg flex items-center justify-center">
                  <svg className="w-6 h-6 text-yellow-600 dark:text-yellow-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Gaps and Hot Targets - 2 Column Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Gaps Column */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-blue-500/5 rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-800 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white">💰 Best Gaps</h2>
              <Link to="/market/gaps" className="text-sm text-primary-600 dark:text-primary-400 hover:underline">
                View all →
              </Link>
            </div>
            <div className="p-6">
              {gapsLoading ? (
                <div className="flex justify-center py-8">
                  <LoadingSpinner />
                </div>
              ) : gaps?.gaps && gaps.gaps.length > 0 ? (
                <div className="space-y-3">
                  {gaps.gaps.map((coin) => (
                    <div
                      key={coin.pair_id}
                      className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-900/50 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors border border-transparent dark:border-gray-800"
                    >
                      <div className="flex items-center gap-3">
                        <div>
                          <p className="font-medium text-gray-900 dark:text-white">{coin.base_currency.toUpperCase()}/{coin.quote_currency.toUpperCase()}</p>
                          <p className="text-sm text-gray-500 dark:text-gray-400">{coin.pair_id}</p>
                        </div>
                      </div>
                      <div className="flex items-center gap-6">
                        <div className="text-right">
                          <p className="text-sm text-gray-500 dark:text-gray-400">Gap</p>
                          <p className="font-semibold text-gray-900 dark:text-white">{formatPercent(coin.gap_percentage)}</p>
                        </div>
                        <div className="text-right">
                          <p className="text-sm text-gray-500 dark:text-gray-400">Price</p>
                          <p className="font-semibold text-gray-900 dark:text-white">{formatIDR(coin.current_price)}</p>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex justify-center py-8 text-gray-500 dark:text-gray-400">
                  No gaps found
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Hot Targets Column */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-blue-500/5 rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-800 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white">🔥 Hot Targets</h2>
              <Link to="/market/pumps" className="text-sm text-primary-600 dark:text-primary-400 hover:underline">
                View all →
              </Link>
            </div>
            <div className="p-6">
              {hotTargetsLoading ? (
                <div className="flex justify-center py-8">
                  <LoadingSpinner />
                </div>
              ) : hotTargets?.scores && hotTargets.scores.length > 0 ? (
                <div className="space-y-3">
                  {hotTargets.scores.map((coin) => (
                    <div
                      key={coin.pair_id}
                      className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-900/50 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors border border-transparent dark:border-gray-800"
                    >
                      <div className="flex items-center gap-3">
                        <div>
                          <p className="font-medium text-gray-900 dark:text-white">{coin.base_currency.toUpperCase()}/{coin.quote_currency.toUpperCase()}</p>
                          <p className="text-sm text-gray-500 dark:text-gray-400">{coin.pair_id}</p>
                        </div>
                      </div>
                      <div className="flex items-center gap-6">
                        {/* Timeframes */}
                        <div className="flex items-center gap-3">
                          {(() => {
                            const change1m = coin.timeframes?.['1m']?.open 
                              ? ((coin.current_price - coin.timeframes['1m'].open) / coin.timeframes['1m'].open) * 100 
                              : 0;
                            const change5m = coin.timeframes?.['5m']?.open 
                              ? ((coin.current_price - coin.timeframes['5m'].open) / coin.timeframes['5m'].open) * 100 
                              : 0;
                            const change15m = coin.timeframes?.['15m']?.open 
                              ? ((coin.current_price - coin.timeframes['15m'].open) / coin.timeframes['15m'].open) * 100 
                              : 0;
                            
                            return (
                              <>
                                <span className={`text-sm font-medium ${change1m >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                                  {formatPercent(change1m)}
                                </span>
                                <span className={`text-sm font-medium ${change5m >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                                  {formatPercent(change5m)}
                                </span>
                                <span className={`text-sm font-medium ${change15m >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                                  {formatPercent(change15m)}
                                </span>
                                <span className={`text-sm font-medium ${coin.change_24h >= 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                                  {formatPercent(coin.change_24h)}
                                </span>
                              </>
                            );
                          })()}
                        </div>
                        {/* Price */}
                        <div className="text-right">
                          <p className="font-semibold text-gray-900 dark:text-white">{formatIDR(coin.current_price)}</p>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex justify-center py-8 text-gray-500 dark:text-gray-400">
                  No hot targets found
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Quick Actions */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <Link
          to="/bots"
          className="bg-gradient-to-br from-primary-600 to-primary-700 rounded-lg p-6 text-white hover:from-primary-700 hover:to-primary-800 transition-all shadow-lg hover:shadow-xl"
        >
          <h3 className="text-lg font-semibold mb-2">Create New Bot</h3>
          <p className="text-primary-100 text-sm">Set up a new trading bot in minutes</p>
        </Link>

        <Link
          to="/copilot"
          className="bg-gradient-to-br from-blue-600 to-blue-700 rounded-lg p-6 text-white hover:from-blue-700 hover:to-blue-800 transition-all shadow-lg hover:shadow-xl"
        >
          <h3 className="text-lg font-semibold mb-2">New Copilot Trade</h3>
          <p className="text-blue-100 text-sm">Execute a manual trade with automation</p>
        </Link>

        <Link
          to="/market"
          className="bg-gradient-to-br from-purple-600 to-purple-700 rounded-lg p-6 text-white hover:from-purple-700 hover:to-purple-800 transition-all shadow-lg hover:shadow-xl"
        >
          <h3 className="text-lg font-semibold mb-2">Market Analysis</h3>
          <p className="text-purple-100 text-sm">View real-time market data and signals</p>
        </Link>
      </div>
    </div>
  );
}
