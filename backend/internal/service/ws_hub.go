package service

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/logger"

	redisHelper "tuyul/backend/pkg/redis"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// Client represents a connected user over WebSocket
type Client struct {
	Hub    *WSHub
	Conn   *websocket.Conn
	UserID string
	Send   chan []byte
}

// WSHub handles WebSocket connections and broadcasting
type WSHub struct {
	clients    map[*Client]bool
	userConns  map[string][]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mu         sync.RWMutex

	redisClient *redis.Client
	log         *logger.Logger
}

func NewWSHub(redisClient *redis.Client) *WSHub {
	return &WSHub{
		clients:     make(map[*Client]bool),
		userConns:   make(map[string][]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan []byte),
		redisClient: redisClient,
		log:         logger.GetLogger(),
	}
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.userConns[client.UserID] = append(h.userConns[client.UserID], client)
			h.mu.Unlock()
			h.log.Infof("WS Client registered: UserID=%s", client.UserID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				// Remove from userConns
				conns := h.userConns[client.UserID]
				for i, c := range conns {
					if c == client {
						h.userConns[client.UserID] = append(conns[:i], conns[i+1:]...)
						break
					}
				}
				if len(h.userConns[client.UserID]) == 0 {
					delete(h.userConns, client.UserID)
				}
			}
			h.mu.Unlock()
			h.log.Infof("WS Client unregistered: UserID=%s", client.UserID)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *WSHub) Broadcast(msg model.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Errorf("Failed to marshal WS broadcast message: %v", err)
		return
	}
	h.broadcast <- data
}

// SendToUser sends a message to all active connections for a specific user
func (h *WSHub) SendToUser(userID string, msg model.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Errorf("Failed to marshal WS direct message: %v", err)
		return
	}

	h.mu.RLock()
	conns, ok := h.userConns[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	for _, client := range conns {
		select {
		case client.Send <- data:
		default:
			// Buffer full, handled by unregistering later
		}
	}
}

// ReadPump handles messages from the client (e.g., heartbeats)
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(512)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Hub.log.Errorf("WS error: %v", err)
			}
			break
		}
		// Currently we don't handle incoming messages other than control messages
		// If we do, we can add logic here (e.g., subscription requests)
	}
}

// WritePump handles outgoing messages to the client
func (c *Client) WritePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Hub closed the channel
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// StartPubSubListener listens to Redis Pub/Sub channels to bridge internal events to WS
func (h *WSHub) StartPubSubListener(ctx context.Context) {
	broadcastKey := redisHelper.GetWSBroadcastKey()
	userPattern := redisHelper.GetWSUserKey("*")
	userPrefix := redisHelper.GetWSUserKey("")

	pubsub := h.redisClient.Subscribe(ctx, broadcastKey, userPattern)
	defer pubsub.Close()

	ch := pubsub.Channel()

	for msg := range ch {
		if msg.Channel == broadcastKey {
			var wsMsg model.WSMessage
			if err := json.Unmarshal([]byte(msg.Payload), &wsMsg); err == nil {
				h.Broadcast(wsMsg)
			}
		} else if len(msg.Channel) > len(userPrefix) && msg.Channel[:len(userPrefix)] == userPrefix {
			userID := msg.Channel[len(userPrefix):]
			var wsMsg model.WSMessage
			if err := json.Unmarshal([]byte(msg.Payload), &wsMsg); err == nil {
				h.SendToUser(userID, wsMsg)
			}
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // In production, check origin
	},
}

// ServeWS handles WebSocket upgrade requests
func (h *WSHub) ServeWS(c *gin.Context) {
	u, exists := c.Get("user_id")
	if !exists {
		util.SendError(c, util.ErrUnauthorized("User not authenticated"))
		return
	}
	userID := u.(string)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Errorf("Failed to upgrade websocket: %v", err)
		return
	}

	client := &Client{
		Hub:    h,
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

	h.register <- client

	go client.WritePump()
	go client.ReadPump()
}
