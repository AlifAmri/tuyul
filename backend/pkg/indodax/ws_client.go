package indodax

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tuyul/backend/pkg/logger"

	"github.com/gorilla/websocket"
)

const (
	// WSURLProduction is the WebSocket URL for production
	WSURLProduction = "wss://ws3.indodax.com/ws/"
	// WSURLDemo is the WebSocket URL for demo
	WSURLDemo = "wss://ws.demo-indodax.com/ws/"

	// DefaultStaticTokenProduction is the static token for production (from docs, should be in config/env in real app but putting here as default)
	DefaultStaticTokenProduction = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE5NDY2MTg0MTV9.UR1lBM6Eqh0yWz-PVirw1uPCxe60FdchR8eNVdsskeo"
)

// WSMessage represents a generic WebSocket message
type WSMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method int             `json:"method,omitempty"`
	Params interface{}     `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// WSClient represents Indodax WebSocket client
type WSClient struct {
	url           string
	token         string
	conn          *websocket.Conn
	mu            sync.Mutex
	subscriptions map[string]bool
	handlers      []func(channel string, data []byte)
	errHandlers   []func(err error)

	done      chan struct{}
	writeChan chan interface{}

	isConnected     bool
	isReconnecting  bool
	reconnectMu      sync.Mutex
	msgID            int64
	maxReconnectAttempts int
	reconnectDelay   time.Duration
}

// NewWSClient creates a new WebSocket client
func NewWSClient(wsURL, token string) *WSClient {
	if wsURL == "" {
		wsURL = WSURLProduction
	}
	if token == "" {
		token = DefaultStaticTokenProduction
	}

	return &WSClient{
		url:                 wsURL,
		token:               token,
		subscriptions:       make(map[string]bool),
		handlers:            make([]func(channel string, data []byte), 0),
		errHandlers:         make([]func(err error), 0),
		done:                make(chan struct{}),
		writeChan:           make(chan interface{}, 100),
		maxReconnectAttempts: 10, // Max attempts before giving up
		reconnectDelay:      2 * time.Second, // Initial delay
	}
}

func (c *WSClient) nextID() int64 {
	return atomic.AddInt64(&c.msgID, 1)
}

// AddMessageHandler adds a handler for incoming messages
func (c *WSClient) AddMessageHandler(handler func(channel string, data []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, handler)
}

// SetMessageHandler sets the handler for incoming messages (clears others for compatibility)
func (c *WSClient) SetMessageHandler(handler func(channel string, data []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = []func(channel string, data []byte){handler}
}

// AddErrorHandler adds a handler for errors
func (c *WSClient) AddErrorHandler(handler func(err error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errHandlers = append(c.errHandlers, handler)
}

// SetErrorHandler sets the handler for errors (clears others for compatibility)
func (c *WSClient) SetErrorHandler(handler func(err error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errHandlers = []func(err error){handler}
}

// Connect connects to the WebSocket server
func (c *WSClient) Connect() error {
	c.mu.Lock()
	
	// If already connected, return immediately
	if c.isConnected {
		c.mu.Unlock()
		return nil
	}

	// Close existing connection if any (cleanup)
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	
	c.mu.Unlock()

	u, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("invalid websocket url: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	c.mu.Lock()
	// Double-check we're still not connected (race condition protection)
	if c.isConnected {
		c.mu.Unlock()
		conn.Close()
		return nil
	}

	c.conn = conn
	c.isConnected = true
	c.mu.Unlock()

	// Start read/write pumps
	go c.readPump()
	go c.writePump()
	go c.pingPump()

	// Authenticate
	if err := c.authenticate(); err != nil {
		c.Close()
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Resubscribe if needed
	c.resubscribe()

	logger.Infof("Successfully connected to Indodax WebSocket at %s", c.url)
	return nil
}

// Close closes the WebSocket connection
func (c *WSClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isConnected {
		return
	}

	c.isConnected = false
	if c.conn != nil {
		c.conn.Close()
	}
	// Do not close channels here to avoid panic on send if multiple goroutines call Close
}

// Subscribe subscribes to a channel
func (c *WSClient) Subscribe(channel string) {
	c.mu.Lock()
	c.subscriptions[channel] = true
	connected := c.isConnected
	c.mu.Unlock()

	if !connected {
		return
	}

	c.writeChan <- WSMessage{
		ID:     c.nextID(),
		Method: 1,
		Params: map[string]string{
			"channel": channel,
		},
	}
	logger.Infof("Subscribed to channel: %s", channel)
}

// Unsubscribe unsubscribes from a channel
func (c *WSClient) Unsubscribe(channel string) {
	c.mu.Lock()
	delete(c.subscriptions, channel)
	connected := c.isConnected
	c.mu.Unlock()

	if !connected {
		return
	}

	c.writeChan <- WSMessage{
		ID:     c.nextID(),
		Method: 2,
		Params: map[string]string{
			"channel": channel,
		},
	}
	logger.Infof("Unsubscribed from channel: %s", channel)
}

func (c *WSClient) authenticate() error {
	// Send auth message immediately via conn (bypass writePump which is just starting)
	msg := WSMessage{
		ID:     c.nextID(),
		Params: map[string]string{"token": c.token},
	}

	// We use writeChan for consistency, assuming writePump starts quickly
	c.writeChan <- msg
	logger.Infof("Sent authentication token to Indodax WebSocket")
	return nil
}

func (c *WSClient) resubscribe() {
	for channel := range c.subscriptions {
		c.Subscribe(channel)
	}
}

func (c *WSClient) readPump() {
	logger.Infof("Entering readPump loop for %s", c.url)
	defer func() {
		logger.Infof("Exiting readPump loop for %s", c.url)
		c.Close()
		// Trigger reconnection
		c.reconnect()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// Check if it's a close error
			if closeErr, ok := err.(*websocket.CloseError); ok {
				logger.Warnf("WS connection closed for %s: code=%d, reason=%s", c.url, closeErr.Code, closeErr.Text)
				// Check if it's a "stale" connection (code 3007)
				if closeErr.Code == 3007 {
					logger.Infof("Connection marked as stale, will reconnect")
				}
			} else {
				logger.Errorf("WS ReadMessage error for %s: %v", c.url, err)
			}
			
			c.mu.Lock()
			handlers := c.errHandlers
			c.mu.Unlock()
			for _, h := range handlers {
				h(err)
			}
			break
		}

		c.handleMessage(message)
	}
}

func (c *WSClient) handleMessage(message []byte) {
	var direct struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(message, &direct); err == nil && direct.Channel != "" {
		c.mu.Lock()
		handlers := c.handlers
		c.mu.Unlock()
		for _, h := range handlers {
			h(direct.Channel, direct.Data)
		}
		return
	}

	// Try nested result format (common for method responses)
	var nested struct {
		ID     int64           `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(message, &nested); err == nil {
		// Handle ping/pong response (just {"id":3} with no result)
		if nested.Result == nil && nested.ID > 0 {
			// Silent ping/pong - no logging needed
			return
		}
		
		if nested.Result == nil {
			return
		}
		// Check if this is an authentication response (has client, version)
		var authResponse struct {
			Client  string `json:"client"`
			Version string `json:"version"`
			Expires bool   `json:"expires"`
			TTL     int64  `json:"ttl"`
		}
		if err := json.Unmarshal(nested.Result, &authResponse); err == nil && authResponse.Client != "" {
			logger.Infof("WSClient: Authentication confirmed (id=%d, client=%s, version=%s, ttl=%d)", 
				nested.ID, authResponse.Client, authResponse.Version, authResponse.TTL)
			return
		}
		
		// Check if this is a subscription response (has recoverable, epoch, offset)
		var subResponse struct {
			Recoverable bool   `json:"recoverable"`
			Epoch       string `json:"epoch"`
			Offset      int64  `json:"offset"`
		}
		if err := json.Unmarshal(nested.Result, &subResponse); err == nil && subResponse.Recoverable {
			logger.Infof("WSClient: Subscription confirmed (id=%d, recoverable=%v, epoch=%s, offset=%d)", 
				nested.ID, subResponse.Recoverable, subResponse.Epoch, subResponse.Offset)
			return
		}
		
		// Check if this is a channel data message
		var resultData struct {
			Channel string          `json:"channel"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(nested.Result, &resultData); err == nil && resultData.Channel != "" {
			// Only log order-book messages, skip market:summary-24h
			if strings.HasPrefix(resultData.Channel, "market:order-book-") {
				logger.Infof("WSClient: Received order-book message - channel=%s, data length=%d", resultData.Channel, len(resultData.Data))
			}
			c.mu.Lock()
			handlers := c.handlers
			c.mu.Unlock()
			for _, h := range handlers {
				h(resultData.Channel, resultData.Data)
			}
			return
		}
	}
}

func (c *WSClient) writePump() {
	defer func() {
		logger.Infof("Exiting writePump loop for %s", c.url)
		c.Close()
		// Trigger reconnection
		c.reconnect()
	}()

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
				logger.Errorf("WS WriteJSON error for %s: %v", c.url, err)
				c.mu.Lock()
				handlers := c.errHandlers
				c.mu.Unlock()
				for _, h := range handlers {
					h(err)
				}
				c.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *WSClient) pingPump() {
	ticker := time.NewTicker(30 * time.Second) // Ping every 30s
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			connected := c.isConnected
			c.mu.Unlock()
			if !connected {
				return
			}
			c.writeChan <- WSMessage{
				ID:     c.nextID(),
				Method: 7, // Ping method
			}
		case <-c.done:
			return
		}
	}
}

// reconnect attempts to reconnect with exponential backoff
func (c *WSClient) reconnect() {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	// Check if already reconnecting
	if c.isReconnecting {
		return
	}

	// Check if already connected
	c.mu.Lock()
	if c.isConnected {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	c.isReconnecting = true
	logger.Infof("Starting reconnection process for %s", c.url)

	go func() {
		defer func() {
			c.reconnectMu.Lock()
			c.isReconnecting = false
			c.reconnectMu.Unlock()
		}()

		delay := c.reconnectDelay
		for attempt := 1; attempt <= c.maxReconnectAttempts; attempt++ {
			// Wait before attempting to reconnect
			time.Sleep(delay)
			
			logger.Infof("Reconnection attempt %d/%d for %s", attempt, c.maxReconnectAttempts, c.url)
			
			// Check if we should stop (e.g., if done channel is closed)
			select {
			case <-c.done:
				logger.Infof("Reconnection cancelled for %s", c.url)
				return
			default:
			}

			// Attempt to reconnect
			err := c.Connect()
			if err == nil {
				logger.Infof("Successfully reconnected to %s after %d attempts", c.url, attempt)
				// Reset delay for next time
				delay = c.reconnectDelay
				return
			}

			logger.Warnf("Reconnection attempt %d/%d failed for %s: %v", attempt, c.maxReconnectAttempts, c.url, err)
			
			// Exponential backoff: double the delay for next attempt (max 60 seconds)
			delay = delay * 2
			if delay > 60*time.Second {
				delay = 60 * time.Second
			}
		}

		logger.Errorf("Failed to reconnect to %s after %d attempts, giving up", c.url, c.maxReconnectAttempts)
	}()
}
