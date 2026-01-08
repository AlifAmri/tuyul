package redis

import "fmt"

var globalPrefix string

// InitKeys initializes the global prefix for all Redis keys
func InitKeys(prefix string) {
	globalPrefix = prefix
}

func fmtKey(format string, a ...interface{}) string {
	return globalPrefix + fmt.Sprintf(format, a...)
}

// Redis key patterns for the application
// Following the pattern: entity:id or entity:id:attribute

// User keys
func UserKey(userID string) string {
	return fmtKey("user:%s", userID)
}

func UserByUsernameKey(username string) string {
	return fmtKey("user:username:%s", username)
}

func UserByEmailKey(email string) string {
	return fmtKey("user:email:%s", email)
}

// Session keys
func SessionKey(sessionID string) string {
	return fmtKey("session:%s", sessionID)
}

func UserSessionsKey(userID string) string {
	return fmtKey("user_sessions:%s", userID)
}

// Token blacklist
func TokenBlacklistKey(token string) string {
	return fmtKey("token_blacklist:%s", token)
}

// API Key keys
func APIKeyKey(userID string) string {
	return fmtKey("api_key:%s", userID)
}

// Market data keys
func CoinKey(pair string) string {
	return fmtKey("coin:%s", pair)
}

func AllCoinsKey() string {
	return fmtKey("coins:all")
}

func ActivePairsKey() string {
	return fmtKey("market:active_pairs")
}

func PumpScoreRankKey() string {
	return fmtKey("market:sorted:pump_score")
}

func GapRankKey() string {
	return fmtKey("market:sorted:gap_percentage")
}

func VolumeRankKey() string {
	return fmtKey("market:sorted:volume_idr")
}

func ChangeRankKey() string {
	return fmtKey("market:sorted:change_24h")
}

// Trade keys (for Copilot/Bot trades)
func TradeKey(tradeID string) string {
	return fmtKey("trade:%s", tradeID)
}

func UserTradesKey(userID string) string {
	return fmtKey("user_trades:%s", userID)
}

func TradesByStatusKey(status string) string {
	return fmtKey("trades_by_status:%s", status)
}

// Order keys
func OrderKey(orderID string) string {
	return fmtKey("order:%s", orderID)
}

func UserOrdersKey(userID string) string {
	return fmtKey("user_orders:%s", userID)
}

func OrdersByStatusKey(status string) string {
	return fmtKey("orders_by_status:%s", status)
}

func PairOrdersKey(pair string) string {
	return fmtKey("pair_orders:%s", pair)
}

func BuySellMapKey(buyOrderID string) string {
	return fmtKey("buy_sell_map:%s", buyOrderID)
}

func OrderIDMapKey(indodaxOrderID string) string {
	return fmtKey("order_id_map:%s", indodaxOrderID)
}

// BotOrdersKey returns a sorted set key for orders by bot (sorted by CreatedAt)
func BotOrdersKey(botID int64) string {
	return fmtKey("bot_orders:%d", botID)
}

// PositionOrdersKey returns a sorted set key for orders by position (sorted by CreatedAt)
func PositionOrdersKey(positionID int64) string {
	return fmtKey("position_orders:%d", positionID)
}

// Balance keys
func BalanceKey(userID, currency string) string {
	return fmtKey("balance:%s:%s", userID, currency)
}

func PaperBalanceKey(userID string) string {
	return fmtKey("paper_balance:user:%s", userID)
}

func BotPaperBalanceKey(botID int64) string {
	return fmtKey("paper_balance:bot:%d", botID)
}

// Bot keys
func BotKey(botID string) string {
	return fmtKey("bot:%s", botID)
}

func UserBotsKey(userID string) string {
	return fmtKey("user_bots:%s", userID)
}

func BotsByTypeKey(botType string) string {
	return fmtKey("bots_by_type:%s", botType)
}

func BotsByStatusKey(status string) string {
	return fmtKey("bots_by_status:%s", status)
}

// Bot position keys (for Pump Hunter)
func PositionKey(positionID string) string {
	return fmtKey("position:%s", positionID)
}

func BotPositionsKey(botID string) string {
	return fmtKey("bot_positions:%s", botID)
}

func ActivePositionsKey(botID string) string {
	return fmtKey("active_positions:%s", botID)
}

// Rate limiting keys
func RateLimitKey(identifier, action string) string {
	return fmtKey("rate_limit:%s:%s", action, identifier)
}

// WebSocket connection keys
func WSConnectionKey(userID string) string {
	return fmtKey("ws_connection:%s", userID)
}

// Indodax WebSocket token keys
func IndodaxWSTokenKey(userID string) string {
	return fmtKey("indodax_ws_token:%s", userID)
}

// Cache keys
func CachePairsKey() string {
	return fmtKey("cache:pairs")
}

func CachePriceIncrementsKey() string {
	return fmtKey("cache:price_increments")
}

func CacheTickerKey(pair string) string {
	return fmtKey("cache:ticker:%s", pair)
}

// WebSocket keys
func GetWSBroadcastKey() string {
	return fmtKey("ws:broadcast")
}

func GetWSUserKey(userID string) string {
	return fmtKey("ws:user:%s", userID)
}

// Pub/Sub channels
// NOTE: Channels are also prefixed when generated via functions or used in Pub/Sub
// We use fmtKey even for constant-like names if they are used as keys/channels

func GetChannelMarketUpdate() string      { return fmtKey("channel:market_update") }
func GetChannelPumpScoreUpdate() string   { return fmtKey("channel:pump_score_update") }
func GetChannelCoinUpdate() string        { return fmtKey("channel:coin_update") }
func GetChannelOrderUpdate() string       { return fmtKey("channel:order_update") }
func GetChannelStopLossTriggered() string { return fmtKey("channel:stop_loss_triggered") }
func GetChannelBotStatusUpdate() string   { return fmtKey("channel:bot_status_update") }
func GetChannelBotPnLUpdate() string      { return fmtKey("channel:bot_pnl_update") }
func GetChannelPositionUpdate() string    { return fmtKey("channel:position_update") }

const (
	// Reserved for prefix logic in UserChannel
	ChannelUserPrefix = "channel:user:"
)

// UserChannel returns a user-specific channel
func UserChannel(userID string) string {
	return fmtKey("%s%s", ChannelUserPrefix, userID)
}
