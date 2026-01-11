import { useEffect } from 'react';
import { useWebSocket } from '@/contexts/WebSocketContext';
import { useNotificationStore } from '@/stores/notificationStore';
import { useAuthStore } from '@/stores/authStore';

export function useNotifications() {
  const { lastMessage, isConnected } = useWebSocket();
  const addNotification = useNotificationStore((state) => state.addNotification);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  useEffect(() => {
    // Only process messages if user is authenticated and WebSocket is connected
    if (!isAuthenticated || !isConnected || !lastMessage) return;

    const message = lastMessage;

    switch (message.type) {
      case 'pump_signal': {
        const payload = message.payload as { pair: string; score: number };
        if (!payload?.pair || typeof payload?.score !== 'number') break;
        
        addNotification({
          type: 'success',
          title: 'Pump Signal Detected',
          message: `${payload.pair} is pumping with score ${payload.score.toFixed(2)}`,
        });
        break;
      }

      case 'stop_loss_triggered': {
        const payload = message.payload as { pair: string; exit_price: string };
        if (!payload?.pair || !payload?.exit_price) break;
        
        addNotification({
          type: 'error',
          title: 'Stop-Loss Triggered',
          message: `${payload.pair} hit stop-loss at ${payload.exit_price}`,
        });
        break;
      }

      case 'order_update': {
        // Don't show toast - let BotsManagementPage handle the update silently
        // The page will invalidate queries and update the UI automatically
        break;
      }

      case 'bot_status': {
        const payload = message.payload as { bot_id: number; status: string };
        if (!payload?.bot_id || !payload?.status) break;
        
        if (payload.status === 'running') {
          addNotification({
            type: 'success',
            title: 'Bot Started',
            message: `Bot #${payload.bot_id} is now running`,
          });
        } else if (payload.status === 'stopped') {
          addNotification({
            type: 'info',
            title: 'Bot Stopped',
            message: `Bot #${payload.bot_id} has been stopped`,
          });
        } else if (payload.status === 'error') {
          addNotification({
            type: 'error',
            title: 'Bot Error',
            message: `Bot #${payload.bot_id} encountered an error`,
          });
        }
        break;
      }

      case 'bot_update': {
        // Don't show toast - let BotsManagementPage handle the update silently
        // The page will invalidate queries and update the UI automatically
        break;
      }

      case 'position_update': {
        const payload = message.payload as { status: string; pair: string; profit_idr?: number };
        if (!payload?.status || !payload?.pair) break;
        
        if (payload.status === 'closed') {
          const profit = payload.profit_idr || 0;
          if (profit > 0) {
            addNotification({
              type: 'success',
              title: 'Position Closed (Profit)',
              message: `${payload.pair}: +Rp ${profit.toLocaleString()}`,
            });
          } else {
            addNotification({
              type: 'error',
              title: 'Position Closed (Loss)',
              message: `${payload.pair}: Rp ${profit.toLocaleString()}`,
            });
          }
        }
        break;
      }

      case 'bot_pnl_update': {
        const payload = message.payload as { bot_id: number; profit: number };
        if (!payload?.bot_id || typeof payload?.profit !== 'number') break;
        
        // Only show if significant profit/loss
        if (Math.abs(payload.profit) > 100000) {
          const profit = payload.profit;
          if (profit > 0) {
            addNotification({
              type: 'success',
              title: 'Bot Profit Update',
              message: `Bot #${payload.bot_id}: +Rp ${profit.toLocaleString()}`,
            });
          } else {
            addNotification({
              type: 'error',
              title: 'Bot Loss Update',
              message: `Bot #${payload.bot_id}: Rp ${profit.toLocaleString()}`,
            });
          }
        }
        break;
      }

      default:
        // Ignore other message types
        break;
    }
  }, [lastMessage]);
}

