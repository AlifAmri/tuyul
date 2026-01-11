import { createContext, useContext, useEffect, useRef, useState, useCallback, ReactNode } from 'react';
import { WebSocketMessage, MarketUpdateMessage } from '@/types/websocket';
import { Coin } from '@/types/market';
import { useMarketStore } from '@/stores/marketStore';
import { useAuthStore } from '@/stores/authStore';

const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws';

interface WebSocketContextValue {
  isConnected: boolean;
  isConnecting: boolean;
  lastMessage: WebSocketMessage | null;
  send: (message: WebSocketMessage) => void;
  subscribe: (channel: string) => void;
  reconnect: () => void;
  disconnect: () => void;
}

const WebSocketContext = createContext<WebSocketContextValue | null>(null);

interface WebSocketProviderProps {
  children: ReactNode;
}

export function WebSocketProvider({ children }: WebSocketProviderProps) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const isMountedRef = useRef(true);
  const connectingRef = useRef(false); // Track if we're currently attempting to connect
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [lastMessage, setLastMessage] = useState<WebSocketMessage | null>(null);
  const { updateCoin, updateCoins } = useMarketStore();
  const { isAuthenticated, isAuthChecking } = useAuthStore();

  const connect = useCallback(() => {
    // Prevent multiple simultaneous connection attempts
    if (connectingRef.current) {
      console.log('[WebSocket] Connection attempt already in progress');
      return;
    }

    // Wait for auth check to complete
    if (isAuthChecking) {
      console.log('[WebSocket] Waiting for auth check to complete...');
      return;
    }

    // Only connect if authenticated
    if (!isAuthenticated) {
      console.log('[WebSocket] User not authenticated - skipping connection');
      return;
    }

    const token = localStorage.getItem('tuyul_access_token');
    if (!token) {
      console.warn('[WebSocket] No access token found - skipping connection');
      return;
    }

    // Don't connect if already connected or connecting
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      console.log('[WebSocket] Already connected');
      return;
    }

    if (wsRef.current?.readyState === WebSocket.CONNECTING) {
      console.log('[WebSocket] Already connecting');
      return;
    }

    // Mark that we're attempting to connect
    connectingRef.current = true;

    // Clean up any existing connection (but only if it's open or closed, not connecting)
    if (wsRef.current) {
      const currentState = wsRef.current.readyState;
      if (currentState === WebSocket.OPEN || currentState === WebSocket.CLOSED) {
        try {
          wsRef.current.close(1000, 'Reconnecting');
        } catch (e) {
          // Ignore errors when closing
        }
      }
      wsRef.current = null;
    }

    setIsConnecting(true);
    const wsUrl = `${WS_URL}?token=${token}`;
    console.log('[WebSocket] Connecting to:', WS_URL);

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log('[WebSocket] Connected');
      connectingRef.current = false; // Clear connection flag
      setIsConnected(true);
      setIsConnecting(false);
      reconnectAttemptsRef.current = 0;
    };

    ws.onmessage = (event) => {
      try {
        // Handle potential newline-delimited JSON or multiple messages
        const rawData = event.data;
        if (!rawData || typeof rawData !== 'string') {
          console.warn('[WebSocket] Received non-string data:', rawData);
          return;
        }

        // Trim whitespace and split by newlines if multiple messages
        const trimmedData = rawData.trim();
        const messages = trimmedData.split('\n').filter(line => line.trim().length > 0);

        // Process each message
        for (const messageStr of messages) {
          try {
            const message = JSON.parse(messageStr) as WebSocketMessage;
            setLastMessage(message);
            
            // Handle market updates
            if (message.type === 'market_update' && message.payload) {
              const marketMessage = message as MarketUpdateMessage;
              const payload = marketMessage.payload;
              
              if (Array.isArray(payload)) {
                // Batched update (array of coins)
                updateCoins(payload as Coin[]);
              } else {
                // Single coin update (backward compatibility)
                updateCoin(payload as Coin);
              }
            }
          } catch (parseError) {
            // Log the problematic message for debugging
            console.error('[WebSocket] Failed to parse message:', parseError);
            console.error('[WebSocket] Raw message data:', messageStr.substring(0, 500)); // First 500 chars
            // Continue processing other messages if there are multiple
          }
        }
      } catch (error) {
        console.error('[WebSocket] Failed to process message event:', error);
        console.error('[WebSocket] Raw event data:', event.data?.substring?.(0, 500)); // First 500 chars
      }
    };

    ws.onerror = (error) => {
      connectingRef.current = false; // Clear connection flag on error
      console.error('[WebSocket] Error:', error);
      // Log more details if available
      if (ws.readyState === WebSocket.CLOSED) {
        console.error('[WebSocket] Connection closed unexpectedly');
      }
      setIsConnecting(false);
    };

    ws.onclose = (event) => {
      connectingRef.current = false; // Clear connection flag
      const closeReason = event.reason || 'No reason provided';
      console.log(`[WebSocket] Disconnected: code=${event.code}, reason="${closeReason}", wasClean=${event.wasClean}`);
      setIsConnected(false);
      setIsConnecting(false);
      wsRef.current = null;

      // Don't reconnect on certain close codes (authentication errors, etc.)
      if (event.code === 1008 || event.code === 4001 || event.code === 4003) {
        // 1008 = Policy violation (authentication issue)
        // 4001 = Unauthorized
        // 4003 = Forbidden
        console.error('[WebSocket] Authentication error - not reconnecting. Please refresh the page.');
        return;
      }

      // Auto reconnect for other errors
      if (reconnectAttemptsRef.current < 10) {
        reconnectAttemptsRef.current += 1;
        const delay = 3000 * Math.min(reconnectAttemptsRef.current, 5); // Exponential backoff
        console.log(`[WebSocket] Reconnecting in ${delay}ms (attempt ${reconnectAttemptsRef.current}/10)`);
        
        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, delay);
      } else {
        console.error('[WebSocket] Max reconnect attempts reached');
      }
    };
  }, [updateCoin, updateCoins, isAuthenticated, isAuthChecking]);

  const disconnect = useCallback(() => {
    connectingRef.current = false; // Clear connection flag
    
    // Clear any pending reconnection attempts
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    // Safely close WebSocket connection
    if (wsRef.current) {
      const ws = wsRef.current;
      const readyState = ws.readyState;
      
      // Only close if not already closed or closing
      if (readyState === WebSocket.CONNECTING || readyState === WebSocket.OPEN) {
        try {
          // Close with a normal closure code
          ws.close(1000, 'Client disconnecting');
        } catch (error) {
          // Ignore errors during cleanup (e.g., already closed)
          // This can happen in React StrictMode when unmounting during connection
          // Suppress this error in development as it's expected behavior
          if (import.meta.env.MODE === 'development') {
            // Silently ignore in dev mode (React StrictMode)
          } else {
            console.warn('[WebSocket] Error during disconnect cleanup:', error);
          }
        }
      }
      wsRef.current = null;
    }

    setIsConnected(false);
    setIsConnecting(false);
  }, []);

  const send = useCallback((message: WebSocketMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(message));
    } else {
      console.warn('[WebSocket] Cannot send message, not connected');
    }
  }, []);

  const subscribe = useCallback((channel: string) => {
    send({
      type: 'subscribed',
      payload: { channel },
    });
  }, [send]);

  // Store latest connect/disconnect in refs to avoid dependency issues
  const connectRef = useRef(connect);
  const disconnectRef = useRef(disconnect);
  
  useEffect(() => {
    connectRef.current = connect;
    disconnectRef.current = disconnect;
  }, [connect, disconnect]);

  useEffect(() => {
    isMountedRef.current = true;

    // Only attempt connection when auth is ready and user is authenticated
    if (!isAuthChecking && isAuthenticated) {
      // Small delay to prevent React StrictMode double-invocation issues
      const timeoutId = setTimeout(() => {
        if (isMountedRef.current && !connectingRef.current && !wsRef.current) {
          connectRef.current();
        }
      }, 100);

      return () => {
        clearTimeout(timeoutId);
        isMountedRef.current = false;
        // Only disconnect if we actually have a connection
        if (wsRef.current) {
          disconnectRef.current();
        }
      };
    } else if (!isAuthChecking && !isAuthenticated) {
      // If auth check is complete and user is not authenticated, disconnect
      disconnectRef.current();
      return () => {
        isMountedRef.current = false;
      };
    }

    return () => {
      isMountedRef.current = false;
      // Only disconnect if we actually have a connection
      if (wsRef.current) {
        disconnectRef.current();
      }
    };
  }, [isAuthChecking, isAuthenticated]); // Only depend on auth state

  const value: WebSocketContextValue = {
    isConnected,
    isConnecting,
    lastMessage,
    send,
    subscribe,
    reconnect: connect,
    disconnect,
  };

  return (
    <WebSocketContext.Provider value={value}>
      {children}
    </WebSocketContext.Provider>
  );
}

export function useWebSocket() {
  const context = useContext(WebSocketContext);
  if (!context) {
    throw new Error('useWebSocket must be used within WebSocketProvider');
  }
  return context;
}

