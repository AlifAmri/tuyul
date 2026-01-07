package indodax

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client represents Indodax API client
type Client struct {
	apiURL     string
	httpClient *http.Client
}

// NewClient creates a new Indodax client
func NewClient(apiURL string) *Client {
	return &Client{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetInfoResponse represents getInfo API response
type GetInfoResponse struct {
	Success int                       `json:"success"`
	Return  *GetInfoReturn            `json:"return"`
	Error   string                    `json:"error,omitempty"`
}

// GetInfoReturn represents the return data from getInfo
type GetInfoReturn struct {
	ServerTime int64              `json:"server_time"`
	Balance    map[string]string  `json:"balance"`
	BalanceHold map[string]string `json:"balance_hold"`
	UserID     string             `json:"user_id"`
	Name       string             `json:"name"`
	Email      string             `json:"email"`
}

// ValidateAPIKey validates Indodax API credentials by calling getInfo
func (c *Client) ValidateAPIKey(key, secret string) (bool, error) {
	// Prepare request data
	nonce := time.Now().UnixMilli()
	method := "getInfo"
	
	// Create form data
	data := url.Values{}
	data.Set("method", method)
	data.Set("nonce", fmt.Sprintf("%d", nonce))

	// Create signature
	payload := data.Encode()
	signature := c.createSignature(payload, secret)

	// Create request
	req, err := http.NewRequest("POST", c.apiURL+"/tapi", strings.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Key", key)
	req.Header.Set("Sign", signature)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var result GetInfoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if successful
	if result.Success == 1 && result.Return != nil {
		return true, nil
	}

	// API key is invalid
	return false, nil
}

// GetInfo gets account information
func (c *Client) GetInfo(key, secret string) (*GetInfoReturn, error) {
	// Prepare request data
	nonce := time.Now().UnixMilli()
	method := "getInfo"
	
	// Create form data
	data := url.Values{}
	data.Set("method", method)
	data.Set("nonce", fmt.Sprintf("%d", nonce))

	// Create signature
	payload := data.Encode()
	signature := c.createSignature(payload, secret)

	// Create request
	req, err := http.NewRequest("POST", c.apiURL+"/tapi", strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Key", key)
	req.Header.Set("Sign", signature)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var result GetInfoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if successful
	if result.Success != 1 || result.Return == nil {
		if result.Error != "" {
			return nil, fmt.Errorf("indodax API error: %s", result.Error)
		}
		return nil, fmt.Errorf("invalid response from Indodax")
	}

	return result.Return, nil
}

// createSignature creates HMAC-SHA512 signature
func (c *Client) createSignature(message, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// GetServerTime gets Indodax server time (public API)
func (c *Client) GetServerTime() (int64, error) {
	resp, err := c.httpClient.Get(c.apiURL + "/api/server_time")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		ServerTime int64 `json:"server_time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	return result.ServerTime, nil
}

