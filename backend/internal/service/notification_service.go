package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/logger"
	"tuyul/backend/pkg/redis"
)

// NotificationService handles publishing events to Redis for WebSocket broadcasting
type NotificationService struct {
	redis *redis.Client
	log   *logger.Logger

	// Market update batching
	marketUpdateBatch map[string]*model.Coin // pairID -> latest coin
	batchMu           sync.RWMutex
	batchTicker       *time.Ticker
	batchStop         chan struct{}
}

func NewNotificationService(redis *redis.Client) *NotificationService {
	ns := &NotificationService{
		redis:             redis,
		log:               logger.GetLogger(),
		marketUpdateBatch: make(map[string]*model.Coin),
		batchTicker:       time.NewTicker(2 * time.Second),
		batchStop:         make(chan struct{}),
	}

	// Start batch flusher
	go ns.startBatchFlusher()

	return ns
}

// NotifyUser sends a message to a specific user via WebSocket
func (s *NotificationService) NotifyUser(ctx context.Context, userID string, msgType model.WSMessageType, payload interface{}) {
	msg := model.WSMessage{
		Type:    msgType,
		Payload: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		s.log.Errorf("Failed to marshal notification: %v", err)
		return
	}

	channel := redis.GetWSUserKey(userID)
	if err := s.redis.Publish(ctx, channel, data); err != nil {
		s.log.Errorf("Failed to publish notification to channel %s: %v", channel, err)
	}
}

// Broadcast sends a message to all connected users
func (s *NotificationService) Broadcast(ctx context.Context, msgType model.WSMessageType, payload interface{}) {
	msg := model.WSMessage{
		Type:    msgType,
		Payload: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		s.log.Errorf("Failed to marshal broadcast notification: %v", err)
		return
	}

	channel := redis.GetWSBroadcastKey()
	if err := s.redis.Publish(ctx, channel, data); err != nil {
		s.log.Errorf("Failed to publish broadcast notification to channel %s: %v", channel, err)
	}
}

// NotifyOrderUpdate sends an order update notification to a user
func (s *NotificationService) NotifyOrderUpdate(ctx context.Context, userID string, order *model.Order) {
	s.NotifyUser(ctx, userID, model.MessageTypeOrderUpdate, order)
}

// NotifyBotUpdate sends a bot status update notification
func (s *NotificationService) NotifyBotUpdate(ctx context.Context, userID string, payload model.WSBotUpdatePayload) {
	s.NotifyUser(ctx, userID, model.MessageTypeBotUpdate, payload)
}

// NotifyPositionUpdate sends a position update notification to a user
func (s *NotificationService) NotifyPositionUpdate(ctx context.Context, userID string, position *model.Position) {
	s.NotifyUser(ctx, userID, model.MessageTypePositionUpdate, position)
}

// NotifyPumpSignal sends a pump signal to all users
func (s *NotificationService) NotifyPumpSignal(ctx context.Context, payload interface{}) {
	s.Broadcast(ctx, model.MessageTypePumpSignal, payload)
}

// NotifyMarketUpdate adds coin to batch (will be flushed every 2 seconds)
func (s *NotificationService) NotifyMarketUpdate(ctx context.Context, coin *model.Coin) {
	// Add to batch (always keep latest per pairID)
	s.batchMu.Lock()
	s.marketUpdateBatch[coin.PairID] = coin
	s.batchMu.Unlock()
}

// startBatchFlusher periodically flushes the market update batch
func (s *NotificationService) startBatchFlusher() {
	for {
		select {
		case <-s.batchTicker.C:
			s.flushMarketUpdateBatch()
		case <-s.batchStop:
			return
		}
	}
}

// flushMarketUpdateBatch sends all collected coin updates as a batch
func (s *NotificationService) flushMarketUpdateBatch() {
	s.batchMu.Lock()
	if len(s.marketUpdateBatch) == 0 {
		s.batchMu.Unlock()
		return
	}

	// Convert map to slice
	coins := make([]*model.Coin, 0, len(s.marketUpdateBatch))
	for _, coin := range s.marketUpdateBatch {
		coins = append(coins, coin)
	}

	// Clear batch
	s.marketUpdateBatch = make(map[string]*model.Coin)
	s.batchMu.Unlock()

	// Broadcast batch
	if len(coins) > 0 {
		ctx := context.Background()
		s.Broadcast(ctx, model.MessageTypeMarketUpdate, coins)
		s.log.Debugf("Flushed market update batch: %d coins", len(coins))
	}
}

// Stop stops the batch flusher and flushes any remaining updates
func (s *NotificationService) Stop() {
	s.batchTicker.Stop()
	close(s.batchStop)
	// Flush any remaining updates
	s.flushMarketUpdateBatch()
}
