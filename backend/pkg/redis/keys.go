package redis

import "fmt"

// Redis key patterns for the application
// Following the pattern: entity:id or entity:id:attribute

// User keys
func UserKey(userID string) string {
	return fmt.Sprintf("user:%s", userID)
}

func UserByUsernameKey(username string) string {
	return fmt.Sprintf("user:username:%s", username)
}

func UserByEmailKey(email string) string {
	return fmt.Sprintf("user:email:%s", email)
}

// Session keys
func SessionKey(sessionID string) string {
	return fmt.Sprintf("session:%s", sessionID)
}

func UserSessionsKey(userID string) string {
	return fmt.Sprintf("user_sessions:%s", userID)
}

// Token blacklist
func TokenBlacklistKey(token string) string {
	return fmt.Sprintf("token_blacklist:%s", token)
}

// API Key keys
func APIKeyKey(userID string) string {
	return fmt.Sprintf("api_key:%s", userID)
}

// Market data keys
func CoinKey(pair string) string {
	return fmt.Sprintf("coin:%s", pair)
}

func AllCoinsKey() string {
	return "coins:all"
}

func PumpScoreRankKey() string {
	return "rank:pump_score"
}

func GapRankKey() string {
	return "rank:gap_percentage"
}

func VolumeRankKey() string {
	return "rank:volume_idr"
}

func ChangeRankKey() string {
	return "rank:change_24h"
}

// Order keys
func OrderKey(orderID string) string {
	return fmt.Sprintf("order:%s", orderID)
}

func UserOrdersKey(userID string) string {
	return fmt.Sprintf("user_orders:%s", userID)
}

func OrdersByStatusKey(status string) string {
	return fmt.Sprintf("orders_by_status:%s", status)
}

func PairOrdersKey(pair string) string {
	return fmt.Sprintf("pair_orders:%s", pair)
}

func BuySellMapKey(buyOrderID string) string {
	return fmt.Sprintf("buy_sell_map:%s", buyOrderID)
}

// Balance keys
func BalanceKey(userID, currency string) string {
	return fmt.Sprintf("balance:%s:%s", userID, currency)
}

// Bot keys
func BotKey(botID string) string {
	return fmt.Sprintf("bot:%s", botID)
}

func UserBotsKey(userID string) string {
	return fmt.Sprintf("user_bots:%s", userID)
}

func BotsByTypeKey(botType string) string {
	return fmt.Sprintf("bots_by_type:%s", botType)
}

func BotsByStatusKey(status string) string {
	return fmt.Sprintf("bots_by_status:%s", status)
}

// Bot position keys (for Pump Hunter)
func PositionKey(positionID string) string {
	return fmt.Sprintf("position:%s", positionID)
}

func BotPositionsKey(botID string) string {
	return fmt.Sprintf("bot_positions:%s", botID)
}

func ActivePositionsKey(botID string) string {
	return fmt.Sprintf("active_positions:%s", botID)
}

// Rate limiting keys
func RateLimitKey(identifier, action string) string {
	return fmt.Sprintf("rate_limit:%s:%s", action, identifier)
}

// WebSocket connection keys
func WSConnectionKey(userID string) string {
	return fmt.Sprintf("ws_connection:%s", userID)
}

// Indodax WebSocket token keys
func IndodaxWSTokenKey(userID string) string {
	return fmt.Sprintf("indodax_ws_token:%s", userID)
}

// Cache keys
func CachePairsKey() string {
	return "cache:pairs"
}

func CachePriceIncrementsKey() string {
	return "cache:price_increments"
}

func CacheTickerKey(pair string) string {
	return fmt.Sprintf("cache:ticker:%s", pair)
}

// Pub/Sub channels
const (
	// Market update channels
	ChannelMarketUpdate     = "channel:market_update"
	ChannelPumpScoreUpdate  = "channel:pump_score_update"
	ChannelCoinUpdate       = "channel:coin_update"

	// Order update channels
	ChannelOrderUpdate      = "channel:order_update"
	ChannelStopLossTriggered = "channel:stop_loss_triggered"

	// Bot update channels
	ChannelBotStatusUpdate  = "channel:bot_status_update"
	ChannelBotPnLUpdate     = "channel:bot_pnl_update"
	ChannelPositionUpdate   = "channel:position_update"

	// User-specific channels
	ChannelUserPrefix       = "channel:user:"
)

// UserChannel returns a user-specific channel
func UserChannel(userID string) string {
	return fmt.Sprintf("%s%s", ChannelUserPrefix, userID)
}

