package model

// WSMessageType represents the type of WebSocket message
type WSMessageType string

const (
	MessageTypeMarketUpdate  WSMessageType = "market_update"
	MessageTypeOrderUpdate   WSMessageType = "order_update"
	MessageTypeBotUpdate     WSMessageType = "bot_update"
	MessageTypeBalanceUpdate WSMessageType = "balance_update"
	MessageTypePumpSignal    WSMessageType = "pump_signal"
	MessageTypeError         WSMessageType = "error"
	MessageTypeAuthSuccess   WSMessageType = "auth_success"
	MessageTypePong          WSMessageType = "pong"
)

// WSMessage is the envelope for all WebSocket messages
type WSMessage struct {
	Type    WSMessageType `json:"type"`
	Payload interface{}   `json:"payload"`
}

// WSAuthRequest is sent by client to authenticate after connection
type WSAuthRequest struct {
	Token string `json:"token"`
}

// WSBotUpdatePayload represents a bot status/PnL update
type WSBotUpdatePayload struct {
	BotID          int64              `json:"bot_id"`
	Status         string             `json:"status"`
	TotalTrades    int                `json:"total_trades"`
	WinningTrades  int                `json:"winning_trades"`
	WinRate        float64            `json:"win_rate"` // Win rate percentage
	TotalProfitIDR float64            `json:"total_profit_idr"`
	EquityIDR      float64            `json:"equity_idr"`
	Balances       map[string]float64 `json:"balances,omitempty"` // Real-time balance updates
}
