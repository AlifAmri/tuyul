package indodax

import (
	"context"
	"encoding/json"
	"fmt"
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

	isConnected bool
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
		restClient: restClient,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		done:       make(chan struct{}),
		writeChan:  make(chan interface{}, 100),
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

	// 1. Generate Token
	tokenInfo, err := c.restClient.GeneratePrivateWSToken(ctx, c.apiKey, c.apiSecret)
	if err != nil {
		return fmt.Errorf("failed to generate private ws token: %w", err)
	}
	c.userChannel = tokenInfo.Channel

	// 2. Connect WS
	u, err := url.Parse(PrivateWSURL)
	if err != nil {
		return fmt.Errorf("invalid websocket url: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	c.conn = conn
	c.isConnected = true

	// Start read/write pumps
	go c.readPump()
	go c.writePump()
	go c.pingPump()

	// 3. Authenticate / Connect message
	if err := c.authenticate(tokenInfo.ConnToken); err != nil {
		c.Close()
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// 4. Subscribe to private channel
	c.subscribe(c.userChannel)

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

func (c *PrivateWSClient) authenticate(token string) error {
	// The struct WSMessage in ws_client.go has Params as interface{}.
	// Let's create a specific map for this structure

	authPayload := map[string]interface{}{
		"connect": map[string]string{
			"token": token,
		},
		"id": time.Now().UnixNano(),
	}

	c.writeChan <- authPayload
	return nil
}

func (c *PrivateWSClient) subscribe(channel string) {
	subPayload := map[string]interface{}{
		"subscribe": map[string]string{
			"channel": channel,
		},
		"id": time.Now().UnixNano(),
	}

	c.writeChan <- subPayload
}

func (c *PrivateWSClient) readPump() {
	defer func() {
		c.Close()
		// Reconnect logic
		time.Sleep(1 * time.Second)
		if !c.isConnected {
			// Need context for Connect, creating background
			// This might block if Connect fails repeatedly, should probably run in goroutine or have smarter backoff
			go func() {
				// Simple infinite retry
				for {
					if err := c.Connect(context.Background()); err == nil {
						break
					}
					time.Sleep(5 * time.Second)
				}
			}()
		}
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if c.onError != nil {
				c.onError(err)
			}
			break
		}

		c.handleMessage(message)
	}
}

func (c *PrivateWSClient) handleMessage(message []byte) {
	// 1. Parse generic to check for "push" event
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
					if c.onOrderUpdate != nil {
						c.onOrderUpdate(item.Order)
					}
				}
			}
			return
		}
	}

	// Maybe check for auth success or error response here if needed
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
