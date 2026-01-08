package market

import (
	"encoding/json"
	"fmt"
	"sync"

	"tuyul/backend/pkg/indodax"
	"tuyul/backend/pkg/logger"
)

// OrderBookTicker represents the best bid and ask for a pair
type OrderBookTicker struct {
	Pair    string  `json:"pair"`
	BestBid float64 `json:"best_bid"`
	BestAsk float64 `json:"best_ask"`
}

// TickerHandler is a callback function for ticker updates
type TickerHandler func(ticker OrderBookTicker)

type SubscriptionManager struct {
	wsClient *indodax.WSClient
	log      *logger.Logger

	// pair -> list of subscribers
	subscribers map[string][]TickerHandler
	// pair -> reference count
	refCount map[string]int
	mu       sync.RWMutex
}

func NewSubscriptionManager(wsClient *indodax.WSClient) *SubscriptionManager {
	sm := &SubscriptionManager{
		wsClient:    wsClient,
		log:         logger.GetLogger(),
		subscribers: make(map[string][]TickerHandler),
		refCount:    make(map[string]int),
	}

	// Set message handler on WSClient
	wsClient.SetMessageHandler(sm.handleWSMessage)

	return sm
}

// Subscribe subscribes to ticker updates for a pair
func (sm *SubscriptionManager) Subscribe(pair string, handler TickerHandler) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	channel := fmt.Sprintf("market:order-book-%s", pair)

	// If first subscriber, subscribe via WSClient
	if sm.refCount[pair] == 0 {
		if err := sm.wsClient.Connect(); err != nil {
			return fmt.Errorf("failed to connect ws: %w", err)
		}
		sm.wsClient.Subscribe(channel)
		sm.log.Infof("Subscribed to WebSocket channel: %s", channel)
	}

	sm.subscribers[pair] = append(sm.subscribers[pair], handler)
	sm.refCount[pair]++

	return nil
}

// Unsubscribe unsubscribes from ticker updates
func (sm *SubscriptionManager) Unsubscribe(pair string, handler TickerHandler) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	handlers := sm.subscribers[pair]
	for i, h := range handlers {
		// Note: comparing functions is tricky in Go, but we can use pointers if needed
		// For now, we'll just remove the last one or implement a better way if multiple bots use same pair
		// In production, we'd use a unique ID for each subscriber
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			sm.subscribers[pair] = append(handlers[:i], handlers[i+1:]...)
			sm.refCount[pair]--
			break
		}
	}

	if sm.refCount[pair] == 0 {
		channel := fmt.Sprintf("market:order-book-%s", pair)
		sm.wsClient.Unsubscribe(channel)
		delete(sm.subscribers, pair)
		delete(sm.refCount, pair)
		sm.log.Infof("Unsubscribed from WebSocket channel: %s", channel)
	}
}

func (sm *SubscriptionManager) handleWSMessage(channel string, data []byte) {
	// We only care about order-book channels
	// Format: market:order-book-btcidr
	var pair string
	if n, err := fmt.Sscanf(channel, "market:order-book-%s", &pair); err != nil || n != 1 {
		return
	}

	// Parse orderbook data
	var obData struct {
		Data struct {
			Pair string `json:"pair"`
			Ask  []struct {
				Price string `json:"price"`
			} `json:"ask"`
			Bid []struct {
				Price string `json:"price"`
			} `json:"bid"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &obData); err != nil {
		sm.log.Errorf("Failed to unmarshal orderbook data for %s: %v", pair, err)
		return
	}

	if len(obData.Data.Ask) == 0 || len(obData.Data.Bid) == 0 {
		return
	}

	// Parse best prices
	var bestAsk, bestBid float64
	fmt.Sscanf(obData.Data.Ask[0].Price, "%f", &bestAsk)
	fmt.Sscanf(obData.Data.Bid[0].Price, "%f", &bestBid)

	ticker := OrderBookTicker{
		Pair:    pair,
		BestAsk: bestAsk,
		BestBid: bestBid,
	}

	// Notify subscribers
	sm.mu.RLock()
	handlers := sm.subscribers[pair]
	sm.mu.RUnlock()

	for _, handler := range handlers {
		handler(ticker)
	}
}
