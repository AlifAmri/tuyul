package indodax

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// PrivateWSURL is the Private WebSocket URL for production
	PrivateWSURL = "wss://pws.indodax.com/ws/?cf_ws_frame_ping_pong=true"
)

// PrivateWSClient represents Indodax Private WebSocket client
type PrivateWSClient struct {
	restClient *Client
	apiKey     string
	apiSecret  string

	conn *websocket.Conn
	mu   sync.Mutex

	// User private channel ID obtained from token generation
	userChannel string

	onOrderUpdate func(order *OrderUpdate)
	onError       func(err error)

	done      chan struct{}
	writeChan chan interface{}

	isConnected     bool
	authFailed      bool // Track if authentication failed (don't reconnect)
	authFailedMu    sync.RWMutex

	// Channels for subscription verification
	authConfirmed chan bool // true = success, false = failed
	subConfirmed  chan bool // true = success, false = failed
	authSubMu      sync.Mutex
	
	// Message ID counter (simple sequential IDs as per Indodax docs)
	currentID int64
	idMu      sync.Mutex
}

// OrderUpdate represents an order update event
type OrderUpdate struct {
	OrderID         string `json:"orderId"`
	TradeID         string `json:"tradeId"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	OrigQty         string `json:"origQty"`
	UnfilledQty     string `json:"unfilledQty"`
	ExecutedQty     string `json:"executedQty"`
	Price           string `json:"price"`
	Status          string `json:"status"`
	ClientOrderID   string `json:"clientOrderId"`
	TransactionTime int64  `json:"transactionTime"`
}

// NewPrivateWSClient creates a new Private WebSocket client
func NewPrivateWSClient(restClient *Client, apiKey, apiSecret string) *PrivateWSClient {
	return &PrivateWSClient{
		restClient:    restClient,
		apiKey:        apiKey,
		apiSecret:     apiSecret,
		done:          make(chan struct{}),
		writeChan:     make(chan interface{}, 100),
		authConfirmed: make(chan bool, 1),
		subConfirmed:  make(chan bool, 1),
	}
}

// SetOrderUpdateHandler sets the handler for order updates
func (c *PrivateWSClient) SetOrderUpdateHandler(handler func(order *OrderUpdate)) {
	c.onOrderUpdate = handler
}

// SetErrorHandler sets the handler for errors
func (c *PrivateWSClient) SetErrorHandler(handler func(err error)) {
	c.onError = handler
}

// Connect connects to the WebSocket server
func (c *PrivateWSClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		return nil
	}

	// Reset auth failed flag when attempting to connect (user might have fixed credentials)
	c.authFailedMu.Lock()
	c.authFailed = false
	c.authFailedMu.Unlock()

	// Reset confirmation channels - create new ones to ensure clean state
	// This prevents old WaitForSubscription calls from blocking on stale channels
	c.authSubMu.Lock()
	c.authConfirmed = make(chan bool, 1)
	c.subConfirmed = make(chan bool, 1)
	c.authSubMu.Unlock()

	// 1. Generate Token
	tokenInfo, err := c.restClient.GeneratePrivateWSToken(ctx, c.apiKey, c.apiSecret)
	if err != nil {
		return fmt.Errorf("failed to generate private ws token: %w", err)
	}
	
	// Validate token and channel
	if tokenInfo.ConnToken == "" {
		return fmt.Errorf("generated token is empty")
	}
	if tokenInfo.Channel == "" {
		return fmt.Errorf("generated channel is empty")
	}
	
	c.userChannel = tokenInfo.Channel

	// 2. Connect WS
	u, err := url.Parse(PrivateWSURL)
	if err != nil {
		return fmt.Errorf("invalid websocket url: %w", err)
	}

	// Connect without Origin header (Indodax rejects connections with Origin header - 403 Forbidden)
	// According to Indodax docs, use DefaultDialer.Dial with nil headers
	// gorilla/websocket may set Origin automatically, but we pass nil to let it handle it
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	
	// Use DialContext with nil headers as per Indodax documentation example
	// Note: gorilla/websocket will automatically set Origin based on URL, but
	// Indodax should accept connections from their own domain
	conn, resp, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to connect to websocket (status %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	c.conn = conn
	c.isConnected = true

	// Start read/write pumps
	go c.readPump()
	go c.writePump()
	go c.pingPump()

	// Small delay to ensure writePump is ready
	time.Sleep(100 * time.Millisecond)

	// 3. Authenticate / Connect message
	if err := c.authenticate(tokenInfo.ConnToken); err != nil {
		c.Close()
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// 4. Subscribe to private channel immediately after auth
	// (We don't wait for auth confirmation here - it will be handled by WaitForSubscription if needed)
	c.subscribe(c.userChannel)

	return nil
}

// WaitForSubscription waits for authentication and subscription to be confirmed
// Returns error if either fails, nil if both succeed
func (c *PrivateWSClient) WaitForSubscription(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	// Get current channel references atomically
	c.authSubMu.Lock()
	authChan := c.authConfirmed
	subChan := c.subConfirmed
	c.authSubMu.Unlock()
	
	// Wait for authentication confirmation
	select {
	case success := <-authChan:
		if !success {
			return fmt.Errorf("authentication failed")
		}
	case <-time.After(time.Until(deadline)):
		return fmt.Errorf("authentication timeout after %v", timeout)
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for subscription confirmation
	select {
	case success := <-subChan:
		if !success {
			return fmt.Errorf("subscription failed")
		}
	case <-time.After(time.Until(deadline)):
		return fmt.Errorf("subscription timeout after %v", timeout)
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// Close closes the WebSocket connection
func (c *PrivateWSClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected {
		return
	}

	c.isConnected = false
	if c.conn != nil {
		c.conn.Close()
	}
}

// setAuthFailed marks authentication as failed (prevents reconnection)
func (c *PrivateWSClient) setAuthFailed() {
	c.authFailedMu.Lock()
	defer c.authFailedMu.Unlock()
	c.authFailed = true
}

// isAuthFailed checks if authentication failed
func (c *PrivateWSClient) isAuthFailed() bool {
	c.authFailedMu.RLock()
	defer c.authFailedMu.RUnlock()
	return c.authFailed
}

func (c *PrivateWSClient) nextID() int64 {
	c.idMu.Lock()
	defer c.idMu.Unlock()
	c.currentID++
	return c.currentID
}

func (c *PrivateWSClient) authenticate(token string) error {
	// Authentication message format per Indodax docs:
	// {"connect":{"token": "..."},"id": 1}
	authPayload := map[string]interface{}{
		"connect": map[string]string{
			"token": token,
		},
		"id": c.nextID(),
	}

	c.writeChan <- authPayload
	return nil
}

func (c *PrivateWSClient) subscribe(channel string) {
	// Subscription message format per Indodax docs:
	// {"subscribe":{"channel":"pws:#..."},"id":2}
	subPayload := map[string]interface{}{
		"subscribe": map[string]string{
			"channel": channel,
		},
		"id": c.nextID(),
	}

	c.writeChan <- subPayload
}

func (c *PrivateWSClient) readPump() {
	defer func() {
		// Close the connection
		c.Close()
		
		// Only reconnect if authentication didn't fail
		if c.isAuthFailed() {
			// Authentication failed - don't reconnect
			return
		}
		
		// Reconnect logic for normal disconnections (including stale connections)
		// Start reconnection in a goroutine
		go func() {
			// Small delay before attempting reconnection
		time.Sleep(1 * time.Second)
			
			// Simple infinite retry with exponential backoff
			backoff := 2 * time.Second
			maxBackoff := 30 * time.Second
			for {
				// Check if auth failed before attempting reconnect
				if c.isAuthFailed() {
					return // Stop reconnecting if auth failed
				}
				
				// Check if already connected (another goroutine might have reconnected)
				c.mu.Lock()
				alreadyConnected := c.isConnected
				c.mu.Unlock()
				if alreadyConnected {
					return // Already reconnected
				}
				
				// Attempt reconnection
				// Log reconnection attempt
				if c.onError != nil {
					c.onError(fmt.Errorf("attempting to reconnect Private WebSocket..."))
				}
				
				connectErr := c.Connect(context.Background())
				if connectErr == nil {
					// Successfully reconnected
					if c.onError != nil {
						c.onError(fmt.Errorf("Private WebSocket reconnected successfully"))
					}
					return
				}
				
				// Log reconnection failure
				if c.onError != nil {
					c.onError(fmt.Errorf("reconnection attempt failed, retrying in %v: %w", backoff, connectErr))
				}
				time.Sleep(backoff)
				
				// Exponential backoff
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				}
			}()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// Check if it's a close error
			if closeErr, ok := err.(*websocket.CloseError); ok {
				// Log close code and reason
				if c.onError != nil {
					c.onError(fmt.Errorf("websocket closed: code=%d, reason=%s", closeErr.Code, closeErr.Text))
				}
				
				// Handle different close codes
				if closeErr.Code == 3501 || closeErr.Code == 1008 {
					// 3501 = Bad request (authentication issue)
					// 1008 = Policy violation (authentication issue)
					// Mark auth as failed and stop reconnecting
					c.setAuthFailed()
					if c.onError != nil {
						c.onError(fmt.Errorf("connection rejected by server (code %d): %s. This may indicate an authentication issue. Please check your API credentials. Reconnection stopped.", closeErr.Code, closeErr.Text))
					}
					// Signal authentication failure if not already signaled
					select {
					case c.authConfirmed <- false:
					default:
					}
					select {
					case c.subConfirmed <- false:
					default:
					}
					return // Exit without triggering reconnection
				} else if closeErr.Code == 3502 {
					// 3502 = Stale connection (normal - token expired or connection idle)
					// This is not an authentication failure, just needs reconnection with new token
					// Don't mark as auth failed, allow reconnection
					if c.onError != nil {
						c.onError(fmt.Errorf("connection marked as stale (code %d): %s. Will reconnect with new token.", closeErr.Code, closeErr.Text))
					}
					// Don't signal auth failure - this is normal and will reconnect
					// Break out of read loop to trigger reconnection
					break
				}
			} else {
				// Check if connection was closed before we got confirmation
				// This can happen if server closes connection immediately
				select {
				case c.authConfirmed <- false:
				default:
				}
				select {
				case c.subConfirmed <- false:
				default:
				}
			if c.onError != nil {
				c.onError(err)
				}
			}
			break
		}

		c.handleMessage(message)
	}
}

func (c *PrivateWSClient) handleMessage(message []byte) {
	// 1. Check for push events (order updates)
	var pushMsg struct {
		Push struct {
			Pub struct {
				Data []struct {
					EventType string       `json:"eventType"`
					Order     *OrderUpdate `json:"order"`
				} `json:"data"`
			} `json:"pub"`
		} `json:"push"`
	}

	if err := json.Unmarshal(message, &pushMsg); err == nil {
		if pushMsg.Push.Pub.Data != nil {
			for _, item := range pushMsg.Push.Pub.Data {
				if item.EventType == "order_update" && item.Order != nil {
					// Log raw incoming order update data for debugging
					orderJSON, _ := json.Marshal(item.Order)
					log.Printf("[WS_ORDER_UPDATE] Raw order update from Indodax WebSocket: %s", string(orderJSON))
					
					if c.onOrderUpdate != nil {
						c.onOrderUpdate(item.Order)
					}
				}
			}
			return
		}
	}

	// 2. Check for authentication response
	var authResp struct {
		ID      int64 `json:"id"`
		Connect *struct {
			Client  string `json:"client"`
			Version string `json:"version"`
			Expires bool   `json:"expires"`
			TTL     int    `json:"ttl"`
		} `json:"connect"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(message, &authResp); err == nil {
		if authResp.Error != nil {
			// Authentication failed or token expired
			if authResp.Error.Code == 109 {
				// Token expired - signal failure for initial subscription check
				// (Later reconnections will handle this differently)
				select {
				case c.authConfirmed <- false:
				default:
				}
				if c.onError != nil {
					c.onError(fmt.Errorf("private WS token expired (code %d): %s", authResp.Error.Code, authResp.Error.Message))
				}
				// Close connection to trigger reconnection with new token
				c.Close()
			} else {
				// Other authentication error - mark as failed and stop reconnecting
				c.setAuthFailed()
				if c.onError != nil {
					c.onError(fmt.Errorf("private WS authentication error (code %d): %s. Reconnection stopped.", authResp.Error.Code, authResp.Error.Message))
				}
				// Signal authentication failure
				select {
				case c.authConfirmed <- false:
				default:
				}
				c.Close()
			}
			return
		}
		if authResp.Connect != nil {
			// Authentication successful - clear auth failed flag
			c.authFailedMu.Lock()
			c.authFailed = false
			c.authFailedMu.Unlock()
			// Signal authentication success
			select {
			case c.authConfirmed <- true:
			default:
			}
			return
		}
	}

	// 3. Check for subscription response
	// Subscription response can be either:
	// - Success: {"id": 2} (no error field)
	// - Error: {"id": 2, "error": {"code": ..., "message": ...}}
	var subResp struct {
		ID      int64 `json:"id"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Subscribe *struct {
			Status string `json:"status"`
		} `json:"subscribe"`
	}

	if err := json.Unmarshal(message, &subResp); err == nil && subResp.ID > 0 {
		if subResp.Error != nil {
			// Subscription failed (e.g., permission denied) - mark as auth failed
			c.setAuthFailed()
			if c.onError != nil {
				c.onError(fmt.Errorf("private WS subscription error (code %d): %s. Reconnection stopped.", subResp.Error.Code, subResp.Error.Message))
			}
			// Signal subscription failure
			select {
			case c.subConfirmed <- false:
			default:
			}
			// Close connection on subscription failure
			c.Close()
			return
		}
		// Subscription successful (no error field means success)
		// Signal subscription success
		select {
		case c.subConfirmed <- true:
		default:
		}
		return
	}
}

func (c *PrivateWSClient) writePump() {
	for {
		select {
		case msg := <-c.writeChan:
			c.mu.Lock()
			if !c.isConnected {
				c.mu.Unlock()
				continue
			}
			err := c.conn.WriteJSON(msg)
			c.mu.Unlock()
			if err != nil {
				if c.onError != nil {
					c.onError(err)
				}
				c.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *PrivateWSClient) pingPump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Private WS might handle ping/pong differently (via query param cf_ws_frame_ping_pong=true)
			// Docs say: wss://pws.indodax.com/ws/?cf_ws_frame_ping_pong=true
			// Usually this means server sends pings and client sends pong frames automatically by websocket lib.
			// But if app level ping is needed:
			// No explicit method 7 ping mentioned in Private WS docs, but usually safe to send standard ping frame.
			// SetPingHandler is default in gorilla/websocket to reply to Ping.

			// We can send a keepalive message if required, but docs don't specify one for Private WS.
			// For now, rely on connection level ping/pong if enabled by URL param.
		case <-c.done:
			return
		}
	}
}
