package market

import (
	"encoding/json"
	"fmt"
	"strings"
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

	// Add message handler on WSClient (don't replace existing handlers)
	wsClient.AddMessageHandler(sm.handleWSMessage)

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
		sm.log.Infof("SubscriptionManager: Subscribing to order-book channel: %s for pair: %s", channel, pair)
		sm.wsClient.Subscribe(channel)
		sm.log.Infof("SubscriptionManager: Subscribe() called for channel: %s", channel)
	} else {
		sm.log.Debugf("SubscriptionManager: Pair %s already has %d subscribers, skipping subscription", pair, sm.refCount[pair])
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
	// Early return for non-order-book channels to avoid unnecessary processing
	if !strings.HasPrefix(channel, "market:order-book-") {
		return
	}

	sm.log.Infof("SubscriptionManager: Received order-book message on channel: %s (data length: %d bytes)", channel, len(data))

	var pair string
	if n, err := fmt.Sscanf(channel, "market:order-book-%s", &pair); err != nil || n != 1 {
		sm.log.Errorf("SubscriptionManager: Failed to extract pair from channel %s: %v", channel, err)
		return
	}
	sm.log.Infof("SubscriptionManager: Processing orderbook for pair: %s", pair)

	// Parse orderbook data
	// According to Indodax docs, the structure is:
	// {
	//   "result": {
	//     "channel": "market:order-book-btcidr",
	//     "data": {
	//       "data": {
	//         "pair": "btcidr",
	//         "ask": [...],
	//         "bid": [...]
	//       },
	//       "offset": 67409
	//     }
	//   }
	// }
	// The ws_client already extracts result.data, so we receive:
	// {
	//   "data": {
	//     "pair": "btcidr",
	//     "ask": [...],
	//     "bid": [...]
	//   },
	//   "offset": 67409
	// }
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
		Offset int64 `json:"offset"`
	}

	if err := json.Unmarshal(data, &obData); err != nil {
		sm.log.Errorf("Failed to unmarshal orderbook data for %s: %v. Raw data: %s", pair, err, string(data))
		return
	}

	if len(obData.Data.Ask) == 0 || len(obData.Data.Bid) == 0 {
		sm.log.Debugf("SubscriptionManager: Empty orderbook for %s (ask=%d bid=%d)", pair, len(obData.Data.Ask), len(obData.Data.Bid))
		return
	}

	// Parse best prices (first element in ask/bid arrays is the best price)
	var bestAsk, bestBid float64
	if _, err := fmt.Sscanf(obData.Data.Ask[0].Price, "%f", &bestAsk); err != nil {
		sm.log.Errorf("Failed to parse ask price for %s: %v", pair, err)
		return
	}
	if _, err := fmt.Sscanf(obData.Data.Bid[0].Price, "%f", &bestBid); err != nil {
		sm.log.Errorf("Failed to parse bid price for %s: %v", pair, err)
		return
	}

	ticker := OrderBookTicker{
		Pair:    pair,
		BestAsk: bestAsk,
		BestBid: bestBid,
	}

	// Notify subscribers
	sm.mu.RLock()
	handlers := sm.subscribers[pair]
	sm.mu.RUnlock()

	sm.log.Debugf("SubscriptionManager: Notifying %d handlers for pair %s (bid=%.2f ask=%.2f)", len(handlers), pair, bestBid, bestAsk)
	for _, handler := range handlers {
		handler(ticker)
	}
}
