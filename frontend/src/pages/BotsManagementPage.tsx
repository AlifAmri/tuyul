import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { botService } from '@/api/services/bot';
import { useAuthStore } from '@/stores/authStore';
import { useMarketStore } from '@/stores/marketStore';
import { BotConfig, BotType, BotConfigRequest, Order, Position, BotSummary } from '@/types/bot';
import { formatIDR, formatNumber, formatPercent } from '@/utils/formatters';
import { cn } from '@/utils/cn';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { NoBotsEmptyState } from '@/components/common/EmptyState';
import { AlertModal } from '@/components/common/AlertModal';
import { useWebSocket } from '@/contexts/WebSocketContext';
import { WebSocketMessage, PositionUpdateMessage, PositionOpenMessage, PositionCloseMessage } from '@/types/websocket';
import { AxiosError } from 'axios';

export default function BotsManagementPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { lastMessage } = useWebSocket();
  const { getCoin, coins: marketCoins } = useMarketStore();
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedBot, setSelectedBot] = useState<BotConfig | null>(null);
  const [showCreateWizard, setShowCreateWizard] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [botType, setBotType] = useState<BotType>('pump_hunter');
  const [createError, setCreateError] = useState<string>('');
  const [isPairPreFilled, setIsPairPreFilled] = useState(false);
  const [isBotTypeDisabled, setIsBotTypeDisabled] = useState(false);
  const [openedFromGaps, setOpenedFromGaps] = useState(false);
  
  // Alert modal state
  const [alertModal, setAlertModal] = useState<{
    open: boolean;
    title: string;
    message: string;
    type: 'success' | 'error' | 'info' | 'warning';
  }>({
    open: false,
    title: '',
    message: '',
    type: 'info',
  });
  
  // Spread information state (for Market Maker bots)
  const [spreadInfo, setSpreadInfo] = useState<{
    buyPrice?: number;
    sellPrice?: number;
    spreadPercent?: number;
  } | null>(null);
  
  // Collapsible sections state for Pump Hunter (accordion - only one open at a time)
  const [expandedSection, setExpandedSection] = useState<'entry' | 'exit' | 'risk'>('entry');

  const toggleSection = (section: 'entry' | 'exit' | 'risk') => {
    // If clicking the already-open section, keep it open (accordion must have one open)
    // If clicking a different section, open that one
    setExpandedSection(section);
  };

  // Form state for bot creation
  const [formData, setFormData] = useState<BotConfigRequest>({
    name: '',
    type: 'pump_hunter',
    pair: '',
    is_paper_trading: true,
    initial_balance_idr: 1000000,
    // Market Maker fields
    order_size_idr: 100000,
    min_gap_percent: 0.5,
    reposition_threshold_percent: 0.1,
    max_loss_idr: 500000,
    // Pump Hunter fields
    entry_rules: {
      min_pump_score: 50.0,
      min_timeframes_positive: 2,
      min_24h_volume_idr: 1000000000,
      min_price_idr: 100,
      excluded_pairs: ['usdtidr', 'usdcidr', 'adaidr', 'daidr', 'busdidr', 'tusdidr'], // Exclude stablecoins by default
      allowed_pairs: [],
    },
    exit_rules: {
      target_profit_percent: 3.0,
      stop_loss_percent: 1.5,
      trailing_stop_enabled: true,
      trailing_stop_percent: 1.0,
      max_hold_minutes: 30,
      exit_on_pump_score_drop: true,
      pump_score_drop_threshold: 20.0,
    },
    risk_management: {
      max_position_idr: 500000,
      max_concurrent_positions: 3,
      daily_loss_limit_idr: 1000000,
      cooldown_after_loss_minutes: 10,
      min_balance_idr: 100000,
    },
  });

  // Form state for bot editing
  const [editFormData, setEditFormData] = useState<BotConfigRequest>({
    name: '',
    type: 'market_maker',
    pair: '',
    is_paper_trading: true,
    initial_balance_idr: 1000000,
    order_size_idr: 100000,
    min_gap_percent: 0.5,
    reposition_threshold_percent: 0.1,
    max_loss_idr: 500000,
    entry_rules: {
      min_pump_score: 20,
      min_timeframes_positive: 2,
      min_24h_volume_idr: 1000000,
      min_price_idr: 100,
      excluded_pairs: [],
      allowed_pairs: [],
    },
    exit_rules: {
      target_profit_percent: 5,
      stop_loss_percent: 3,
      trailing_stop_enabled: false,
      trailing_stop_percent: 2,
      max_hold_minutes: 60,
      exit_on_pump_score_drop: true,
      pump_score_drop_threshold: 10,
    },
    risk_management: {
      max_position_idr: 500000,
      max_concurrent_positions: 3,
      daily_loss_limit_idr: 1000000,
      cooldown_after_loss_minutes: 10,
      min_balance_idr: 100000,
    },
  });

  // Fetch bots
  const { data: bots, isLoading: botsLoading } = useQuery({
    queryKey: ['bots'],
    queryFn: () => botService.getBots(),
  });

  // Get API key from user store
  const { user } = useAuthStore();
  const apiKey = user?.api_key;

  // Create bot mutation
  const createBotMutation = useMutation({
    mutationFn: (data: BotConfigRequest) => botService.createBot(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bots'] });
      setShowCreateWizard(false);
      setCreateError('');
      resetForm();
    },
    onError: (error: AxiosError<any>) => {
      const errorMessage = error.response?.data?.error?.message || error.message || 'Failed to create bot';
      setCreateError(errorMessage);
    },
  });

  // Start bot mutation
  const startBotMutation = useMutation({
    mutationFn: (id: number) => botService.startBot(id),
    onSuccess: () => {
      // Invalidate queries - useEffect will sync selectedBot
      queryClient.invalidateQueries({ queryKey: ['bots'] });
    },
    onError: (error: AxiosError<any>) => {
      const errorMessage = error.response?.data?.error?.message || error.message || 'Failed to start bot';
      console.error('[StartBot] Error:', errorMessage, error);
      setAlertModal({
        open: true,
        title: 'Failed to Start Bot',
        message: errorMessage,
        type: 'error',
      });
    },
  });

  // Stop bot mutation
  const stopBotMutation = useMutation({
    mutationFn: (id: number) => botService.stopBot(id),
    onSuccess: () => {
      // Invalidate queries - useEffect will sync selectedBot
      queryClient.invalidateQueries({ queryKey: ['bots'] });
    },
    onError: (error: AxiosError<any>) => {
      const errorMessage = error.response?.data?.error?.message || error.message || 'Failed to stop bot';
      console.error('[StopBot] Error:', errorMessage, error);
      setAlertModal({
        open: true,
        title: 'Failed to Stop Bot',
        message: errorMessage,
        type: 'error',
      });
    },
  });

  // Delete bot mutation
  const deleteBotMutation = useMutation({
    mutationFn: (id: number) => botService.deleteBot(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bots'] });
      setSelectedBot(null);
    },
  });

  // Update bot mutation
  const updateBotMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<BotConfigRequest> }) => 
      botService.updateBot(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['bots'] });
      setShowEditModal(false);
    },
  });

  // Fetch bot positions (for selected bot - Pump Hunter only)
  const { data: positions, isLoading: positionsLoading } = useQuery({
    queryKey: ['bot-positions', selectedBot?.id],
    queryFn: () => selectedBot ? botService.getBotPositions(selectedBot.id) : Promise.resolve(null),
    enabled: !!selectedBot && selectedBot.type === 'pump_hunter',
  });

  // Fetch bot orders (for selected bot - Both types)
  // No refetchInterval - updates come via WebSocket order_update messages
  const { data: orders, isLoading: ordersLoading } = useQuery({
    queryKey: ['bot-orders', selectedBot?.id],
    queryFn: () => selectedBot ? botService.getBotOrders(selectedBot.id) : Promise.resolve(null),
    enabled: !!selectedBot,
    refetchOnWindowFocus: false,
    refetchOnMount: false,
    staleTime: Infinity, // Data is fresh until WebSocket updates it
  });

  // Sync selectedBot when bots list updates (e.g., after start/stop, bot_update)
  // This ensures the summary cards (balances, trades, profit) update in real-time
  useEffect(() => {
    if (selectedBot && bots?.bots) {
      const updatedBot = bots.bots.find(b => b.id === selectedBot.id);
      if (updatedBot) {
        // Always update selectedBot to reflect any changes (status, balances, trades, profit, etc.)
        // React will handle re-render optimization
        setSelectedBot(updatedBot);
        
        // Clear spread info if bot stopped or if it's not a Market Maker
        if (updatedBot.status === 'stopped' || updatedBot.status === 'error' || updatedBot.type !== 'market_maker') {
          setSpreadInfo(null);
        }
      }
    }
  }, [bots, selectedBot]);
  
  // Clear spread info when switching bots
  useEffect(() => {
    setSpreadInfo(null);
  }, [selectedBot?.id]);

  // Handle URL params for pre-filling bot creation form
  useEffect(() => {
    const create = searchParams.get('create');
    const type = searchParams.get('type') as BotType | null;
    const pair = searchParams.get('pair');

    if (create === 'true') {
      // Open the create modal
      setShowCreateWizard(true);
      
      // Set bot type if provided
      if (type && (type === 'market_maker' || type === 'pump_hunter')) {
        setBotType(type);
        setFormData(prev => ({ ...prev, type }));
      }
      
      // Pre-fill pair if provided (for Market Maker)
      if (pair && type === 'market_maker') {
        setFormData(prev => ({ ...prev, pair: pair.toLowerCase() }));
        setIsPairPreFilled(true);
        setIsBotTypeDisabled(true); // Disable bot type selection when coming from gaps
        setOpenedFromGaps(true); // Track that modal was opened from gaps
      }
      
      // Clear URL params after reading them
      setSearchParams({}, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  const resetForm = () => {
    setIsPairPreFilled(false);
    setIsBotTypeDisabled(false);
    setOpenedFromGaps(false);
    setFormData({
      name: '',
      type: 'pump_hunter',
      pair: '',
      is_paper_trading: true,
      initial_balance_idr: 1000000,
      // Market Maker fields
      order_size_idr: 100000,
      min_gap_percent: 0.5,
      reposition_threshold_percent: 0.1,
      max_loss_idr: 500000,
      // Pump Hunter fields
      entry_rules: {
        min_pump_score: 50.0,
        min_timeframes_positive: 2,
        min_24h_volume_idr: 1000000000,
        min_price_idr: 100,
        excluded_pairs: ['usdtidr', 'usdcidr', 'adaidr', 'daidr', 'busdidr', 'tusdidr'],
        allowed_pairs: [],
      },
      exit_rules: {
        target_profit_percent: 3.0,
        stop_loss_percent: 1.5,
        trailing_stop_enabled: true,
        trailing_stop_percent: 1.0,
        max_hold_minutes: 30,
        exit_on_pump_score_drop: true,
        pump_score_drop_threshold: 20.0,
      },
      risk_management: {
        max_position_idr: 500000,
        max_concurrent_positions: 3,
        daily_loss_limit_idr: 1000000,
        cooldown_after_loss_minutes: 10,
        min_balance_idr: 100000,
      },
    });
    setBotType('pump_hunter');
  };

  const handleCloseModal = () => {
    setShowCreateWizard(false);
    setIsPairPreFilled(false);
    setIsBotTypeDisabled(false);
    if (openedFromGaps) {
      // Navigate back to market analysis gaps tab
      navigate('/market/gaps');
      setOpenedFromGaps(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    
    // Build submit data based on bot type
    if (formData.type === 'pump_hunter') {
      // Pump Hunter: send entry_rules, exit_rules, risk_management, and initial_balance_idr
      const submitData: BotConfigRequest = {
        name: formData.name,
        type: 'pump_hunter',
        pair: '', // Pump Hunter scans all pairs
        is_paper_trading: formData.is_paper_trading,
        api_key_id: formData.api_key_id,
        initial_balance_idr: formData.initial_balance_idr,
        entry_rules: formData.entry_rules,
        exit_rules: formData.exit_rules,
        risk_management: formData.risk_management,
      };
      createBotMutation.mutate(submitData);
    } else {
      // Market Maker: send Market Maker specific fields
      const submitData: BotConfigRequest = {
        name: formData.name,
        type: 'market_maker',
        pair: formData.pair,
        is_paper_trading: formData.is_paper_trading,
        api_key_id: formData.api_key_id,
        initial_balance_idr: formData.initial_balance_idr,
        order_size_idr: formData.order_size_idr,
        min_gap_percent: formData.min_gap_percent,
        reposition_threshold_percent: formData.reposition_threshold_percent,
        max_loss_idr: formData.max_loss_idr,
      };
      createBotMutation.mutate(submitData);
    }
  };

  const handleEditSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (selectedBot) {
      // Validate Pump Hunter risk management fields
      if (editFormData.type === 'pump_hunter' && editFormData.risk_management) {
        const risk = editFormData.risk_management;
        if (!risk.max_position_idr || risk.max_position_idr <= 0) {
          setAlertModal({
            open: true,
            title: 'Validation Error',
            message: 'Max Position Size (IDR) must be greater than 0',
            type: 'error',
          });
          return;
        }
        if (!risk.daily_loss_limit_idr || risk.daily_loss_limit_idr <= 0) {
          setAlertModal({
            open: true,
            title: 'Validation Error',
            message: 'Daily Loss Limit (IDR) must be greater than 0',
            type: 'error',
          });
          return;
        }
        if (!risk.min_balance_idr || risk.min_balance_idr <= 0) {
          setAlertModal({
            open: true,
            title: 'Validation Error',
            message: 'Min Balance (IDR) must be greater than 0',
            type: 'error',
          });
          return;
        }
      }
      
      // For Pump Hunter, set pair to empty string since it scans all pairs
      const submitData = {
        ...editFormData,
        pair: editFormData.type === 'pump_hunter' ? '' : editFormData.pair,
      };
      updateBotMutation.mutate({
        id: selectedBot.id,
        data: submitData,
      });
    }
  };

  const openEditModal = (bot: BotConfig) => {
    setBotType(bot.type);
    // Reset expanded section - Entry Rules for Pump Hunter, Trading Parameters (entry) for Market Maker
    setExpandedSection('entry');
    setEditFormData({
      name: bot.name,
      type: bot.type,
      pair: bot.pair || '',
      is_paper_trading: bot.is_paper_trading,
      initial_balance_idr: bot.initial_balance_idr,
      order_size_idr: bot.order_size_idr,
      min_gap_percent: bot.min_gap_percent,
      reposition_threshold_percent: bot.reposition_threshold_percent,
      max_loss_idr: bot.max_loss_idr,
      // Pump Hunter fields
      entry_rules: bot.entry_rules || {
        min_pump_score: 20,
        min_timeframes_positive: 2,
        min_24h_volume_idr: 1000000,
        min_price_idr: 100,
        excluded_pairs: [],
        allowed_pairs: [],
      },
      exit_rules: bot.exit_rules || {
        target_profit_percent: 5,
        stop_loss_percent: 3,
        trailing_stop_enabled: false,
        trailing_stop_percent: 2,
        max_hold_minutes: 60,
        exit_on_pump_score_drop: true,
        pump_score_drop_threshold: 10,
      },
      risk_management: bot.risk_management || {
        max_position_idr: 500000,
        max_concurrent_positions: 3,
        daily_loss_limit_idr: 1000000,
        cooldown_after_loss_minutes: 10,
        min_balance_idr: 100000,
      },
    });
    setShowEditModal(true);
  };

  // Get bot type badge
  const getBotTypeBadge = (type: BotType) => {
    switch (type) {
      case 'market_maker': return 'Market Maker';
      case 'pump_hunter': return 'Pump Hunter';
      default: return type;
    }
  };

  // Handle WebSocket messages for real-time bot updates
  useEffect(() => {
    if (!lastMessage) return;

    const message = lastMessage as WebSocketMessage;

    switch (message.type) {
      case 'bot_status':
      case 'bot_started':
      case 'bot_stopped':
      case 'bot_updated':
      case 'bot_stats':
        // Invalidate bots query to refetch latest data
        queryClient.invalidateQueries({ queryKey: ['bots'] });
        break;

      case 'bot_update': {
        // Update cache directly instead of invalidating (no HTTP refetch)
        const botUpdatePayload = message.payload as { 
          bot_id?: number;
          status?: 'starting' | 'running' | 'stopped' | 'error';
          buy_price?: number;
          sell_price?: number;
          spread_percent?: number;
          balances?: Record<string, number>;
          total_trades?: number;
          winning_trades?: number;
          total_profit_idr?: number;
          win_rate?: number;
        };
        
        if (botUpdatePayload?.bot_id) {
          // Update bots cache directly (no HTTP refetch)
          queryClient.setQueryData(['bots'], (oldData: { bots: BotConfig[]; count: number } | undefined) => {
            if (!oldData?.bots) return oldData;
            
            return {
              ...oldData,
              bots: oldData.bots.map((bot) => {
                if (bot.id === botUpdatePayload.bot_id) {
                  return {
                    ...bot,
                    ...(botUpdatePayload.status && { status: botUpdatePayload.status }),
                    ...(botUpdatePayload.balances && { balances: { ...bot.balances, ...botUpdatePayload.balances } }),
                    ...(botUpdatePayload.total_trades !== undefined && { total_trades: botUpdatePayload.total_trades }),
                    ...(botUpdatePayload.winning_trades !== undefined && { winning_trades: botUpdatePayload.winning_trades }),
                    ...(botUpdatePayload.total_profit_idr !== undefined && { total_profit_idr: botUpdatePayload.total_profit_idr }),
                    ...(botUpdatePayload.win_rate !== undefined && { win_rate: botUpdatePayload.win_rate }),
                  };
                }
                return bot;
              }),
            };
          });
          
          // Store spread information if available (for Market Maker bots)
          // Only update if this update is for the currently selected bot
          if (selectedBot && 
              botUpdatePayload.bot_id === selectedBot.id &&
              (botUpdatePayload.buy_price !== undefined || 
               botUpdatePayload.sell_price !== undefined || 
               botUpdatePayload.spread_percent !== undefined)) {
            setSpreadInfo({
              buyPrice: botUpdatePayload.buy_price,
              sellPrice: botUpdatePayload.sell_price,
              spreadPercent: botUpdatePayload.spread_percent,
            });
          }
          
          // Note: Orders are updated via order_update messages, positions via position_update messages
          // No need to invalidate here - they handle their own updates
        }
        break;
      }

      case 'position_update': {
        // Update positions cache directly using position ID (no HTTP refetch)
        const positionPayload = message.payload as Position;
        const botId = message.bot_id || positionPayload?.bot_config_id || positionPayload?.bot_id;
        
        if (botId && positionPayload && positionPayload.id) {
          queryClient.setQueryData(['bot-positions', botId], (oldData: { positions: Position[] } | Position[] | null | undefined) => {
            // Handle both response format { positions: Position[] } and array format Position[]
            const positionsArray = Array.isArray(oldData) ? oldData : oldData?.positions || [];
            
            // Check if position already exists (match by id)
            const existingIndex = positionsArray.findIndex((p) => p.id === positionPayload.id);
            
            if (existingIndex >= 0) {
              // Update existing position - merge to preserve any fields not in the update
              const updated = [...positionsArray];
              updated[existingIndex] = {
                ...updated[existingIndex],
                ...positionPayload,
              };
              
              // Return in the same format as received
              return Array.isArray(oldData) ? updated : { positions: updated };
            } else {
              // Create new position - add to the list
              const updated = [...positionsArray, positionPayload];
              
              // Return in the same format as received
              return Array.isArray(oldData) ? updated : { positions: updated };
            }
          });
        }
        break;
      }
      
      case 'position_open': {
        // Handle position_open message (same as position_update but with different structure)
        const positionPayload = (message as PositionOpenMessage).data as Position;
        const botId = message.bot_id || positionPayload?.bot_config_id || positionPayload?.bot_id;
        
        if (botId && positionPayload && positionPayload.id) {
          queryClient.setQueryData(['bot-positions', botId], (oldData: { positions: Position[] } | Position[] | null | undefined) => {
            const positionsArray = Array.isArray(oldData) ? oldData : oldData?.positions || [];
            
            // Check if position already exists
            const existingIndex = positionsArray.findIndex((p) => p.id === positionPayload.id);
            
            if (existingIndex >= 0) {
              // Update existing position
              const updated = [...positionsArray];
              updated[existingIndex] = {
                ...updated[existingIndex],
                ...positionPayload,
              };
              return Array.isArray(oldData) ? updated : { positions: updated };
            } else {
              // Create new position
              const updated = [...positionsArray, positionPayload];
              return Array.isArray(oldData) ? updated : { positions: updated };
            }
          });
        }
        break;
      }
      
      case 'position_close': {
        // Handle position_close message
        const closePayload = (message as PositionCloseMessage).data;
        const positionPayload = closePayload?.position as Position;
        const botId = message.bot_id || positionPayload?.bot_config_id || positionPayload?.bot_id;
        
        if (botId && positionPayload && positionPayload.id) {
          queryClient.setQueryData(['bot-positions', botId], (oldData: { positions: Position[] } | Position[] | null | undefined) => {
            const positionsArray = Array.isArray(oldData) ? oldData : oldData?.positions || [];
            
            // Update the closed position
            const existingIndex = positionsArray.findIndex((p) => p.id === positionPayload.id);
            
            if (existingIndex >= 0) {
              const updated = [...positionsArray];
              updated[existingIndex] = {
                ...updated[existingIndex],
                ...positionPayload,
                status: 'closed' as const,
              };
              return Array.isArray(oldData) ? updated : { positions: updated };
            } else {
              // If position doesn't exist, add it (shouldn't happen, but handle gracefully)
              const updated = [...positionsArray, { ...positionPayload, status: 'closed' as const }];
              return Array.isArray(oldData) ? updated : { positions: updated };
            }
          });
          
          // Also update bot stats if summary is provided
          if (closePayload?.summary) {
            queryClient.setQueryData(['bots'], (oldData: { bots: BotConfig[]; count: number } | undefined) => {
              if (!oldData?.bots) return oldData;
              
              return {
                ...oldData,
                bots: oldData.bots.map((bot) => {
                  if (bot.id === botId) {
                    return {
                      ...bot,
                      total_trades: closePayload.summary.total_trades,
                      winning_trades: closePayload.summary.winning_trades,
                      total_profit_idr: closePayload.summary.total_profit_idr,
                      win_rate: closePayload.summary.win_rate,
                    };
                  }
                  return bot;
                }),
              };
            });
          }
        }
        break;
      }

      case 'order_update': {
        // Update cache directly instead of invalidating (no HTTP refetch)
        const orderPayload = message.payload as Order & { parent_id?: number };
        // Get bot_id from parent_id, or use selectedBot if parent_id is 0/undefined
        const botId = orderPayload?.parent_id && orderPayload.parent_id > 0 
          ? orderPayload.parent_id 
          : selectedBot?.id;
        
        if (botId && orderPayload && orderPayload.id) {
          queryClient.setQueryData(['bot-orders', botId], (oldData: Order[] | null | undefined) => {
            if (!oldData) return [orderPayload as Order];
            
            // Check if order already exists (match by id or order_id)
            const existingIndex = oldData.findIndex((o) => 
              o.id === orderPayload.id || o.order_id === orderPayload.order_id
            );
            
            if (existingIndex >= 0) {
              // If order is cancelled, remove it from the list
              if (orderPayload.status === 'cancelled') {
                return oldData.filter((o) => 
                  o.id !== orderPayload.id && o.order_id !== orderPayload.order_id
                );
              }
              
              // Update existing order - merge to preserve any fields not in the update
              const updated = [...oldData];
              updated[existingIndex] = {
                ...updated[existingIndex],
                ...orderPayload,
                // Ensure all required fields are present
                id: orderPayload.id,
                order_id: orderPayload.order_id || updated[existingIndex].order_id,
                pair: orderPayload.pair || updated[existingIndex].pair,
                side: orderPayload.side || updated[existingIndex].side,
                status: orderPayload.status || updated[existingIndex].status,
                price: orderPayload.price || updated[existingIndex].price,
                amount: orderPayload.amount || updated[existingIndex].amount,
                filled_amount: orderPayload.filled_amount ?? updated[existingIndex].filled_amount,
                filled_at: orderPayload.filled_at || updated[existingIndex].filled_at,
                is_paper_trade: orderPayload.is_paper_trade ?? updated[existingIndex].is_paper_trade,
                created_at: orderPayload.created_at && orderPayload.created_at !== '0001-01-01T00:00:00Z' 
                  ? orderPayload.created_at 
                  : updated[existingIndex].created_at,
                updated_at: orderPayload.updated_at && orderPayload.updated_at !== '0001-01-01T00:00:00Z'
                  ? orderPayload.updated_at
                  : updated[existingIndex].updated_at,
              } as Order;
              return updated;
            } else {
              // Don't add cancelled orders to the list
              if (orderPayload.status === 'cancelled') {
                return oldData;
              }
              // Add new order (prepend to show newest first)
              return [orderPayload as Order, ...oldData];
            }
          });
        }
        break;
      }

      case 'bot_pnl_update': {
        // Update cache directly instead of invalidating (no HTTP refetch)
        const pnlPayload = message.payload as { bot_id?: number; profit?: number };
        if (pnlPayload?.bot_id && typeof pnlPayload.profit === 'number') {
          const profit = pnlPayload.profit;
          queryClient.setQueryData(['bots'], (oldData: { bots: BotConfig[]; count: number } | undefined) => {
            if (!oldData?.bots) return oldData;
            
            return {
              ...oldData,
              bots: oldData.bots.map((bot) => {
                if (bot.id === pnlPayload.bot_id) {
                  return {
                    ...bot,
                    total_profit_idr: (bot.total_profit_idr || 0) + profit,
                  };
                }
                return bot;
              }),
            };
          });
        }
        break;
      }

      default:
        break;
    }
  }, [lastMessage, queryClient, selectedBot]);

  return (
    <div className="max-w-[1800px] mx-auto">
      <div className="space-y-6">
        {/* Two Column Layout */}
        {botsLoading ? (
          <div className="flex justify-center py-12">
            <LoadingSpinner />
          </div>
        ) : !bots || !bots.bots || bots.bots.length === 0 ? (
          <NoBotsEmptyState onCreate={() => {
            resetForm();
            setShowCreateWizard(true);
            setCreateError('');
          }} />
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Left Column - Bot List */}
            <div className="lg:col-span-1 space-y-3">
              {/* Summon Helper Button */}
              <button
                onClick={() => {
                  resetForm();
                  setShowCreateWizard(true);
                  setCreateError('');
                }}
                className="w-full px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors"
              >
                + Summon Helper
              </button>
              {bots?.bots?.map((bot: BotConfig) => (
                <button
                  key={bot.id}
                  onClick={() => setSelectedBot(bot)}
                  className={cn(
                    'w-full relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-xl p-4 overflow-hidden border transition-all text-left',
                    selectedBot?.id === bot.id
                      ? 'border-primary-500 shadow-lg shadow-primary-500/20'
                      : 'border-gray-800 hover:border-gray-700'
                  )}
                >
                  <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-xl pointer-events-none" />
                  <div className="relative z-10">
                    {/* Single Row: Status | Name + Badges | P&L */}
                    <div className="grid gap-3 items-center" style={{ gridTemplateColumns: 'auto 1fr auto' }}>
                      {/* Column 1: Status (small, centered vertically with entire content) */}
                      <div className="flex items-center self-center">
                        <span
                          className={cn(
                            'px-2 py-1 rounded-full text-xs font-semibold',
                            bot.status === 'running' && 'bg-green-500/20 text-green-500',
                            bot.status === 'stopped' && 'bg-gray-500/20 text-gray-400',
                            bot.status === 'error' && 'bg-red-500/20 text-red-500',
                            bot.status === 'starting' && 'bg-yellow-500/20 text-yellow-500'
                          )}
                        >
                          {bot.status === 'running' ? '🟢' : bot.status === 'error' ? '🔴' : '⚪'}
                        </span>
                      </div>
                      
                      {/* Column 2: Name + Badges (left-aligned) */}
                      <div className="flex flex-col gap-2">
                        <h3 className="text-base font-bold text-white truncate">{bot.name}</h3>
                        <div className="flex items-center gap-2">
                          <span className="px-2 py-1 bg-primary-500/20 text-primary-400 rounded text-xs font-medium">
                            {bot.pair.toUpperCase()}
                          </span>
                          <span className="px-2 py-1 bg-gray-800 text-gray-300 rounded text-xs font-medium">
                            {bot.type === 'market_maker' ? 'MM' : 'PH'}
                          </span>
                          <span className={cn(
                            'px-2 py-1 rounded text-xs font-medium',
                            bot.is_paper_trading 
                              ? 'bg-blue-500/20 text-blue-400' 
                              : 'bg-yellow-500/20 text-yellow-400'
                          )}>
                            {bot.is_paper_trading ? 'Paper' : 'Live'}
                          </span>
                        </div>
                      </div>
                      
                      {/* Column 3: P&L (centered vertically) */}
                      <div className="flex items-center self-center">
                        <span className={cn('font-bold text-sm whitespace-nowrap', (bot.total_profit_idr || 0) >= 0 ? 'text-green-500' : 'text-red-500')}>
                          {formatIDR(bot.total_profit_idr || 0)}
                        </span>
                      </div>
                    </div>
                  </div>
                </button>
              ))}
            </div>

            {/* Right Column - Bot Detail */}
            <div className="lg:col-span-2">
              {selectedBot ? (
                <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl overflow-hidden border border-gray-800">
                  <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
                  <div className="relative z-10">
                    {/* Bot Detail Header */}
                    <div className="px-6 py-4 border-b border-gray-800">
                      <div className="flex items-center justify-between">
                        <div>
                          <h2 className="text-2xl font-bold text-white mb-1">{selectedBot.name}</h2>
                          <div className="flex items-center gap-3">
                            <span className="text-gray-400">{getBotTypeBadge(selectedBot.type)}</span>
                            {selectedBot.type === 'market_maker' && selectedBot.pair && (
                              <>
                                <span className="text-gray-600">•</span>
                                <span className="text-white font-medium">{selectedBot.pair.toUpperCase()}</span>
                              </>
                            )}
                            <span className="text-gray-600">•</span>
                            <span className="text-gray-400">
                              {selectedBot.is_paper_trading ? 'Paper' : 'Live'}
                            </span>
                            <span className="text-gray-600">•</span>
                            <span
                              className={cn(
                                'px-3 py-1 rounded-full text-xs font-semibold',
                                selectedBot.status === 'running' && 'bg-green-500/20 text-green-500',
                                selectedBot.status === 'stopped' && 'bg-gray-500/20 text-gray-400',
                                selectedBot.status === 'error' && 'bg-red-500/20 text-red-500',
                                selectedBot.status === 'starting' && 'bg-yellow-500/20 text-yellow-500'
                              )}
                            >
                              {selectedBot.status.toUpperCase()}
                            </span>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          {selectedBot.status === 'stopped' || selectedBot.status === 'error' ? (
                            <>
                              <button
                                onClick={() => startBotMutation.mutate(selectedBot.id)}
                                disabled={startBotMutation.isPending}
                                className="p-2 bg-green-600 hover:bg-green-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
                                title="Start Bot"
                              >
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                </svg>
                              </button>
                              <button
                                onClick={() => openEditModal(selectedBot)}
                                className="p-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors"
                                title="Edit Config"
                              >
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                                </svg>
                              </button>
                            </>
                          ) : (
                            <button
                              onClick={() => stopBotMutation.mutate(selectedBot.id)}
                              disabled={stopBotMutation.isPending}
                              className="p-2 bg-red-600 hover:bg-red-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
                              title="Stop Bot"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
                              </svg>
                            </button>
                          )}
                          <button
                            onClick={() => {
                              if (confirm('Are you sure you want to delete this bot?')) {
                                deleteBotMutation.mutate(selectedBot.id);
                                setSelectedBot(null);
                              }
                            }}
                            disabled={selectedBot.status === 'running' || deleteBotMutation.isPending}
                            className="p-2 bg-gray-700 hover:bg-gray-600 disabled:bg-gray-900 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
                            title="Delete Bot"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                            </svg>
                          </button>
                          {selectedBot.type === 'market_maker' && selectedBot.pair && (
                            <button
                              onClick={() => {
                                window.open(`https://indodax.com/trade/${selectedBot.pair.toUpperCase()}`, '_blank', 'noopener,noreferrer');
                              }}
                              className="p-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-lg transition-colors"
                              title="View Chart on Indodax"
                            >
                              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                              </svg>
                            </button>
                          )}
                        </div>
                      </div>
                    </div>

                    {/* Market Maker Bot Detail */}
                    {selectedBot.type === 'market_maker' && (
                      <>
                        {/* Stats Grid */}
                        <div className="grid gap-4 p-6 border-b border-gray-800 grid-cols-2 md:grid-cols-5">
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">Total Trades</p>
                            <p className="text-white font-bold text-xl">{selectedBot.total_trades || 0}</p>
                          </div>
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">Win Rate</p>
                            <p className="text-white font-bold text-xl">{formatPercent(selectedBot.win_rate || 0)}</p>
                          </div>
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">Total P&L</p>
                            <p className={cn('font-bold text-xl', (selectedBot.total_profit_idr || 0) >= 0 ? 'text-green-500' : 'text-red-500')}>
                              {formatNumber(selectedBot.total_profit_idr || 0, 0)}
                            </p>
                          </div>
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">IDR Balance</p>
                            <p className="text-white font-bold text-xl">{formatNumber(selectedBot.balances?.idr || 0, 0)}</p>
                          </div>
                          {selectedBot.pair && (() => {
                            const baseCurrencyKey = selectedBot.pair.replace(/idr$/i, '').toLowerCase();
                            const baseCurrency = baseCurrencyKey.toUpperCase();
                            const baseBalance = selectedBot.balances?.[baseCurrencyKey] || 0;
                            return (
                              <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                                <p className="text-gray-400 text-sm mb-1">{baseCurrency} Balance</p>
                                <p className="text-white font-bold text-xl">{baseBalance.toFixed(4)}</p>
                              </div>
                            );
                          })()}
                        </div>

                        {/* Spread Information */}
                        <div className="p-6 border-b border-gray-800">
                          {spreadInfo && (spreadInfo.buyPrice !== undefined || spreadInfo.sellPrice !== undefined || spreadInfo.spreadPercent !== undefined) ? (
                            <div className="space-y-4">
                              <h3 className="text-xl font-bold text-white mb-4">Market Spread</h3>
                              
                              {/* Warning when spread is too tight */}
                              {spreadInfo.spreadPercent !== undefined && 
                               selectedBot.min_gap_percent && 
                               spreadInfo.spreadPercent < selectedBot.min_gap_percent && (
                                <div className="bg-yellow-500/10 border border-yellow-500/30 rounded-lg p-4">
                                  <div className="flex items-start gap-3">
                                    <svg className="w-5 h-5 text-yellow-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                                    </svg>
                                    <div className="flex-1">
                                      <p className="text-sm text-yellow-400 font-medium">Waiting for Better Spread</p>
                                      <p className="text-sm text-yellow-300 mt-1">
                                        Current spread ({formatPercent(spreadInfo.spreadPercent)}) is below minimum required ({formatPercent(selectedBot.min_gap_percent)}). 
                                        Bot is waiting for a profitable spread before placing orders.
                                      </p>
                                    </div>
                                  </div>
                                </div>
                              )}
                              
                              {/* Spread details */}
                              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                                {spreadInfo.buyPrice !== undefined && (
                                  <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                                    <p className="text-gray-400 text-sm mb-1">Buy Price (Bid)</p>
                                    <p className="text-white font-bold text-xl">{formatNumber(spreadInfo.buyPrice, 0)}</p>
                                  </div>
                                )}
                                {spreadInfo.sellPrice !== undefined && (
                                  <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                                    <p className="text-gray-400 text-sm mb-1">Sell Price (Ask)</p>
                                    <p className="text-white font-bold text-xl">{formatNumber(spreadInfo.sellPrice, 0)}</p>
                                  </div>
                                )}
                                {spreadInfo.spreadPercent !== undefined && (
                                  <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                                    <p className="text-gray-400 text-sm mb-1">Spread</p>
                                    <p className={cn(
                                      'font-bold text-xl',
                                      spreadInfo.spreadPercent >= (selectedBot.min_gap_percent || 0) 
                                        ? 'text-green-500' 
                                        : 'text-yellow-500'
                                    )}>
                                      {formatPercent(spreadInfo.spreadPercent)}
                                    </p>
                                  </div>
                                )}
                              </div>
                            </div>
                          ) : selectedBot.status === 'running' ? (
                            <div className="text-center py-2">
                              <div className="flex items-center justify-center gap-2">
                                <div className="w-1.5 h-1.5 bg-primary-500 rounded-full animate-pulse"></div>
                                <p className="text-gray-500 text-xs animate-pulse">Waiting for market data...</p>
                              </div>
                            </div>
                          ) : null}
                        </div>

                        {/* Orders Section */}
                        <div className="p-6">
                          <h3 className="text-xl font-bold text-white mb-4">Orders</h3>
                          
                          {ordersLoading ? (
                            <div className="flex justify-center py-8">
                              <LoadingSpinner />
                            </div>
                          ) : orders && orders.length > 0 ? (
                            <div className="overflow-x-auto custom-scrollbar">
                              <table className="w-full">
                                <thead>
                                  <tr className="border-b border-gray-800">
                                    <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Side</th>
                                    <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Status</th>
                                    <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Price</th>
                                    <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Amount</th>
                                    <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Filled</th>
                                    <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Created</th>
                                  </tr>
                                </thead>
                                <tbody>
                                  {[...orders].sort((a, b) => {
                                    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
                                  }).map((order) => (
                                    <tr key={order.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                                      <td className="px-4 py-3">
                                        <span className={cn(
                                          'px-2 py-1 rounded text-xs font-medium',
                                          order.side === 'buy' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'
                                        )}>
                                          {order.side.toUpperCase()}
                                        </span>
                                      </td>
                                      <td className="px-4 py-3">
                                        <span className={cn(
                                          'px-2 py-1 rounded text-xs font-medium',
                                          order.status === 'filled' ? 'bg-green-500/20 text-green-400' :
                                          order.status === 'open' ? 'bg-blue-500/20 text-blue-400' :
                                          order.status === 'pending' ? 'bg-yellow-500/20 text-yellow-400' :
                                          order.status === 'cancelled' ? 'bg-gray-500/20 text-gray-400' :
                                          'bg-red-500/20 text-red-400'
                                        )}>
                                          {order.status.toUpperCase()}
                                        </span>
                                      </td>
                                      <td className="px-4 py-3 text-right text-white">{formatNumber(order.price, 0)}</td>
                                      <td className="px-4 py-3 text-right text-white">{order.amount.toFixed(8)}</td>
                                      <td className="px-4 py-3 text-right text-gray-400">{order.filled_amount.toFixed(8)}</td>
                                      <td className="px-4 py-3 text-gray-400 text-xs">
                                        {new Date(order.created_at).toLocaleString()}
                                      </td>
                                    </tr>
                                  ))}
                                </tbody>
                              </table>
                            </div>
                          ) : (
                            <div className="bg-gray-900/50 rounded-lg p-8 border border-gray-800 text-center">
                              <p className="text-gray-400">No orders yet</p>
                            </div>
                          )}
                        </div>
                      </>
                    )}

                    {/* Pump Hunter Bot Detail */}
                    {selectedBot.type === 'pump_hunter' && (
                      <>
                        {/* Stats Grid */}
                        <div className="grid gap-4 p-6 border-b border-gray-800 grid-cols-2 md:grid-cols-4">
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">Total Trades</p>
                            <p className="text-white font-bold text-xl">{selectedBot.total_trades || 0}</p>
                          </div>
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">Win Rate</p>
                            <p className="text-white font-bold text-xl">{formatPercent(selectedBot.win_rate || 0)}</p>
                          </div>
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">Total P&L</p>
                            <p className={cn('font-bold text-xl', (selectedBot.total_profit_idr || 0) >= 0 ? 'text-green-500' : 'text-red-500')}>
                              {formatNumber(selectedBot.total_profit_idr || 0, 0)}
                            </p>
                          </div>
                          <div className="bg-gray-900/50 rounded-lg p-4 border border-gray-800">
                            <p className="text-gray-400 text-sm mb-1">IDR Balance</p>
                            <p className="text-white font-bold text-xl">{formatNumber(selectedBot.balances?.idr || 0, 0)}</p>
                          </div>
                        </div>

                        {/* Positions Section */}
                        <div className="p-6 border-b border-gray-800">
                          <h3 className="text-xl font-bold text-white mb-4">Positions</h3>
                          
                          {positionsLoading ? (
                            <div className="flex justify-center py-8">
                              <LoadingSpinner />
                            </div>
                          ) : positions && (positions.positions?.length > 0 || (Array.isArray(positions) && positions.length > 0)) ? (
                            <>
                              {/* Open Positions Table */}
                              {(() => {
                                const allPositions = Array.isArray(positions) ? positions : positions.positions;
                                const openPositions = allPositions.filter((p) => 
                                  p.status === 'pending' || 
                                  p.status === 'buying' || 
                                  p.status === 'open' || 
                                  p.status === 'selling'
                                );
                                
                                return openPositions.length > 0 ? (
                                  <div className="mb-6">
                                    <h4 className="text-lg font-semibold text-white mb-3">Open Positions</h4>
                                    <div className="overflow-x-auto custom-scrollbar">
                                      <table className="w-full">
                                        <thead>
                                          <tr className="border-b border-gray-800">
                                            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Pair</th>
                                            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Status</th>
                                            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Current Price</th>
                                            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Entry Price</th>
                                            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Exit Price</th>
                                            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Profit</th>
                                            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Entry Time</th>
                                          </tr>
                                        </thead>
                                        <tbody>
                                          {openPositions.map((position) => {
                                            const coin = getCoin(position.pair);
                                            const currentPrice = coin?.current_price;
                                            
                                            // Calculate profit for open positions based on current price
                                            let profitIdr = position.profit_idr ?? 0;
                                            let profitPercent = position.profit_percent ?? 0;
                                            
                                            // Ensure profitIdr is a valid number
                                            if (typeof profitIdr !== 'number' || isNaN(profitIdr)) {
                                              profitIdr = 0;
                                            }
                                            
                                            if (position.status === 'open' && currentPrice && position.entry_price > 0) {
                                              // Calculate profit percentage
                                              profitPercent = ((currentPrice - position.entry_price) / position.entry_price) * 100;
                                              
                                              // Ensure profitPercent is valid
                                              if (isNaN(profitPercent)) {
                                                profitPercent = 0;
                                              }
                                              
                                              // Calculate profit in IDR
                                              // Use entry_quantity if available, otherwise calculate from entry_amount_idr
                                              if (position.entry_quantity && position.entry_quantity > 0) {
                                                const calculatedProfit = (currentPrice - position.entry_price) * position.entry_quantity;
                                                if (!isNaN(calculatedProfit)) {
                                                  profitIdr = calculatedProfit;
                                                }
                                              } else if (position.entry_amount_idr && position.entry_amount_idr > 0) {
                                                // Calculate profit based on entry amount
                                                const calculatedProfit = position.entry_amount_idr * (profitPercent / 100);
                                                if (!isNaN(calculatedProfit)) {
                                                  profitIdr = calculatedProfit;
                                                }
                                              }
                                            }
                                            
                                            // Final validation to ensure we never display NaN
                                            if (isNaN(profitIdr)) {
                                              profitIdr = 0;
                                            }
                                            if (isNaN(profitPercent)) {
                                              profitPercent = 0;
                                            }
                                            
                                            return (
                                              <tr key={position.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                                                <td className="px-4 py-3 text-white font-medium">{position.pair.toUpperCase()}</td>
                                                <td className="px-4 py-3">
                                                  <span className={cn(
                                                    'px-2 py-1 rounded text-xs font-medium',
                                                    position.status === 'open' ? 'bg-green-500/20 text-green-400' :
                                                    position.status === 'pending' ? 'bg-yellow-500/20 text-yellow-400' :
                                                    position.status === 'buying' ? 'bg-green-500/20 text-green-400' :
                                                    position.status === 'selling' ? 'bg-red-500/20 text-red-400' :
                                                    'bg-gray-500/20 text-gray-400'
                                                  )}>
                                                    {position.status.toUpperCase()}
                                                  </span>
                                                </td>
                                                <td className="px-4 py-3 text-right text-white">
                                                  {currentPrice ? formatNumber(currentPrice, 0) : '-'}
                                                </td>
                                                <td className="px-4 py-3 text-right text-white">
                                                  {formatNumber(position.entry_price, 0)}
                                                </td>
                                                <td className="px-4 py-3 text-right text-white">
                                                  {position.exit_price ? formatNumber(position.exit_price, 0) : '-'}
                                                </td>
                                                <td className="px-4 py-3 text-right">
                                                  {position.status === 'buying' || position.status === 'pending' ? (
                                                    <span className="text-gray-500">-</span>
                                                  ) : (
                                                    <div>
                                                      <div className={cn(
                                                        'font-bold',
                                                        profitIdr >= 0 ? 'text-green-500' : 'text-red-500'
                                                      )}>
                                                        {formatIDR(profitIdr)}
                                                      </div>
                                                      <div className={cn(
                                                        'text-xs',
                                                        profitPercent >= 0 ? 'text-green-400' : 'text-red-400'
                                                      )}>
                                                        {formatPercent(profitPercent)}
                                                      </div>
                                                    </div>
                                                  )}
                                                </td>
                                                <td className="px-4 py-3 text-gray-400 text-xs">
                                                  {position.entry_at ? (
                                                    <div className="flex flex-col">
                                                      <span>{new Date(position.entry_at).toLocaleDateString()}</span>
                                                      <span>{new Date(position.entry_at).toLocaleTimeString()}</span>
                                                    </div>
                                                  ) : '-'}
                                                </td>
                                              </tr>
                                            );
                                          })}
                                        </tbody>
                                      </table>
                                    </div>
                                  </div>
                                ) : (
                                  <div className="bg-gray-900/50 rounded-lg p-12 border border-gray-800 text-center mb-6">
                                    {selectedBot.status === 'running' ? (
                                      <div className="flex flex-col items-center">
                                        <div className="relative mb-6">
                                          <div className={cn(
                                            "absolute inset-0 rounded-full blur-2xl animate-pulse",
                                            (selectedBot.total_profit_idr ?? 0) >= 0 
                                              ? "bg-green-500/20" 
                                              : "bg-red-500/20"
                                          )} />
                                          <img 
                                            src={(selectedBot.total_profit_idr ?? 0) >= 0 ? "/tuyul-work-win.png" : "/tuyul-work-lost.png"} 
                                            alt="Tuyul Working" 
                                            className="relative w-80 h-auto drop-shadow-2xl" 
                                          />
                                        </div>
                                        <div className="space-y-2">
                                          <h4 className={cn(
                                            "text-2xl font-bold bg-clip-text text-transparent animate-pulse",
                                            (selectedBot.total_profit_idr ?? 0) >= 0
                                              ? "bg-gradient-to-r from-primary-400 via-green-400 to-primary-400"
                                              : "bg-gradient-to-r from-red-400 via-orange-400 to-red-400"
                                          )}>
                                            sneaking around looking for easy money...
                                          </h4>
                                          <p className="text-gray-500 text-sm italic">"i'm on a heist master! let me work my magic..."</p>
                                        </div>
                                      </div>
                                    ) : selectedBot.status === 'stopped' ? (
                                      <div className="flex flex-col items-center">
                                        <div className="relative mb-6">
                                          {(() => {
                                            const profit = selectedBot.total_profit_idr ?? 0;
                                            if (profit === 0) {
                                              return (
                                                <>
                                                  <div className="absolute inset-0 rounded-full blur-2xl bg-gray-500/20" />
                                                  <img 
                                                    src="/tuyul-bored.png" 
                                                    alt="Tuyul" 
                                                    className="relative w-64 h-auto drop-shadow-2xl" 
                                                  />
                                                </>
                                              );
                                            } else if (profit > 0) {
                                              return (
                                                <>
                                                  <div className="absolute inset-0 rounded-full blur-2xl bg-green-500/20" />
                                                  <img 
                                                    src="/tuyul-win.png" 
                                                    alt="Tuyul" 
                                                    className="relative w-64 h-auto drop-shadow-2xl" 
                                                  />
                                                </>
                                              );
                                            } else {
                                              return (
                                                <>
                                                  <div className="absolute inset-0 rounded-full blur-2xl bg-red-500/20" />
                                                  <img 
                                                    src="/tuyul-lost.png" 
                                                    alt="Tuyul" 
                                                    className="relative w-64 h-auto drop-shadow-2xl" 
                                                  />
                                                </>
                                              );
                                            }
                                          })()}
                                        </div>
                                        {(() => {
                                          const profit = selectedBot.total_profit_idr ?? 0;
                                          if (profit > 0) {
                                            return (
                                              <div className="space-y-2">
                                                <p className="text-lg font-semibold text-green-400 italic">"hehehe! look at all this loot i collected!"</p>
                                              </div>
                                            );
                                          } else if (profit < 0) {
                                            return (
                                              <div className="space-y-2">
                                                <p className="text-lg font-semibold text-red-400 italic">"uh oh master... i might have... lost some money? please don't fire me!"</p>
                                              </div>
                                            );
                                          } else {
                                            return (
                                              <div className="space-y-2">
                                                <p className="text-lg font-semibold text-gray-400 italic">"master... i'm just sitting here doing nothing... can i go steal something?"</p>
                                              </div>
                                            );
                                          }
                                        })()}
                                      </div>
                                    ) : (
                                      <p className="text-gray-400">No open positions</p>
                                    )}
                                  </div>
                                );
                              })()}
                              
                              {/* Closed Positions Table */}
                              {(() => {
                                const allPositions = Array.isArray(positions) ? positions : positions.positions;
                                const closedPositions = allPositions.filter((p) => p.status === 'closed' || p.status === 'error');
                                
                                return closedPositions.length > 0 ? (
                                  <div>
                                    <h4 className="text-lg font-semibold text-white mb-3">Closed Positions</h4>
                                    <div className="overflow-x-auto custom-scrollbar">
                                      <table className="w-full">
                                        <thead>
                                          <tr className="border-b border-gray-800">
                                            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Pair</th>
                                            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Entry Price</th>
                                            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Exit Price</th>
                                            <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Profit</th>
                                            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Entry Time</th>
                                            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Exit Time</th>
                                          </tr>
                                        </thead>
                                        <tbody>
                                          {closedPositions.map((position) => {
                                            let profitIdr = position.profit_idr ?? 0;
                                            let profitPercent = position.profit_percent ?? 0;
                                            
                                            // Ensure values are valid numbers
                                            if (typeof profitIdr !== 'number' || isNaN(profitIdr)) {
                                              profitIdr = 0;
                                            }
                                            if (typeof profitPercent !== 'number' || isNaN(profitPercent)) {
                                              profitPercent = 0;
                                            }
                                            
                                            return (
                                              <tr key={position.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                                                <td className="px-4 py-3 text-white font-medium">{position.pair.toUpperCase()}</td>
                                                <td className="px-4 py-3 text-right text-white">
                                                  {formatNumber(position.entry_price, 0)}
                                                </td>
                                                <td className="px-4 py-3 text-right text-white">
                                                  {position.exit_price ? formatNumber(position.exit_price, 0) : '-'}
                                                </td>
                                                <td className="px-4 py-3 text-right">
                                                  <div>
                                                    <div className={cn(
                                                      'font-bold',
                                                      profitIdr >= 0 ? 'text-green-500' : 'text-red-500'
                                                    )}>
                                                      {formatIDR(profitIdr)}
                                                    </div>
                                                    <div className={cn(
                                                      'text-xs',
                                                      profitPercent >= 0 ? 'text-green-400' : 'text-red-400'
                                                    )}>
                                                      {formatPercent(profitPercent)}
                                                    </div>
                                                  </div>
                                                </td>
                                                <td className="px-4 py-3 text-gray-400 text-xs">
                                                  {position.entry_at ? new Date(position.entry_at).toLocaleTimeString() : '-'}
                                                </td>
                                                <td className="px-4 py-3 text-gray-400 text-xs">
                                                  {position.exit_at ? new Date(position.exit_at).toLocaleTimeString() : '-'}
                                                </td>
                                              </tr>
                                            );
                                          })}
                                        </tbody>
                                      </table>
                                    </div>
                                  </div>
                                ) : null;
                              })()}
                            </>
                          ) : (
                            <div className="bg-gray-900/50 rounded-lg p-12 border border-gray-800 text-center">
                              {selectedBot.status === 'running' ? (
                                <div className="flex flex-col items-center">
                                  <div className="relative mb-6">
                                    <div className={cn(
                                      "absolute inset-0 rounded-full blur-2xl animate-pulse",
                                      (selectedBot.total_profit_idr ?? 0) >= 0 
                                        ? "bg-green-500/20" 
                                        : "bg-red-500/20"
                                    )} />
                                    <img 
                                      src={(selectedBot.total_profit_idr ?? 0) >= 0 ? "/tuyul-work-win.png" : "/tuyul-work-lost.png"} 
                                      alt="Tuyul Working" 
                                      className="relative w-80 h-auto drop-shadow-2xl" 
                                    />
                                  </div>
                                  <div className="space-y-2">
                                    <h4 className={cn(
                                      "text-2xl font-bold bg-clip-text text-transparent animate-pulse",
                                      (selectedBot.total_profit_idr ?? 0) >= 0
                                        ? "bg-gradient-to-r from-primary-400 via-green-400 to-primary-400"
                                        : "bg-gradient-to-r from-red-400 via-orange-400 to-red-400"
                                    )}>
                                      sneaking around looking for easy money...
                                    </h4>
                                    <p className="text-gray-500 text-sm italic">"i'm on a heist master! let me work my magic..."</p>
                                  </div>
                                </div>
                              ) : selectedBot.status === 'stopped' ? (
                                <div className="flex flex-col items-center">
                                  <div className="relative mb-6">
                                    {(() => {
                                      const profit = selectedBot.total_profit_idr ?? 0;
                                      if (profit === 0) {
                                        return (
                                          <>
                                            <div className="absolute inset-0 rounded-full blur-2xl bg-gray-500/20" />
                                            <img 
                                              src="/tuyul-bored.png" 
                                              alt="Tuyul" 
                                              className="relative w-64 h-auto drop-shadow-2xl" 
                                            />
                                          </>
                                        );
                                      } else if (profit > 0) {
                                        return (
                                          <>
                                            <div className="absolute inset-0 rounded-full blur-2xl bg-green-500/20" />
                                            <img 
                                              src="/tuyul-win.png" 
                                              alt="Tuyul" 
                                              className="relative w-64 h-auto drop-shadow-2xl" 
                                            />
                                          </>
                                        );
                                      } else {
                                        return (
                                          <>
                                            <div className="absolute inset-0 rounded-full blur-2xl bg-red-500/20" />
                                            <img 
                                              src="/tuyul-lost.png" 
                                              alt="Tuyul" 
                                              className="relative w-64 h-auto drop-shadow-2xl" 
                                            />
                                          </>
                                        );
                                      }
                                    })()}
                                  </div>
                                  {(() => {
                                    const profit = selectedBot.total_profit_idr ?? 0;
                                    if (profit > 0) {
                                      return (
                                        <div className="space-y-2">
                                          <p className="text-lg font-semibold text-green-400 italic">"hehehe! look at all this loot i collected!"</p>
                                        </div>
                                      );
                                    } else if (profit < 0) {
                                      return (
                                        <div className="space-y-2">
                                          <p className="text-lg font-semibold text-red-400 italic">"uh oh master... i might have... lost some money? please don't fire me!"</p>
                                        </div>
                                      );
                                    } else {
                                      return (
                                        <div className="space-y-2">
                                          <p className="text-lg font-semibold text-gray-400 italic">"master... i'm just sitting here doing nothing... can i go steal something?"</p>
                                        </div>
                                      );
                                    }
                                  })()}
                                </div>
                              ) : (
                                <p className="text-gray-400">No positions yet</p>
                              )}
                            </div>
                          )}
                        </div>

                        {/* Orders Section - Only show if there are positions */}
                        {positions && (positions.positions?.length > 0 || (Array.isArray(positions) && positions.length > 0)) && (
                          <div className="p-6">
                            <h3 className="text-xl font-bold text-white mb-4">Orders</h3>
                            
                            {ordersLoading ? (
                              <div className="flex justify-center py-8">
                                <LoadingSpinner />
                              </div>
                            ) : orders && orders.length > 0 ? (
                              <div className="overflow-x-auto custom-scrollbar">
                                <table className="w-full">
                                  <thead>
                                    <tr className="border-b border-gray-800">
                                      <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Pair</th>
                                      <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Side</th>
                                      <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Status</th>
                                      <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Price</th>
                                      <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Amount</th>
                                      <th className="px-4 py-3 text-right text-xs font-semibold text-gray-400 uppercase">Filled</th>
                                      <th className="px-4 py-3 text-left text-xs font-semibold text-gray-400 uppercase">Created</th>
                                    </tr>
                                </thead>
                                <tbody>
                                  {[...orders].sort((a, b) => {
                                    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
                                  }).map((order) => (
                                    <tr key={order.id} className="border-b border-gray-800/50 hover:bg-gray-800/30 transition-colors">
                                      <td className="px-4 py-3 text-white font-medium">{order.pair.toUpperCase()}</td>
                                      <td className="px-4 py-3">
                                        <span className={cn(
                                          'px-2 py-1 rounded text-xs font-medium',
                                          order.side === 'buy' ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400'
                                        )}>
                                          {order.side.toUpperCase()}
                                        </span>
                                      </td>
                                      <td className="px-4 py-3">
                                        <span className={cn(
                                          'px-2 py-1 rounded text-xs font-medium',
                                          order.status === 'filled' ? 'bg-green-500/20 text-green-400' :
                                          order.status === 'open' ? 'bg-blue-500/20 text-blue-400' :
                                          order.status === 'pending' ? 'bg-yellow-500/20 text-yellow-400' :
                                          order.status === 'cancelled' ? 'bg-gray-500/20 text-gray-400' :
                                          'bg-red-500/20 text-red-400'
                                        )}>
                                          {order.status.toUpperCase()}
                                        </span>
                                      </td>
                                      <td className="px-4 py-3 text-right text-white">{formatNumber(order.price, 0)}</td>
                                      <td className="px-4 py-3 text-right text-white">{order.amount.toFixed(8)}</td>
                                      <td className="px-4 py-3 text-right text-gray-400">{order.filled_amount.toFixed(8)}</td>
                                      <td className="px-4 py-3 text-gray-400 text-xs">
                                        {new Date(order.created_at).toLocaleString()}
                                      </td>
                                    </tr>
                                  ))}
                                </tbody>
                              </table>
                            </div>
                          ) : (
                            <div className="bg-gray-900/50 rounded-lg p-8 border border-gray-800 text-center">
                              <p className="text-gray-400">No orders yet</p>
                            </div>
                          )}
                          </div>
                        )}
                      </>
                    )}
                  </div>
                </div>
              ) : (
                <div className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl overflow-hidden border border-gray-800 h-full flex items-center justify-center min-h-[500px]">
                  <div className="text-center p-12">
                    <div className="w-16 h-16 bg-gray-800 rounded-full flex items-center justify-center mx-auto mb-4">
                      <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 15l-2 5L9 9l11 4-5 2zm0 0l5 5M7.188 2.239l.777 2.897M5.136 7.965l-2.898-.777M13.95 4.05l-2.122 2.122m-5.657 5.656l-2.12 2.122" />
                      </svg>
                    </div>
                    <h3 className="text-lg font-semibold text-white mb-2">Select a Bot</h3>
                    <p className="text-gray-400">Click on a bot from the list to view details and manage orders</p>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Create Bot Wizard Modal */}
        {showCreateWizard && (
          <div
            className="fixed inset-0 left-0 right-0 top-0 bottom-0 bg-black/80 backdrop-blur-sm z-[9999] flex items-center justify-center p-4"
            style={{ margin: 0 }}
            onClick={handleCloseModal}
          >
            <div
              className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-8 max-w-5xl w-full border border-gray-800 max-h-[90vh] overflow-y-auto custom-scrollbar"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
              <div className="relative z-10">
                <h2 className="text-2xl font-bold text-white mb-6">Summon New Helper</h2>
                
                {/* Error Display */}
                {createError && (
                  <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-4 mb-6">
                    <div className="flex items-start gap-3">
                      <svg className="w-5 h-5 text-red-500 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                      <div className="flex-1">
                        <p className="text-sm text-red-400 font-medium">Error creating bot</p>
                        <p className="text-sm text-red-300 mt-1">{createError}</p>
                      </div>
                      <button
                        type="button"
                        onClick={() => setCreateError('')}
                        className="text-red-400 hover:text-red-300"
                      >
                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                        </svg>
                      </button>
                    </div>
                  </div>
                )}

                <form onSubmit={handleSubmit} className="space-y-4">
                  {/* Grid Layout: Left column for basic info, Right column for advanced config */}
                  <div className="grid gap-6 grid-cols-1 lg:grid-cols-2">
                    {/* LEFT COLUMN - Basic Configuration */}
                    <div className="space-y-4">
                      {/* Bot Name */}
                      <div>
                        <label className="block text-sm font-medium text-gray-300 mb-2">Bot Name</label>
                        <input
                          type="text"
                          value={formData.name}
                          onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                          className="w-full px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg text-white focus:outline-none focus:border-primary-500"
                          placeholder="My Trading Bot"
                          required
                          autoFocus
                        />
                      </div>

                      {/* Bot Type */}
                      <div>
                        <label className="block text-sm font-medium text-gray-300 mb-2">Bot Type</label>
                        <div className="grid grid-cols-2 gap-3">
                      <button
                        type="button"
                        onClick={() => {
                          if (!isBotTypeDisabled) {
                            setBotType('market_maker');
                            setFormData({ ...formData, type: 'market_maker' });
                          }
                        }}
                        disabled={isBotTypeDisabled}
                        className={cn(
                          'p-4 rounded-lg border-2 transition-all text-left',
                          botType === 'market_maker'
                            ? 'border-primary-500 bg-primary-500/10'
                            : 'border-gray-700 hover:border-gray-600',
                          isBotTypeDisabled && 'opacity-60 cursor-not-allowed'
                        )}
                      >
                        <div className="text-2xl mb-2">🤖</div>
                        <div className="font-semibold text-white mb-1">Market Maker</div>
                        <div className="text-xs text-gray-400">Spread capture</div>
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          if (!isBotTypeDisabled) {
                            setBotType('pump_hunter');
                            setFormData({ ...formData, type: 'pump_hunter' });
                          }
                        }}
                        disabled={isBotTypeDisabled}
                        className={cn(
                          'p-4 rounded-lg border-2 transition-all text-left',
                          botType === 'pump_hunter'
                            ? 'border-primary-500 bg-primary-500/10'
                            : 'border-gray-700 hover:border-gray-600',
                          isBotTypeDisabled && 'opacity-60 cursor-not-allowed'
                        )}
                      >
                        <div className="text-2xl mb-2">🎯</div>
                        <div className="font-semibold text-white mb-1">Pump Hunter</div>
                        <div className="text-xs text-gray-400">Momentum trading</div>
                      </button>
                    </div>
                    {isBotTypeDisabled && (
                      <p className="text-xs text-gray-400 mt-1">Bot type is pre-selected from market analysis</p>
                    )}
                  </div>

                  {/* Trading Pair - Only for Market Maker */}
                  {formData.type === 'market_maker' && (
                    <div>
                      <label className="block text-sm font-medium text-gray-300 mb-2">Trading Pair</label>
                      <input
                        type="text"
                        value={formData.pair}
                        onChange={(e) => setFormData({ ...formData, pair: e.target.value.toLowerCase() })}
                        disabled={isPairPreFilled}
                        className={cn(
                          "w-full px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg text-white focus:outline-none focus:border-primary-500",
                          isPairPreFilled && "opacity-60 cursor-not-allowed"
                        )}
                        placeholder="btcidr"
                        required
                      />
                      {isPairPreFilled && (
                        <p className="text-xs text-gray-400 mt-1">Pair is pre-filled from market analysis</p>
                      )}
                    </div>
                  )}

                  {/* Pair Filters - Only for Pump Hunter */}
                  {formData.type === 'pump_hunter' && (
                    <>
                      <div>
                        <label className="block text-sm font-medium text-gray-300 mb-2">
                          Excluded Pairs (Optional)
                        </label>
                        <input
                          type="text"
                          placeholder="e.g., usdtidr, usdcidr"
                          value={formData.entry_rules?.excluded_pairs?.join(', ') || ''}
                          onChange={(e) => {
                            const pairs = e.target.value
                              .split(',')
                              .map((p) => p.trim().toLowerCase())
                              .filter((p) => p.length > 0);
                            setFormData({
                              ...formData,
                              entry_rules: {
                                ...formData.entry_rules!,
                                excluded_pairs: pairs,
                              },
                            });
                          }}
                          className="w-full px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-primary-500"
                        />
                      </div>

                    </>
                  )}

                  {/* Initial Balance - Only for Pump Hunter */}
                  {formData.type === 'pump_hunter' && (
                    <div>
                      <label className="block text-sm font-medium text-gray-300 mb-2">Initial Balance (IDR)</label>
                      <input
                        type="number"
                        min="10000"
                        step="10000"
                        value={formData.initial_balance_idr || 1000000}
                        onChange={(e) => setFormData({
                          ...formData,
                          initial_balance_idr: parseFloat(e.target.value),
                        })}
                        className="w-full px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg text-white focus:outline-none focus:border-primary-500"
                        required
                      />
                    </div>
                  )}

                      {/* Trading Mode */}
                      <div>
                        <label className="block text-sm font-medium text-gray-300 mb-2">Trading Mode</label>
                        <div className="flex gap-2">
                          <button
                            type="button"
                            onClick={() => setFormData({ ...formData, is_paper_trading: true })}
                            className={cn(
                              'flex-1 px-4 py-2 rounded-lg font-medium transition-all',
                              formData.is_paper_trading
                                ? 'bg-primary-600 text-white'
                                : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                            )}
                          >
                            📄 Paper
                          </button>
                          <button
                            type="button"
                            onClick={() => setFormData({ ...formData, is_paper_trading: false })}
                            disabled={!apiKey}
                            className={cn(
                              'flex-1 px-4 py-2 rounded-lg font-medium transition-all',
                              !formData.is_paper_trading
                                ? 'bg-primary-600 text-white'
                                : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700',
                              !apiKey && 'opacity-50 cursor-not-allowed'
                            )}
                            title={!apiKey ? 'Configure API key first' : ''}
                          >
                            💰 Live
                          </button>
                        </div>
                      </div>
                    </div>
                    {/* END LEFT COLUMN */}

                    {/* RIGHT COLUMN - Market Maker Configuration */}
                    {formData.type === 'market_maker' && (
                      <div className="space-y-3">
                        {/* Trading Parameters - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('entry')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Trading Parameters</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'entry' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'entry' && (
                            <div className="p-4 space-y-3">
                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Initial Balance (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={formData.initial_balance_idr || 1000000}
                                  onChange={(e) => setFormData({
                                    ...formData,
                                    initial_balance_idr: parseFloat(e.target.value),
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Order Size (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={formData.order_size_idr || 100000}
                                  onChange={(e) => setFormData({
                                    ...formData,
                                    order_size_idr: parseFloat(e.target.value),
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div className="grid grid-cols-2 gap-3">
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Min Gap %</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={formData.min_gap_percent || 0.5}
                                    onChange={(e) => setFormData({
                                      ...formData,
                                      min_gap_percent: parseFloat(e.target.value),
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>

                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Reposition %</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={formData.reposition_threshold_percent || 0.1}
                                    onChange={(e) => setFormData({
                                      ...formData,
                                      reposition_threshold_percent: parseFloat(e.target.value),
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Max Loss (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={formData.max_loss_idr || 500000}
                                  onChange={(e) => setFormData({
                                    ...formData,
                                    max_loss_idr: parseFloat(e.target.value),
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    )}

                    {/* RIGHT COLUMN - Pump Hunter Advanced Config (Collapsible) */}
                    {formData.type === 'pump_hunter' && (
                      <div className="space-y-3">
                        {/* Entry Rules - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('entry')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Entry Rules</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'entry' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'entry' && (
                            <div className="p-4 space-y-3">
                      
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="block text-xs text-gray-400 mb-1">Min Pump Score</label>
                          <input
                            type="number"
                            step="0.1"
                            value={formData.entry_rules?.min_pump_score || 50}
                            onChange={(e) => setFormData({
                              ...formData,
                              entry_rules: {
                                ...formData.entry_rules!,
                                min_pump_score: parseFloat(e.target.value),
                              },
                            })}
                            className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                          />
                        </div>
                        
                        <div>
                          <label className="block text-xs text-gray-400 mb-1">Min Positive TF</label>
                          <input
                            type="number"
                            min="1"
                            max="4"
                            value={formData.entry_rules?.min_timeframes_positive || 2}
                            onChange={(e) => setFormData({
                              ...formData,
                              entry_rules: {
                                ...formData.entry_rules!,
                                min_timeframes_positive: parseInt(e.target.value),
                              },
                            })}
                            className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                          />
                        </div>
                      </div>

                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Min 24h Volume (IDR)</label>
                        <input
                          type="number"
                          step="1000000"
                          value={formData.entry_rules?.min_24h_volume_idr || 1000000000}
                          onChange={(e) => setFormData({
                            ...formData,
                            entry_rules: {
                              ...formData.entry_rules!,
                              min_24h_volume_idr: parseFloat(e.target.value),
                            },
                          })}
                          className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                        />
                      </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Min Price (IDR)</label>
                                <input
                                  type="number"
                                  step="10"
                                  value={formData.entry_rules?.min_price_idr || 100}
                                  onChange={(e) => setFormData({
                                    ...formData,
                                    entry_rules: {
                                      ...formData.entry_rules!,
                                      min_price_idr: parseFloat(e.target.value),
                                    },
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>
                            </div>
                          )}
                        </div>

                        {/* Exit Rules - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('exit')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Exit Rules</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'exit' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'exit' && (
                            <div className="p-4 space-y-3">
                      
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <label className="block text-xs text-gray-400 mb-1">Target Profit %</label>
                          <input
                            type="number"
                            step="0.1"
                            value={formData.exit_rules?.target_profit_percent || 3}
                            onChange={(e) => setFormData({
                              ...formData,
                              exit_rules: {
                                ...formData.exit_rules!,
                                target_profit_percent: parseFloat(e.target.value),
                              },
                            })}
                            className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                          />
                        </div>
                        
                        <div>
                          <label className="block text-xs text-gray-400 mb-1">Stop Loss %</label>
                          <input
                            type="number"
                            step="0.1"
                            value={formData.exit_rules?.stop_loss_percent || 1.5}
                            onChange={(e) => setFormData({
                              ...formData,
                              exit_rules: {
                                ...formData.exit_rules!,
                                stop_loss_percent: parseFloat(e.target.value),
                              },
                            })}
                            className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                          />
                        </div>
                      </div>

                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="trailing-stop"
                          checked={formData.exit_rules?.trailing_stop_enabled || false}
                          onChange={(e) => setFormData({
                            ...formData,
                            exit_rules: {
                              ...formData.exit_rules!,
                              trailing_stop_enabled: e.target.checked,
                            },
                          })}
                          className="w-4 h-4 bg-gray-900 border-gray-700 rounded focus:ring-primary-500"
                        />
                        <label htmlFor="trailing-stop" className="text-sm text-gray-300">Enable Trailing Stop</label>
                      </div>

                      {formData.exit_rules?.trailing_stop_enabled && (
                        <div>
                          <label className="block text-xs text-gray-400 mb-1">Trailing Stop %</label>
                          <input
                            type="number"
                            step="0.1"
                            value={formData.exit_rules?.trailing_stop_percent || 1}
                            onChange={(e) => setFormData({
                              ...formData,
                              exit_rules: {
                                ...formData.exit_rules!,
                                trailing_stop_percent: parseFloat(e.target.value),
                              },
                            })}
                            className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                          />
                        </div>
                      )}

                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Max Hold Time (minutes)</label>
                        <input
                          type="number"
                          step="1"
                          value={formData.exit_rules?.max_hold_minutes || 30}
                          onChange={(e) => setFormData({
                            ...formData,
                            exit_rules: {
                              ...formData.exit_rules!,
                              max_hold_minutes: parseInt(e.target.value),
                            },
                          })}
                          className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                        />
                      </div>

                      <div className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          id="pump-score-exit"
                          checked={formData.exit_rules?.exit_on_pump_score_drop || false}
                          onChange={(e) => setFormData({
                            ...formData,
                            exit_rules: {
                              ...formData.exit_rules!,
                              exit_on_pump_score_drop: e.target.checked,
                            },
                          })}
                          className="w-4 h-4 bg-gray-900 border-gray-700 rounded focus:ring-primary-500"
                        />
                        <label htmlFor="pump-score-exit" className="text-sm text-gray-300">Exit on Pump Score Drop</label>
                      </div>

                              {formData.exit_rules?.exit_on_pump_score_drop && (
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Pump Score Drop Threshold</label>
                                  <input
                                    type="number"
                                    step="1"
                                    value={formData.exit_rules?.pump_score_drop_threshold || 20}
                                    onChange={(e) => setFormData({
                                      ...formData,
                                      exit_rules: {
                                        ...formData.exit_rules!,
                                        pump_score_drop_threshold: parseFloat(e.target.value),
                                      },
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                              )}
                            </div>
                          )}
                        </div>

                        {/* Risk Management - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('risk')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Risk Management</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'risk' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'risk' && (
                            <div className="p-4 space-y-3">
                      
                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Max Position Size (IDR)</label>
                        <input
                          type="number"
                          min="10000"
                          step="10000"
                          value={formData.risk_management?.max_position_idr || 500000}
                          onChange={(e) => setFormData({
                            ...formData,
                            risk_management: {
                              ...formData.risk_management!,
                              max_position_idr: parseFloat(e.target.value),
                            },
                          })}
                          className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                        />
                      </div>

                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Max Concurrent Positions</label>
                        <input
                          type="number"
                          min="1"
                          max="10"
                          value={formData.risk_management?.max_concurrent_positions || 3}
                          onChange={(e) => setFormData({
                            ...formData,
                            risk_management: {
                              ...formData.risk_management!,
                              max_concurrent_positions: parseInt(e.target.value),
                            },
                          })}
                          className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                        />
                      </div>

                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Daily Loss Limit (IDR)</label>
                        <input
                          type="number"
                          min="10000"
                          step="10000"
                          value={formData.risk_management?.daily_loss_limit_idr || 1000000}
                          onChange={(e) => setFormData({
                            ...formData,
                            risk_management: {
                              ...formData.risk_management!,
                              daily_loss_limit_idr: parseFloat(e.target.value),
                            },
                          })}
                          className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                        />
                      </div>

                      <div>
                        <label className="block text-xs text-gray-400 mb-1">Cooldown After Loss (minutes)</label>
                        <input
                          type="number"
                          step="1"
                          value={formData.risk_management?.cooldown_after_loss_minutes || 10}
                          onChange={(e) => setFormData({
                            ...formData,
                            risk_management: {
                              ...formData.risk_management!,
                              cooldown_after_loss_minutes: parseInt(e.target.value),
                            },
                          })}
                          className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                        />
                      </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Min Balance (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={formData.risk_management?.min_balance_idr || 100000}
                                  onChange={(e) => setFormData({
                                    ...formData,
                                    risk_management: {
                                      ...formData.risk_management!,
                                      min_balance_idr: parseFloat(e.target.value),
                                    },
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    )}

                  {/* Info */}
                  </div>
                  {/* END GRID */}

                  {/* Actions */}
                  <div className="flex gap-3 pt-4">
                    <button
                      type="button"
                      onClick={handleCloseModal}
                      className="flex-1 px-4 py-2 bg-gray-800 hover:bg-gray-700 text-white rounded-lg font-medium transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      disabled={createBotMutation.isPending}
                      className="flex-1 px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white rounded-lg font-medium transition-colors"
                    >
                      {createBotMutation.isPending ? 'Summoning...' : 'Summon Helper'}
                    </button>
                  </div>
                </form>
              </div>
            </div>
          </div>
        )}

        {/* Edit Bot Config Modal */}
        {showEditModal && selectedBot && (
          <div
            className="fixed inset-0 left-0 right-0 top-0 bottom-0 bg-black/80 backdrop-blur-sm z-[9999] flex items-center justify-center p-4"
            style={{ margin: 0 }}
            onClick={() => setShowEditModal(false)}
          >
            <div
              className="relative bg-gradient-to-br from-gray-950 via-black to-gray-900/50 rounded-2xl p-8 max-w-5xl w-full border border-gray-800 max-h-[90vh] overflow-y-auto custom-scrollbar"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="absolute inset-0 bg-gradient-to-br from-primary-500/5 via-transparent to-transparent rounded-2xl pointer-events-none" />
              <div className="relative z-10">
                <h2 className="text-2xl font-bold text-white mb-6">Edit Bot Configuration</h2>
                <form onSubmit={handleEditSubmit} className="space-y-4">
                  {/* Grid Layout: Left column for basic info, Right column for advanced config */}
                  <div className="grid gap-6 grid-cols-1 lg:grid-cols-2">
                    {/* LEFT COLUMN - Basic Configuration */}
                    <div className="space-y-4">
                      {/* Bot Name */}
                      <div>
                        <label className="block text-sm font-medium text-gray-300 mb-2">Bot Name</label>
                        <input
                          type="text"
                          value={editFormData.name}
                          onChange={(e) => setEditFormData({ ...editFormData, name: e.target.value })}
                          className="w-full px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg text-white focus:outline-none focus:border-primary-500"
                          placeholder="My Trading Bot"
                          required
                        />
                      </div>

                      {/* Bot Type - Read Only */}
                      <div>
                        <label className="block text-sm font-medium text-gray-300 mb-2">Bot Type</label>
                        <div className="px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-gray-400">
                          {editFormData.type === 'market_maker' ? 'Market Maker' : 'Pump Hunter'}
                        </div>
                      </div>

                      {/* Trading Pair - Only for Market Maker */}
                      {editFormData.type === 'market_maker' && (
                        <div>
                          <label className="block text-sm font-medium text-gray-300 mb-2">Trading Pair</label>
                          <input
                            type="text"
                            value={editFormData.pair}
                            onChange={(e) => setEditFormData({ ...editFormData, pair: e.target.value.toLowerCase() })}
                            className="w-full px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg text-white focus:outline-none focus:border-primary-500"
                            placeholder="btcidr"
                            required
                          />
                        </div>
                      )}

                      {/* Pair Filters - Only for Pump Hunter */}
                      {editFormData.type === 'pump_hunter' && (
                        <div>
                          <label className="block text-sm font-medium text-gray-300 mb-2">
                            Excluded Pairs (Optional)
                          </label>
                          <input
                            type="text"
                            placeholder="e.g., usdtidr, usdcidr"
                            value={editFormData.entry_rules?.excluded_pairs?.join(', ') || ''}
                            onChange={(e) => {
                              const pairs = e.target.value
                                .split(',')
                                .map((p) => p.trim().toLowerCase())
                                .filter((p) => p.length > 0);
                              setEditFormData({
                                ...editFormData,
                                entry_rules: {
                                  ...editFormData.entry_rules!,
                                  excluded_pairs: pairs,
                                },
                              });
                            }}
                            className="w-full px-4 py-2 bg-gray-900 border border-gray-700 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-primary-500"
                          />
                        </div>
                      )}

                      {/* Trading Mode */}
                      <div>
                        <label className="block text-sm font-medium text-gray-300 mb-2">Trading Mode</label>
                        <div className="flex gap-2">
                          <button
                            type="button"
                            onClick={() => setEditFormData({ ...editFormData, is_paper_trading: true })}
                            className={cn(
                              'flex-1 px-4 py-2 rounded-lg font-medium transition-all',
                              editFormData.is_paper_trading
                                ? 'bg-primary-600 text-white'
                                : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
                            )}
                          >
                            Paper
                          </button>
                          <button
                            type="button"
                            onClick={() => setEditFormData({ ...editFormData, is_paper_trading: false })}
                            disabled={!apiKey}
                            className={cn(
                              'flex-1 px-4 py-2 rounded-lg font-medium transition-all',
                              !editFormData.is_paper_trading
                                ? 'bg-primary-600 text-white'
                                : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700',
                              !apiKey && 'opacity-50 cursor-not-allowed'
                            )}
                            title={!apiKey ? 'Configure API key first' : ''}
                          >
                            Live
                          </button>
                        </div>
                      </div>
                    </div>
                    {/* END LEFT COLUMN */}

                    {/* RIGHT COLUMN - Market Maker Configuration */}
                    {editFormData.type === 'market_maker' && (
                      <div className="space-y-3">
                        {/* Trading Parameters - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('entry')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Trading Parameters</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'entry' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'entry' && (
                            <div className="p-4 space-y-3">
                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Initial Balance (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={editFormData.initial_balance_idr || 1000000}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    initial_balance_idr: parseFloat(e.target.value),
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Order Size (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={editFormData.order_size_idr || 100000}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    order_size_idr: parseFloat(e.target.value),
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div className="grid grid-cols-2 gap-3">
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Min Gap %</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={editFormData.min_gap_percent || 0.5}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      min_gap_percent: parseFloat(e.target.value),
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>

                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Reposition %</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={editFormData.reposition_threshold_percent || 0.1}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      reposition_threshold_percent: parseFloat(e.target.value),
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Max Loss (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={editFormData.max_loss_idr || 500000}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    max_loss_idr: parseFloat(e.target.value),
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    )}

                    {/* RIGHT COLUMN - Pump Hunter Advanced Config (Collapsible) */}
                    {editFormData.type === 'pump_hunter' && (
                      <div className="space-y-3">
                        {/* Entry Rules - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('entry')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Entry Rules</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'entry' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'entry' && (
                            <div className="p-4 space-y-3">
                              <div className="grid grid-cols-2 gap-3">
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Min Pump Score</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={editFormData.entry_rules?.min_pump_score || 50}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      entry_rules: {
                                        ...editFormData.entry_rules!,
                                        min_pump_score: parseFloat(e.target.value),
                                      },
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                                
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Min Positive TF</label>
                                  <input
                                    type="number"
                                    min="1"
                                    max="4"
                                    value={editFormData.entry_rules?.min_timeframes_positive || 2}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      entry_rules: {
                                        ...editFormData.entry_rules!,
                                        min_timeframes_positive: parseInt(e.target.value),
                                      },
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Min 24h Volume (IDR)</label>
                                <input
                                  type="number"
                                  step="1000000"
                                  value={editFormData.entry_rules?.min_24h_volume_idr || 1000000000}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    entry_rules: {
                                      ...editFormData.entry_rules!,
                                      min_24h_volume_idr: parseFloat(e.target.value),
                                    },
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Min Price (IDR)</label>
                                <input
                                  type="number"
                                  step="10"
                                  value={editFormData.entry_rules?.min_price_idr || 100}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    entry_rules: {
                                      ...editFormData.entry_rules!,
                                      min_price_idr: parseFloat(e.target.value),
                                    },
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>
                            </div>
                          )}
                        </div>

                        {/* Exit Rules - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('exit')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Exit Rules</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'exit' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'exit' && (
                            <div className="p-4 space-y-3">
                              <div className="grid grid-cols-2 gap-3">
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Target Profit %</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={editFormData.exit_rules?.target_profit_percent || 3}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      exit_rules: {
                                        ...editFormData.exit_rules!,
                                        target_profit_percent: parseFloat(e.target.value),
                                      },
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                                
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Stop Loss %</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={editFormData.exit_rules?.stop_loss_percent || 1.5}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      exit_rules: {
                                        ...editFormData.exit_rules!,
                                        stop_loss_percent: parseFloat(e.target.value),
                                      },
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                              </div>

                              <div className="flex items-center gap-2">
                                <input
                                  type="checkbox"
                                  id="edit-trailing-stop"
                                  checked={editFormData.exit_rules?.trailing_stop_enabled || false}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    exit_rules: {
                                      ...editFormData.exit_rules!,
                                      trailing_stop_enabled: e.target.checked,
                                    },
                                  })}
                                  className="w-4 h-4 bg-gray-900 border-gray-700 rounded focus:ring-primary-500"
                                />
                                <label htmlFor="edit-trailing-stop" className="text-sm text-gray-300">Enable Trailing Stop</label>
                              </div>

                              {editFormData.exit_rules?.trailing_stop_enabled && (
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Trailing Stop %</label>
                                  <input
                                    type="number"
                                    step="0.1"
                                    value={editFormData.exit_rules?.trailing_stop_percent || 1}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      exit_rules: {
                                        ...editFormData.exit_rules!,
                                        trailing_stop_percent: parseFloat(e.target.value),
                                      },
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                              )}

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Max Hold Time (minutes)</label>
                                <input
                                  type="number"
                                  step="1"
                                  value={editFormData.exit_rules?.max_hold_minutes || 30}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    exit_rules: {
                                      ...editFormData.exit_rules!,
                                      max_hold_minutes: parseInt(e.target.value),
                                    },
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div className="flex items-center gap-2">
                                <input
                                  type="checkbox"
                                  id="edit-pump-score-exit"
                                  checked={editFormData.exit_rules?.exit_on_pump_score_drop || false}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    exit_rules: {
                                      ...editFormData.exit_rules!,
                                      exit_on_pump_score_drop: e.target.checked,
                                    },
                                  })}
                                  className="w-4 h-4 bg-gray-900 border-gray-700 rounded focus:ring-primary-500"
                                />
                                <label htmlFor="edit-pump-score-exit" className="text-sm text-gray-300">Exit on Pump Score Drop</label>
                              </div>

                              {editFormData.exit_rules?.exit_on_pump_score_drop && (
                                <div>
                                  <label className="block text-xs text-gray-400 mb-1">Pump Score Drop Threshold</label>
                                  <input
                                    type="number"
                                    step="1"
                                    value={editFormData.exit_rules?.pump_score_drop_threshold || 20}
                                    onChange={(e) => setEditFormData({
                                      ...editFormData,
                                      exit_rules: {
                                        ...editFormData.exit_rules!,
                                        pump_score_drop_threshold: parseFloat(e.target.value),
                                      },
                                    })}
                                    className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                  />
                                </div>
                              )}
                            </div>
                          )}
                        </div>

                        {/* Risk Management - Collapsible */}
                        <div className="border border-gray-700 rounded-lg overflow-hidden">
                          <button
                            type="button"
                            onClick={() => toggleSection('risk')}
                            className="w-full flex items-center justify-between p-4 bg-gray-800/50 hover:bg-gray-800 transition-colors"
                          >
                            <h4 className="text-sm font-semibold text-white">Risk Management</h4>
                            <svg 
                              className={cn("w-5 h-5 text-gray-400 transition-transform", expandedSection === 'risk' && "rotate-180")}
                              fill="none" 
                              stroke="currentColor" 
                              viewBox="0 0 24 24"
                            >
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                            </svg>
                          </button>
                          
                          {expandedSection === 'risk' && (
                            <div className="p-4 space-y-3">
                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Max Position Size (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={editFormData.risk_management?.max_position_idr || 500000}
                                  onChange={(e) => {
                                    const value = e.target.value === '' ? 0 : parseFloat(e.target.value);
                                    setEditFormData({
                                      ...editFormData,
                                      risk_management: {
                                        ...editFormData.risk_management!,
                                        max_position_idr: isNaN(value) ? 0 : value,
                                      },
                                    });
                                  }}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Max Concurrent Positions</label>
                                <input
                                  type="number"
                                  min="1"
                                  max="10"
                                  value={editFormData.risk_management?.max_concurrent_positions || 3}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    risk_management: {
                                      ...editFormData.risk_management!,
                                      max_concurrent_positions: parseInt(e.target.value),
                                    },
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Daily Loss Limit (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={editFormData.risk_management?.daily_loss_limit_idr || 1000000}
                                  onChange={(e) => {
                                    const value = e.target.value === '' ? 0 : parseFloat(e.target.value);
                                    setEditFormData({
                                      ...editFormData,
                                      risk_management: {
                                        ...editFormData.risk_management!,
                                        daily_loss_limit_idr: isNaN(value) ? 0 : value,
                                      },
                                    });
                                  }}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Cooldown After Loss (minutes)</label>
                                <input
                                  type="number"
                                  step="1"
                                  value={editFormData.risk_management?.cooldown_after_loss_minutes || 10}
                                  onChange={(e) => setEditFormData({
                                    ...editFormData,
                                    risk_management: {
                                      ...editFormData.risk_management!,
                                      cooldown_after_loss_minutes: parseInt(e.target.value),
                                    },
                                  })}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>

                              <div>
                                <label className="block text-xs text-gray-400 mb-1">Min Balance (IDR)</label>
                                <input
                                  type="number"
                                  min="10000"
                                  step="10000"
                                  value={editFormData.risk_management?.min_balance_idr || 100000}
                                  onChange={(e) => {
                                    const value = e.target.value === '' ? 0 : parseFloat(e.target.value);
                                    setEditFormData({
                                      ...editFormData,
                                      risk_management: {
                                        ...editFormData.risk_management!,
                                        min_balance_idr: isNaN(value) ? 0 : value,
                                      },
                                    });
                                  }}
                                  className="w-full px-3 py-1.5 bg-gray-900 border border-gray-700 rounded text-white text-sm focus:outline-none focus:border-primary-500"
                                />
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                  {/* END GRID */}

                  {/* Actions */}
                  <div className="flex gap-3 pt-4">
                    <button
                      type="button"
                      onClick={() => setShowEditModal(false)}
                      className="flex-1 px-4 py-2 bg-gray-800 hover:bg-gray-700 text-white rounded-lg font-medium transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      type="submit"
                      disabled={updateBotMutation.isPending}
                      className="flex-1 px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-gray-700 disabled:cursor-not-allowed text-white rounded-lg font-medium transition-colors"
                    >
                      {updateBotMutation.isPending ? 'Saving...' : 'Save Changes'}
                    </button>
                  </div>
                </form>
              </div>
            </div>
          </div>
        )}

        {/* Alert Modal */}
        <AlertModal
          open={alertModal.open}
          onClose={() => setAlertModal({ ...alertModal, open: false })}
          title={alertModal.title}
          message={alertModal.message}
          type={alertModal.type}
        />
      </div>
    </div>
  );
}
