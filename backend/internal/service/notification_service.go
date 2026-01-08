package service

import (
	"context"
	"encoding/json"

	"tuyul/backend/internal/model"
	"tuyul/backend/pkg/logger"
	"tuyul/backend/pkg/redis"
)

// NotificationService handles publishing events to Redis for WebSocket broadcasting
type NotificationService struct {
	redis *redis.Client
	log   *logger.Logger
}

func NewNotificationService(redis *redis.Client) *NotificationService {
	return &NotificationService{
		redis: redis,
		log:   logger.GetLogger(),
	}
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

// NotifyPumpSignal sends a pump signal to all users
func (s *NotificationService) NotifyPumpSignal(ctx context.Context, payload interface{}) {
	s.Broadcast(ctx, model.MessageTypePumpSignal, payload)
}
