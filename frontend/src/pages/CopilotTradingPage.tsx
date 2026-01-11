import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { tradeService } from '@/api/services/trade';
import { apiKeyService } from '@/api/services/apiKey';
import { useAuthStore } from '@/stores/authStore';
import { Trade, TradeRequest } from '@/types/trade';
import { formatIDR, formatPercent } from '@/utils/formatters';
import { cn } from '@/utils/cn';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { useWebSocket } from '@/contexts/WebSocketContext';
import { WebSocketMessage } from '@/types/websocket';
import { AxiosError } from 'axios';

type TradeMode = 'paper' | 'live';
type TradeStatus = 'pending' | 'filled' | 'completed' | 'cancelled' | 'stopped';

export default function CopilotTradingPage() {
  const queryClient = useQueryClient();
  const { lastMessage } = useWebSocket();
  const [mode, setMode] = useState<TradeMode>('paper');
  const [showNewTradeForm, setShowNewTradeForm] = useState(false);
  const [selectedTrade, setSelectedTrade] = useState<Trade | null>(null);
  const [createError, setCreateError] = useState<string>('');
  const [activeTab, setActiveTab] = useState<'active' | 'history'>('active');

  // Form state
  const [formData, setFormData] = useState<TradeRequest>({
    pair: '',
    buying_price: 0,
    volume_idr: 0,
    target_profit: 0,
    stop_loss: 0,
    is_paper_trade: true,
  });

  // Get API key from user store
  const { user } = useAuthStore();
  const apiKey = user?.api_key;

  // Fetch trades (NO refetchInterval - WebSocket only)
  const { data: trades, isLoading: tradesLoading } = useQuery({
    queryKey: ['copilot', 'trades'],
    queryFn: () => tradeService.getTrades(),
  });

  // Fetch account info for live mode
  const { data: accountInfo } = useQuery({
    queryKey: ['api-keys', 'account-info'],
    queryFn: () => apiKeyService.getAccountInfo(),
    enabled: mode === 'live' && !!apiKey,
  });

  // WebSocket listener for real-time trade updates
  useEffect(() => {
    if (!lastMessage) return;

    const message = lastMessage as WebSocketMessage;
    
    if (message.type === 'order_update' || message.type === 'stop_loss_triggered') {
      // Invalidate trades query to refetch
      queryClient.invalidateQueries({ queryKey: ['copilot', 'trades'] });
    }
  }, [lastMessage, queryClient]);

  // Create trade mutation
  const createTradeMutation = useMutation({
    mutationFn: (data: TradeRequest) => tradeService.createTrade(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['copilot', 'trades'] });
      setShowNewTradeForm(false);
      setCreateError('');
      resetForm();
    },
    onError: (error: AxiosError<any>) => {
      const errMsg = error.response?.data?.error?.message || error.message || 'Failed to create trade';
      setCreateError(errMsg);
    },
  });

  // Cancel trade mutation
  const cancelTradeMutation = useMutation({
    mutationFn: (id: number) => tradeService.cancelTrade(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['copilot', 'trades'] });
    },
  });

  // Manual sell mutation
  const manualSellMutation = useMutation({
    mutationFn: (id: number) => tradeService.manualSell(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['copilot', 'trades'] });
    },
  });

  const resetForm = () => {
    setFormData({
      pair: '',
      buying_price: 0,
      volume_idr: 0,
      target_profit: 0,
      stop_loss: 0,
      is_paper_trade: true,
    });
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError('');
    createTradeMutation.mutate({ ...formData, is_paper_trade: mode === 'paper' });
  };

  const handleModeChange = (newMode: TradeMode) => {
    setMode(newMode);
  };

  // Filter trades by status
  const activeTrades = trades?.trades?.filter((t: Trade) => t.status === 'pending' || t.status === 'filled') || [];
  const historyTrades = trades?.trades?.filter((t: Trade) => t.status === 'completed' || t.status === 'cancelled' || t.status === 'stopped') || [];

  // Get status color
  const getStatusColor = (status: TradeStatus) => {
    switch (status) {
      case 'pending': return 'text-yellow-500 bg-yellow-500/10';
      case 'filled': return 'text-blue-500 bg-blue-500/10';
      case 'completed': return 'text-green-500 bg-green-500/10';
      case 'cancelled': return 'text-gray-500 bg-gray-500/10';
      case 'stopped': return 'text-red-500 bg-red-500/10';
      default: return 'text-gray-500 bg-gray-500/10';
    }
  };

  return (
    <div className="max-w-[1800px] mx-auto">
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-gray-900 dark:text-white">Candle Keeper</h1>
            <p className="text-gray-500 dark:text-gray-400 mt-1">
              I'll keep the candle lit while you're out stealing.
            </p>
          </div>
          <button
            onClick={() => setShowNewTradeForm(true)}
            className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors"
          >
            + New Target
          </button>
        </div>

        {/* Mode Toggle & Balance */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-6 overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10 flex items-center justify-between">
            {/* Mode Toggle */}
            <div className="flex items-center gap-4">
              <span className="text-gray-400 font-medium">Trading Mode:</span>
              <div className="flex gap-2">
                <button
                  onClick={() => handleModeChange('paper')}
                  className={cn(
                    'px-4 py-2 rounded-lg font-medium transition-all',
                    mode === 'paper'
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                  )}
                >
                  Paper Trading
                </button>
                <button
                  onClick={() => handleModeChange('live')}
                  disabled={!apiKey}
                  title={!apiKey ? 'Configure API key first' : ''}
                  className={cn(
                    'px-4 py-2 rounded-lg font-medium transition-all',
                    !apiKey && 'opacity-50 cursor-not-allowed',
                    mode === 'live'
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700',
                    !apiKey && 'hover:bg-gray-800 hover:text-gray-400'
                  )}
                >
                  Live Trading
                </button>
              </div>
            </div>

            {/* Balance (Live mode only) */}
            {mode === 'live' && accountInfo && (
              <div className="flex items-center gap-6">
                <div>
                  <p className="text-gray-400 text-sm">IDR Balance</p>
                  <p className="text-white font-bold text-lg">{formatIDR(parseFloat(accountInfo.balance.idr || '0'))}</p>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Trades Tab Section */}
        <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl overflow-hidden border border-gray-800">
          <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
          <div className="relative z-10">
            {/* Tab Navigation */}
            <div className="px-6 py-4 border-b border-gray-800">
              <div className="flex items-center gap-4">
                <button
                  onClick={() => setActiveTab('active')}
                  className={cn(
                    'px-4 py-2 rounded-lg font-medium transition-all',
                    activeTab === 'active'
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                  )}
                >
                  Active Trades
                </button>
                <button
                  onClick={() => setActiveTab('history')}
                  className={cn(
                    'px-4 py-2 rounded-lg font-medium transition-all',
                    activeTab === 'history'
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                  )}
                >
                  Trade History
                </button>
              </div>
            </div>

            {/* Tab Content */}
            <div className="p-6">
              {tradesLoading ? (
                <div className="flex justify-center py-8">
                  <LoadingSpinner />
                </div>
              ) : activeTab === 'active' ? (
                // Active Trades Tab
                activeTrades.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-12">
                    <img 
                      src="/tuyul-crying.png" 
                      alt="Crying Tuyul" 
                      className="w-40 h-auto mb-4"
                    />
                    <h3 className="text-xl font-bold text-white mb-2">No Active Targets</h3>
                    <p className="text-gray-400 text-center max-w-md">
                      I'm ready to keep the candle lit!<br />
                      Create a new target and I'll watch it 24/7.
                    </p>
                  </div>
                ) : (
                  <div className="space-y-3">
                    {activeTrades.map((trade: Trade) => (
                      <div
                        key={trade.id}
                        className="bg-gray-900/50 rounded-lg p-4 border border-gray-800 hover:bg-gray-800/50 transition-colors cursor-pointer"
                        onClick={() => setSelectedTrade(trade)}
                      >
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-4">
                            <div>
                              <p className="font-bold text-white text-lg">{trade.pair.toUpperCase()}</p>
                              <p className="text-sm text-gray-400">
                                {trade.is_paper_trade ? '📄 Paper' : '💰 Live'}
                              </p>
                            </div>
                            <div className={cn('px-3 py-1 rounded-full text-sm font-medium', getStatusColor(trade.status))}>
                              {trade.status.toUpperCase()}
                            </div>
                          </div>
                          <div className="flex items-center gap-6">
                            <div className="text-right">
                              <p className="text-gray-400 text-sm">Buy Price</p>
                              <p className="text-white font-semibold">{formatIDR(trade.buy_price)}</p>
                            </div>
                            <div className="text-right">
                              <p className="text-gray-400 text-sm">Amount</p>
                              <p className="text-white font-semibold">{trade.buy_amount.toFixed(8)}</p>
                            </div>
                            <div className="text-right">
                              <p className="text-gray-400 text-sm">Target Profit</p>
                              <p className="text-green-500 font-semibold">{formatPercent(trade.target_profit)}</p>
                            </div>
                            <div className="text-right">
                              <p className="text-gray-400 text-sm">Stop Loss</p>
                              <p className="text-red-500 font-semibold">{formatPercent(trade.stop_loss)}</p>
                            </div>
                            <div className="flex gap-2">
                              {trade.status === 'pending' && (
                                <button
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    cancelTradeMutation.mutate(trade.id);
                                  }}
                                  className="px-3 py-1 bg-gray-700 hover:bg-gray-600 text-white text-sm rounded-lg transition-colors"
                                >
                                  Cancel
                                </button>
                              )}
                              {trade.status === 'filled' && (
                                <button
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    manualSellMutation.mutate(trade.id);
                                  }}
                                  className="px-3 py-1 bg-primary-600 hover:bg-primary-700 text-white text-sm rounded-lg transition-colors"
                                >
                                  Sell Now
                                </button>
                              )}
                            </div>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )
              ) : (
                // Trade History Tab
                historyTrades.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-12">
                    <img 
                      src="/tuyul-crying.png" 
                      alt="Crying Tuyul" 
                      className="w-40 h-auto mb-4"
                    />
                    <h3 className="text-xl font-bold text-white mb-2">No History Yet</h3>
                    <p className="text-gray-400 text-center max-w-md">
                      Once I complete targets for you, they'll show up here with all the loot details!
                    </p>
                  </div>
                ) : (
                  <div className="overflow-x-auto custom-scrollbar">
                    <table className="w-full">
                      <thead>
                        <tr className="border-b border-gray-800">
                          <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Pair</th>
                          <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Status</th>
                          <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Buy Price</th>
                          <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Sell Price</th>
                          <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Volume</th>
                          <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">P&L</th>
                          <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Mode</th>
                        </tr>
                      </thead>
                      <tbody>
                        {historyTrades.map((trade: Trade) => {
                          const pnl = trade.sell_price 
                            ? (trade.sell_price - trade.buy_price) * trade.buy_amount 
                            : 0;
                          
                          return (
                            <tr
                              key={trade.id}
                              onClick={() => setSelectedTrade(trade)}
                              className="border-b border-gray-800/50 hover:bg-gray-900/50 cursor-pointer transition-colors"
                            >
                              <td className="px-4 py-3 text-white font-medium">{trade.pair.toUpperCase()}</td>
                              <td className="px-4 py-3">
                                <span className={cn('px-2 py-1 rounded-full text-xs font-medium', getStatusColor(trade.status))}>
                                  {trade.status.toUpperCase()}
                                </span>
                              </td>
                              <td className="px-4 py-3 text-right text-white">{formatIDR(trade.buy_price)}</td>
                              <td className="px-4 py-3 text-right text-white">
                                {trade.sell_price ? formatIDR(trade.sell_price) : '-'}
                              </td>
                              <td className="px-4 py-3 text-right text-white">{trade.buy_amount.toFixed(8)}</td>
                              <td className="px-4 py-3 text-right">
                                <span className={cn('font-semibold', pnl >= 0 ? 'text-green-500' : 'text-red-500')}>
                                  {pnl !== 0 ? formatIDR(pnl) : '-'}
                                </span>
                              </td>
                              <td className="px-4 py-3 text-right text-gray-400 text-sm">
                                {trade.is_paper_trade ? 'Paper' : 'Live'}
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )
              )}
            </div>
          </div>
        </div>

        {/* New Trade Form Modal */}
        {showNewTradeForm && (
          <div
            className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4"
            onClick={() => setShowNewTradeForm(false)}
          >
            <div
              className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-8 max-w-md w-full border border-gray-800"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
              <div className="relative z-10">
                <div className="flex items-center justify-between mb-6">
                  <h2 className="text-2xl font-bold text-white">New Target</h2>
                  <button
                    onClick={() => {
                      setShowNewTradeForm(false);
                      setCreateError('');
                    }}
                    className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
                  >
                    <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>

                {createError && (
                  <div className="mb-4 p-4 bg-red-500/10 border border-red-500/50 rounded-lg flex items-start justify-between">
                    <div className="flex items-start gap-3">
                      <svg className="w-5 h-5 text-red-500 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                      <p className="text-red-400 text-sm">{createError}</p>
                    </div>
                    <button
                      onClick={() => setCreateError('')}
                      className="text-red-400 hover:text-red-300"
                    >
                      <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  </div>
                )}

                <form onSubmit={handleSubmit} className="space-y-4">
                  <div>
                    <label className="block text-gray-400 text-sm font-medium mb-2">Pair</label>
                    <input
                      type="text"
                      placeholder="e.g., btcidr"
                      value={formData.pair}
                      onChange={(e) => setFormData({ ...formData, pair: e.target.value })}
                      className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                      required
                    />
                  </div>

                  <div>
                    <label className="block text-gray-400 text-sm font-medium mb-2">Buying Price (IDR)</label>
                    <input
                      type="number"
                      placeholder="0"
                      value={formData.buying_price || ''}
                      onChange={(e) => setFormData({ ...formData, buying_price: parseFloat(e.target.value) })}
                      className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                      required
                    />
                  </div>

                  <div>
                    <label className="block text-gray-400 text-sm font-medium mb-2">Volume (IDR)</label>
                    <input
                      type="number"
                      placeholder="0"
                      value={formData.volume_idr || ''}
                      onChange={(e) => setFormData({ ...formData, volume_idr: parseFloat(e.target.value) })}
                      className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                      required
                    />
                  </div>

                  <div>
                    <label className="block text-gray-400 text-sm font-medium mb-2">Target Profit (%)</label>
                    <input
                      type="number"
                      step="0.1"
                      placeholder="0"
                      value={formData.target_profit || ''}
                      onChange={(e) => setFormData({ ...formData, target_profit: parseFloat(e.target.value) })}
                      className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                      required
                    />
                  </div>

                  <div>
                    <label className="block text-gray-400 text-sm font-medium mb-2">Stop Loss (%)</label>
                    <input
                      type="number"
                      step="0.1"
                      placeholder="0"
                      value={formData.stop_loss || ''}
                      onChange={(e) => setFormData({ ...formData, stop_loss: parseFloat(e.target.value) })}
                      className="w-full px-4 py-2 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
                      required
                    />
                  </div>

                  <button
                    type="submit"
                    disabled={createTradeMutation.isPending}
                    className="w-full px-4 py-3 bg-primary-600 hover:bg-primary-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
                  >
                    {createTradeMutation.isPending ? 'Creating...' : 'Create Trade'}
                  </button>
                </form>
              </div>
            </div>
          </div>
        )}

        {/* Trade Detail Modal */}
        {selectedTrade && (
          <div
            className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4"
            onClick={() => setSelectedTrade(null)}
          >
            <div
              className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-8 max-w-lg w-full border border-gray-800"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
              <div className="relative z-10">
                <div className="flex items-center justify-between mb-6">
                  <div>
                    <h2 className="text-2xl font-bold text-white">{selectedTrade.pair.toUpperCase()}</h2>
                    <span className={cn('inline-block px-3 py-1 rounded-full text-sm font-medium mt-2', getStatusColor(selectedTrade.status))}>
                      {selectedTrade.status.toUpperCase()}
                    </span>
                  </div>
                  <button
                    onClick={() => setSelectedTrade(null)}
                    className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
                  >
                    <svg className="w-6 h-6 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>

                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                      <p className="text-gray-400 text-sm mb-1">Buy Price</p>
                      <p className="text-white font-bold text-lg">{formatIDR(selectedTrade.buy_price)}</p>
                    </div>
                    <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                      <p className="text-gray-400 text-sm mb-1">Sell Price</p>
                      <p className="text-white font-bold text-lg">
                        {selectedTrade.sell_price ? formatIDR(selectedTrade.sell_price) : '-'}
                      </p>
                    </div>
                  </div>

                  <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                    <p className="text-gray-400 text-sm mb-1">Buy Amount</p>
                    <p className="text-white font-bold text-lg">{selectedTrade.buy_amount.toFixed(8)}</p>
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                      <p className="text-gray-400 text-sm mb-1">Target Profit</p>
                      <p className="text-green-500 font-bold text-lg">{formatPercent(selectedTrade.target_profit)}</p>
                    </div>
                    <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                      <p className="text-gray-400 text-sm mb-1">Stop Loss</p>
                      <p className="text-red-500 font-bold text-lg">{formatPercent(selectedTrade.stop_loss)}</p>
                    </div>
                  </div>

                  {selectedTrade.sell_price && (
                    <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                      <p className="text-gray-400 text-sm mb-1">P&L</p>
                      <p className={cn(
                        'font-bold text-2xl',
                        (selectedTrade.sell_price - selectedTrade.buy_price) * selectedTrade.buy_amount >= 0
                          ? 'text-green-500'
                          : 'text-red-500'
                      )}>
                        {formatIDR((selectedTrade.sell_price - selectedTrade.buy_price) * selectedTrade.buy_amount)}
                      </p>
                    </div>
                  )}

                  <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                    <p className="text-gray-400 text-sm mb-1">Mode</p>
                    <p className="text-white font-medium">
                      {selectedTrade.is_paper_trade ? '📄 Paper Trading' : '💰 Live Trading'}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

