package indodax

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// Client represents Indodax API client
type Client struct {
	apiURL               string
	httpClient           *http.Client
	publicLimiter        *rate.Limiter
	privateTradeLimiter  *rate.Limiter // 20 requests per second
	privateCancelLimiter *rate.Limiter // 30 requests per second
}

// NewClient creates a new Indodax client
func NewClient(apiURL string) *Client {
	return &Client{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		// Public API rate limited to 180 requests/minute (3 requests/second)
		publicLimiter: rate.NewLimiter(rate.Limit(3), 1),
		// Trade API rate limited to 20 requests per second
		privateTradeLimiter: rate.NewLimiter(rate.Limit(20), 5),
		// Cancel Order API rate limited to 30 requests per second
		privateCancelLimiter: rate.NewLimiter(rate.Limit(30), 5),
	}
}

// CommonResponse represents the common structure of Indodax responses
type CommonResponse struct {
	Success int    `json:"success"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// GetInfoResponse represents getInfo API response
type GetInfoResponse struct {
	CommonResponse
	Return *GetInfoReturn `json:"return"`
}

// GetInfoReturn represents the return data from getInfo
type GetInfoReturn struct {
	ServerTime  int64             `json:"server_time"`
	Balance     map[string]string `json:"balance"`
	BalanceHold map[string]string `json:"balance_hold"`
	UserID      string            `json:"user_id"`
	Name        string            `json:"name"`
	Email       string            `json:"email"`
}

type TradeRequest struct {
	Pair          string  `json:"pair"`
	Type          string  `json:"type"`                 // buy or sell
	Price         float64 `json:"price"`                // required for limit order
	IDR           float64 `json:"idr,omitempty"`        // amount of rupiah to buy coin
	Coin          float64 `json:"coin,omitempty"`       // amount of coin to buy/sell (mapped to 'btc' or specific coin param in logic)
	OrderType     string  `json:"order_type,omitempty"` // limit or market
	ClientOrderID string  `json:"client_order_id,omitempty"`
	TimeInForce   string  `json:"time_in_force,omitempty"` // GTC, MOC
}

type TradeResponse struct {
	CommonResponse
	Return *TradeReturn `json:"return"`
}

type TradeReturn struct {
	ReceiveCoin   string `json:"receive_btc"` // Indodax often uses receive_btc generic name or specific, handling might be tricky with JSON unmarshalling if key changes
	ReceiveIDR    string `json:"receive_idr"` // For sell
	SpendRP       int    `json:"spend_rp"`
	Fee           int    `json:"fee"`
	RemainRP      int    `json:"remain_rp"`
	OrderID       int64  `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
}

type OpenOrdersResponse struct {
	CommonResponse
	Return *OpenOrdersReturn `json:"return"`
}

type OpenOrdersReturn struct {
	Orders []OrderInfo `json:"orders"`
}

type OrderInfo struct {
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
	SubmitTime    string `json:"submit_time"`
	Price         string `json:"price"`
	Type          string `json:"type"`
	OrderType     string `json:"order_type"`
	OrderCoin     string `json:"order_btc"` // often generic
	RemainCoin    string `json:"remain_btc"`
	OrderIDR      string `json:"order_idr"`
	RemainIDR     string `json:"remain_idr"`
	Status        string `json:"status"`
}

type OrderHistoryResponse struct {
	CommonResponse
	Return *OrderHistoryReturn `json:"return"`
}

type OrderHistoryReturn struct {
	Orders []OrderInfo `json:"orders"`
}

type GetOrderResponse struct {
	CommonResponse
	Return *GetOrderReturn `json:"return"`
}

type GetOrderReturn struct {
	Order OrderInfo `json:"order"`
}

type CancelOrderResponse struct {
	CommonResponse
	Return *CancelOrderReturn `json:"return"`
}

type CancelOrderReturn struct {
	OrderID       int64             `json:"order_id"`
	ClientOrderID string            `json:"client_order_id"`
	Type          string            `json:"type"`
	Pair          string            `json:"pair"`
	Balance       map[string]string `json:"balance,omitempty"`
}

// ... Public API Structs (Pair, PriceIncrements, etc.) ...
// Pair represents a trading pair
type Pair struct {
	ID                     string  `json:"id"`
	Symbol                 string  `json:"symbol"`
	BaseCurrency           string  `json:"base_currency"`
	TradedCurrency         string  `json:"traded_currency"`
	TradedCurrencyUnit     string  `json:"traded_currency_unit"`
	Description            string  `json:"description"`
	TickerID               string  `json:"ticker_id"`
	VolumePrecision        int     `json:"volume_precision"`
	PricePrecision         int     `json:"price_precision"`
	PriceRound             int     `json:"price_round"`
	PriceScale             int     `json:"pricescale"`
	TradeMinBaseCurrency   int     `json:"trade_min_base_currency"`
	TradeMinTradedCurrency float64 `json:"trade_min_traded_currency"`
	HasMemo                bool    `json:"has_memo"`
	UrlLogo                string  `json:"url_logo"`
	UrlLogoPng             string  `json:"url_logo_png"`
}

// UnmarshalJSON is a custom unmarshaler for Pair to handle cases where Indodax API returns floats for int fields
func (p *Pair) UnmarshalJSON(data []byte) error {
	type Alias Pair
	aux := &struct {
		VolumePrecision interface{} `json:"volume_precision"`
		PricePrecision  interface{} `json:"price_precision"`
		PriceRound      interface{} `json:"price_round"`
		PriceScale      interface{} `json:"pricescale"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	toInt := func(v interface{}) int {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		default:
			return 0
		}
	}

	p.VolumePrecision = toInt(aux.VolumePrecision)
	p.PricePrecision = toInt(aux.PricePrecision)
	p.PriceRound = toInt(aux.PriceRound)
	p.PriceScale = toInt(aux.PriceScale)

	return nil
}

// PriceIncrementsResponse represents price increments response
type PriceIncrementsResponse struct {
	Increments map[string]string `json:"increments"`
}

// SummariesResponse represents market summaries response
type SummariesResponse struct {
	Tickers   map[string]TickerDetail `json:"tickers"`
	Prices24h map[string]string       `json:"prices_24h"`
	Prices7d  map[string]string       `json:"prices_7d"`
}

// TickerDetail represents ticker detail in summaries
type TickerDetail struct {
	High       string `json:"high"`
	Low        string `json:"low"`
	VolBTC     string `json:"vol_btc"` // Note: This field name depends on pair, can be vol_xx
	VolIDR     string `json:"vol_idr"` // Note: This field name depends on pair
	Last       string `json:"last"`
	Buy        string `json:"buy"`
	Sell       string `json:"sell"`
	ServerTime int64  `json:"server_time"`
	Name       string `json:"name"`
}

// TickerResponse represents single ticker response
type TickerResponse struct {
	Ticker Ticker `json:"ticker"`
}

// Ticker represents ticker data
type Ticker struct {
	High       string `json:"high"`
	Low        string `json:"low"`
	VolBase    string `json:"vol_base"`  // Dynamically mapped
	VolQuote   string `json:"vol_quote"` // Dynamically mapped
	Last       string `json:"last"`
	Buy        string `json:"buy"`
	Sell       string `json:"sell"`
	ServerTime int64  `json:"server_time"`
}

// Trade represents a trade history item
type Trade struct {
	Date   string `json:"date"`
	Price  string `json:"price"`
	Amount string `json:"amount"`
	TID    string `json:"tid"`
	Type   string `json:"type"` // "buy" or "sell"
}

// DepthResponse represents order book depth
type DepthResponse struct {
	Buy  [][]interface{} `json:"buy"`  // [price, volume]
	Sell [][]interface{} `json:"sell"` // [price, volume]
}

// Helper to create signature
func (c *Client) createSignature(message, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// doPrivateRequest executes a private API request
func (c *Client) doPrivateRequest(ctx context.Context, method string, params url.Values, key, secret string, result interface{}) error {
	// Rate Limiting
	if method == "trade" {
		if err := c.privateTradeLimiter.Wait(ctx); err != nil {
			return err
		}
	} else if method == "cancelOrder" {
		if err := c.privateCancelLimiter.Wait(ctx); err != nil {
			return err
		}
	} else {
		// Default reasonable limit logic could go here, or shared limiter
	}

	nonce := time.Now().UnixMilli()

	if params == nil {
		params = url.Values{}
	}
	params.Set("method", method)
	params.Set("nonce", fmt.Sprintf("%d", nonce))
	params.Set("timestamp", fmt.Sprintf("%d", nonce)) // Good practice even if nonce used

	payload := params.Encode()
	signature := c.createSignature(payload, secret)

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/tapi", strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Key", key)
	req.Header.Set("Sign", signature)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// First unmarshal into common response to check success
	var common CommonResponse
	if err := json.Unmarshal(body, &common); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if common.Success != 1 {
		errMsg := common.Error
		if errMsg == "" {
			errMsg = common.Message // Indodax sometimes uses message field
		}
		if errMsg == "" {
			errMsg = "unknown error from indodax"
		}
		return fmt.Errorf("indodax API error: %s", errMsg)
	}

	// If successful, unmarshal into the specific result
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to parse result: %w", err)
	}

	return nil
}

// ValidateAPIKey validates Indodax API credentials by calling getInfo
func (c *Client) ValidateAPIKey(key, secret string) (bool, error) {
	_, err := c.GetInfo(context.Background(), key, secret)
	if err != nil {
		// If error is related to credentials, return false, nil
		// Note: We need a better way to check specifically for auth errors
		// For now simple error check
		if strings.Contains(err.Error(), "Invalid credentials") || strings.Contains(err.Error(), "invalid_credentials") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetInfo gets account information
func (c *Client) GetInfo(ctx context.Context, key, secret string) (*GetInfoReturn, error) {
	var result GetInfoResponse
	if err := c.doPrivateRequest(ctx, "getInfo", nil, key, secret, &result); err != nil {
		return nil, err
	}
	return result.Return, nil
}

// Trade executes a trade order
func (c *Client) Trade(ctx context.Context, key, secret string, req TradeRequest) (*TradeReturn, error) {
	params := url.Values{}
	params.Set("pair", req.Pair)
	params.Set("type", req.Type)
	params.Set("price", fmt.Sprintf("%f", req.Price))

	if req.IDR > 0 {
		params.Set("idr", fmt.Sprintf("%f", req.IDR))
	}
	// For coin parameter, Indodax often requires the specific coin code as key (e.g. 'btc')
	// We need to parse the pair to find the coin code, or accept a param.
	// Usually for pair "btcidr", the coin param key is "btc".
	if req.Coin > 0 {
		// naive extraction: split pair by underscore or take prefix if 'idr' suffix
		coinKey := "btc" // default fallback
		if strings.HasSuffix(req.Pair, "idr") {
			coinKey = strings.TrimSuffix(req.Pair, "idr")
		} else if strings.Contains(req.Pair, "_") {
			parts := strings.Split(req.Pair, "_")
			if len(parts) > 0 {
				coinKey = parts[0]
			}
		}
		// override coin code if logic is complex - maybe map in future
		params.Set(coinKey, fmt.Sprintf("%f", req.Coin))
	}

	if req.OrderType != "" {
		params.Set("order_type", req.OrderType)
	}
	if req.ClientOrderID != "" {
		params.Set("client_order_id", req.ClientOrderID)
	}
	if req.TimeInForce != "" {
		params.Set("time_in_force", req.TimeInForce)
	}

	var result TradeResponse
	if err := c.doPrivateRequest(ctx, "trade", params, key, secret, &result); err != nil {
		return nil, err
	}
	return result.Return, nil
}

// OpenOrders gets open orders
func (c *Client) OpenOrders(ctx context.Context, key, secret string, pair string) ([]OrderInfo, error) {
	params := url.Values{}
	if pair != "" {
		params.Set("pair", pair)
	}

	var result OpenOrdersResponse
	if err := c.doPrivateRequest(ctx, "openOrders", params, key, secret, &result); err != nil {
		return nil, err
	}

	return result.Return.Orders, nil
}

// OrderHistory gets order history
func (c *Client) OrderHistory(ctx context.Context, key, secret string, pair string, count int) ([]OrderInfo, error) {
	params := url.Values{}
	params.Set("pair", pair)
	if count > 0 {
		params.Set("count", strconv.Itoa(count))
	}

	var result OrderHistoryResponse
	if err := c.doPrivateRequest(ctx, "orderHistory", params, key, secret, &result); err != nil {
		return nil, err
	}
	return result.Return.Orders, nil
}

// GetOrder gets specific order details
func (c *Client) GetOrder(ctx context.Context, key, secret string, pair string, orderID int64) (*OrderInfo, error) {
	params := url.Values{}
	params.Set("pair", pair)
	params.Set("order_id", fmt.Sprintf("%d", orderID))

	var result GetOrderResponse
	if err := c.doPrivateRequest(ctx, "getOrder", params, key, secret, &result); err != nil {
		return nil, err
	}
	return &result.Return.Order, nil
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(ctx context.Context, key, secret string, pair string, orderID int64, typeStr string) (*CancelOrderReturn, error) {
	params := url.Values{}
	params.Set("pair", pair)
	params.Set("order_id", fmt.Sprintf("%d", orderID))
	params.Set("type", typeStr) // buy or sell

	var result CancelOrderResponse
	if err := c.doPrivateRequest(ctx, "cancelOrder", params, key, secret, &result); err != nil {
		return nil, err
	}
	return result.Return, nil
}

// PrivateWSTokenResponse represents the response for generating private WS token
type PrivateWSTokenResponse struct {
	Success int                 `json:"success"`
	Return  *PrivateWSTokenInfo `json:"return"`
	Error   string              `json:"error,omitempty"`
}

// PrivateWSTokenInfo contains token and channel for private WS
type PrivateWSTokenInfo struct {
	ConnToken string `json:"connToken"`
	Channel   string `json:"channel"`
}

// GeneratePrivateWSToken generates a token for Private WebSocket
func (c *Client) GeneratePrivateWSToken(ctx context.Context, key, secret string) (*PrivateWSTokenInfo, error) {
	u := c.apiURL + "/api/private_ws/v1/generate_token"

	// Create body
	params := url.Values{}
	params.Set("client", "tapi")
	params.Set("tapi_key", key)
	payload := params.Encode()

	// Create signature
	signature := c.createSignature(payload, secret)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Sign", signature)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result PrivateWSTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Success != 1 || result.Return == nil {
		return nil, fmt.Errorf("failed to generate token: %s", result.Error)
	}

	return result.Return, nil
}

// doPublicRequest executes a public API request with rate limiting
func (c *Client) doPublicRequest(ctx context.Context, endpoint string, result interface{}) error {
	// Wait for rate limiter
	if err := c.publicLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}

// GetServerTime gets Indodax server time (public API)
func (c *Client) GetServerTime(ctx context.Context) (int64, error) {
	var result struct {
		ServerTime int64 `json:"server_time"`
	}

	if err := c.doPublicRequest(ctx, "/api/server_time", &result); err != nil {
		return 0, err
	}

	return result.ServerTime, nil
}

// GetPairs gets all available pairs
func (c *Client) GetPairs(ctx context.Context) ([]Pair, error) {
	var pairs []Pair
	if err := c.doPublicRequest(ctx, "/api/pairs", &pairs); err != nil {
		return nil, err
	}
	return pairs, nil
}

// GetPriceIncrements gets price increments for all pairs
func (c *Client) GetPriceIncrements(ctx context.Context) (map[string]string, error) {
	var result PriceIncrementsResponse
	if err := c.doPublicRequest(ctx, "/api/price_increments", &result); err != nil {
		return nil, err
	}
	return result.Increments, nil
}

// GetSummaries gets market summaries
func (c *Client) GetSummaries(ctx context.Context) (*SummariesResponse, error) {
	var result SummariesResponse
	if err := c.doPublicRequest(ctx, "/api/summaries", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTicker gets single ticker price
func (c *Client) GetTicker(ctx context.Context, pairID string) (*Ticker, error) {
	var result TickerResponse
	pairID = strings.ToLower(pairID)
	// Default to btcidr if empty
	if pairID == "" {
		pairID = "btcidr"
	} else if !strings.HasSuffix(pairID, "idr") && !strings.HasSuffix(pairID, "btc") && !strings.Contains(pairID, "_") {
		// Assuming IDR pair if not specified, but best to force full pair ID
		pairID = pairID + "idr"
	}

	if err := c.doPublicRequest(ctx, "/api/ticker/"+pairID, &result); err != nil {
		return nil, err
	}
	return &result.Ticker, nil
}

// GetTickerAll gets all ticker prices
// Note: The response structure for ticker_all is dynamic with keys as pair names
func (c *Client) GetTickerAll(ctx context.Context) (map[string]interface{}, error) {
	var result struct {
		Tickers map[string]interface{} `json:"tickers"`
	}
	if err := c.doPublicRequest(ctx, "/api/ticker_all", &result); err != nil {
		return nil, err
	}
	return result.Tickers, nil
}

// GetTrades gets trades for a pair
func (c *Client) GetTrades(ctx context.Context, pairID string) ([]Trade, error) {
	var trades []Trade
	if pairID == "" {
		pairID = "btcidr"
	}
	if err := c.doPublicRequest(ctx, "/api/trades/"+pairID, &trades); err != nil {
		return nil, err
	}
	return trades, nil
}

// GetDepth gets order book depth
func (c *Client) GetDepth(ctx context.Context, pairID string) (*DepthResponse, error) {
	var depth DepthResponse
	if pairID == "" {
		pairID = "btcidr"
	}
	if err := c.doPublicRequest(ctx, "/api/depth/"+pairID, &depth); err != nil {
		return nil, err
	}
	return &depth, nil
}


