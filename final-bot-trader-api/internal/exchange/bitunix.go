package exchange

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"final-bot-trader-api/internal/circuitbreaker"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/utils"

	"github.com/google/uuid"
)

// FlexibleInt64 handles JSON values that can be either string or int64
// Bitunix sometimes returns timestamps as strings, sometimes as int64
type FlexibleInt64 int64

// UnmarshalJSON implements json.Unmarshaler to handle both string and int64
func (f *FlexibleInt64) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		val, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return err
		}
		*f = FlexibleInt64(val)
		return nil
	}

	var val int64
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	*f = FlexibleInt64(val)
	return nil
}

// Int64 returns the int64 value
func (f FlexibleInt64) Int64() int64 {
	return int64(f)
}

// BitunixClient implements the Client interface for Bitunix exchange
type BitunixClient struct {
	APIKey         string
	SecretKey      string
	BaseURL        string
	circuitBreaker *circuitbreaker.CircuitBreaker
}

// NewBitunixClient creates a new Bitunix exchange client
func NewBitunixClient(apiKey, secretKey, baseURL string) *BitunixClient {
	if baseURL == "" {
		baseURL = "https://fapi.bitunix.com"
	}

	cbConfig := circuitbreaker.DefaultConfig()
	cb := circuitbreaker.NewCircuitBreaker(cbConfig)

	return &BitunixClient{
		APIKey:         apiKey,
		SecretKey:      secretKey,
		BaseURL:        baseURL,
		circuitBreaker: cb,
	}
}

// SetCircuitBreakerConfig allows configuring the circuit breaker
func (c *BitunixClient) SetCircuitBreakerConfig(config circuitbreaker.Config) {
	c.circuitBreaker = circuitbreaker.NewCircuitBreaker(config)
}

// SetCircuitBreakerStateChangeCallback sets a callback for circuit breaker state changes
func (c *BitunixClient) SetCircuitBreakerStateChangeCallback(fn func(from, to circuitbreaker.State)) {
	if c.circuitBreaker != nil {
		c.circuitBreaker.SetStateChangeCallback(fn)
	}
}

// generateNonce generates a random nonce string using UUID
func (c *BitunixClient) generateNonce() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

// signRequest generates Bitunix signature using double SHA256
func (c *BitunixClient) signRequest(nonce, timestamp, apiKey, queryParams, body string) string {
	digestInput := strings.TrimSpace(nonce + timestamp + apiKey + queryParams + body)
	digestHash := sha256.Sum256([]byte(digestInput))
	digest := hex.EncodeToString(digestHash[:])

	signInput := strings.TrimSpace(digest + c.SecretKey)
	signHash := sha256.Sum256([]byte(signInput))
	return hex.EncodeToString(signHash[:])
}

func (c *BitunixClient) doRequest(
	ctx context.Context,
	method, endpoint string,
	params map[string]string,
	requestBody io.Reader,
	signed bool,
) ([]byte, error) {
	start := time.Now()
	endpointName := utils.ExtractEndpoint(endpoint)

	var result []byte
	var lastErr error

	isFailure := func(err error) bool {
		if err == nil {
			return false
		}
		if utils.IsRateLimitError(err) {
			return false
		}
		if err == context.Canceled || err == context.DeadlineExceeded {
			return false
		}
		return true
	}

	err := c.circuitBreaker.Call(func() error {
		retryConfig := utils.DefaultRetryConfig()
		retryConfig.MaxRetries = 2

		var rateLimitErr *utils.RateLimitError
		attempt := 0

		for attempt <= retryConfig.MaxRetries {
			var err error
			result, err = c.doRequestOnce(ctx, method, endpoint, params, requestBody, signed)
			if err == nil {
				return nil
			}

			lastErr = err

			if rle, ok := err.(*utils.RateLimitError); ok {
				rateLimitErr = rle
				retryAfter := rle.RetryAfter

				if retryAfter > 5*time.Minute {
					retryAfter = time.Duration(float64(retryAfter) * 0.5)
				}

				backoffDelay := time.Duration(1<<uint(attempt)) * time.Second
				if retryAfter > backoffDelay {
					backoffDelay = retryAfter
				}
				if backoffDelay > 60*time.Second {
					backoffDelay = 60 * time.Second
				}

				log.Printf("[Bitunix] Rate limit (HTTP 429) - Attempt %d/%d", attempt+1, retryConfig.MaxRetries+1)
				if rateLimitErr.Remaining > 0 {
					log.Printf("   Requests remaining: %d", rateLimitErr.Remaining)
				}
				if !rateLimitErr.ResetTime.IsZero() {
					log.Printf("   Reset at: %v", rateLimitErr.ResetTime.Format("15:04:05"))
				}
				log.Printf("   Waiting %v before retry...", backoffDelay)

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoffDelay):
					attempt++
					continue
				}
			}

			if strings.Contains(err.Error(), "timeout") ||
				strings.Contains(err.Error(), "connection") ||
				strings.Contains(err.Error(), "500") ||
				strings.Contains(err.Error(), "502") ||
				strings.Contains(err.Error(), "503") {
				attempt++
				if attempt <= retryConfig.MaxRetries {
					delay := time.Duration(float64(retryConfig.InitialDelay) *
						math.Pow(retryConfig.BackoffMultiplier, float64(attempt-1)))
					if delay > retryConfig.MaxDelay {
						delay = retryConfig.MaxDelay
					}
					time.Sleep(delay)
				}
				continue
			}

			return err
		}

		if rateLimitErr != nil {
			return fmt.Errorf("persistent rate limit after %d attempts: %w",
				retryConfig.MaxRetries+1, rateLimitErr)
		}

		return fmt.Errorf("retry exhausted after %d attempts: %w", retryConfig.MaxRetries+1, lastErr)
	}, isFailure)

	duration := time.Since(start)
	utils.RecordAPIMetrics("bitunix", endpointName, method, duration, err)

	if err != nil {
		if strings.Contains(err.Error(), "circuit breaker is OPEN") {
			log.Printf("[Bitunix] Circuit breaker OPEN - blocking API calls")
			utils.RecordCircuitBreakerFailure("bitunix")
		}
		return nil, err
	}

	return result, nil
}

func (c *BitunixClient) doRequestOnce(
	ctx context.Context,
	method, endpoint string,
	params map[string]string,
	requestBody io.Reader,
	signed bool,
) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	var bodyStr string
	if requestBody != nil {
		bodyBytes, err := io.ReadAll(requestBody)
		if err != nil {
			return nil, fmt.Errorf("error reading request body: %w", err)
		}
		bodyStr = string(bodyBytes)
		requestBody = strings.NewReader(bodyStr)
	}

	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	var fullURL string
	var headers map[string]string

	if signed {
		nonce := c.generateNonce()
		timestamp := fmt.Sprintf("%d", time.Now().UTC().UnixMilli())

		var sortedKeys []string
		for k := range params {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)

		var builder strings.Builder
		for _, k := range sortedKeys {
			builder.WriteString(k)
			builder.WriteString(params[k])
		}
		queryParamsStr := builder.String()

		signature := c.signRequest(nonce, timestamp, c.APIKey, queryParamsStr, bodyStr)

		headers = map[string]string{
			"api-key":      c.APIKey,
			"nonce":        nonce,
			"timestamp":    timestamp,
			"sign":         signature,
			"language":     "en-US",
			"Content-Type": "application/json",
		}

		if len(query) > 0 {
			fullURL = fmt.Sprintf("%s%s?%s", c.BaseURL, endpoint, query.Encode())
		} else {
			fullURL = fmt.Sprintf("%s%s", c.BaseURL, endpoint)
		}
	} else {
		if len(query) > 0 {
			fullURL = fmt.Sprintf("%s%s?%s", c.BaseURL, endpoint, query.Encode())
		} else {
			fullURL = fmt.Sprintf("%s%s", c.BaseURL, endpoint)
		}
		headers = map[string]string{
			"Content-Type": "application/json",
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, requestBody)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("context deadline exceeded: %w", err)
		}
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
			return nil, fmt.Errorf("request timeout (API took longer than 30s): %w", err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 429 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		remaining := parseRateLimitRemaining(resp.Header.Get("X-RateLimit-Remaining"))
		resetTime := parseRateLimitReset(resp.Header.Get("X-RateLimit-Reset"))

		if retryAfter == 0 {
			retryAfter = 60 * time.Second
		}

		return nil, &utils.RateLimitError{
			StatusCode: 429,
			RetryAfter: retryAfter,
			Message:    string(body),
			Remaining:  remaining,
			ResetTime:  resetTime,
		}
	}

	if resp.StatusCode >= 400 {
		log.Printf("Bitunix API error - Status: %d, Response: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("bitunix error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetPrice fetches the current price for a given symbol
func (c *BitunixClient) GetPrice(ctx context.Context, symbol string) (float64, error) {
	endpoint := "/api/v1/futures/market/tickers"
	params := url.Values{}
	params.Set("symbols", symbol)

	fullURL := fmt.Sprintf("%s%s?%s", c.BaseURL, endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("bitunix error %d: %s", resp.StatusCode, string(body))
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Symbol    string `json:"symbol"`
			MarkPrice string `json:"markPrice"`
			LastPrice string `json:"lastPrice"`
			Last      string `json:"last"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return 0, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return 0, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	for _, ticker := range bitunixResponse.Data {
		if ticker.Symbol == symbol {
			priceStr := ticker.LastPrice
			if priceStr == "" {
				priceStr = ticker.Last
			}
			if priceStr == "" {
				return 0, fmt.Errorf("no price found for symbol %s", symbol)
			}
			return strconv.ParseFloat(priceStr, 64)
		}
	}

	return 0, fmt.Errorf("symbol %s not found in tickers response", symbol)
}

// GetKlines fetches historical candle data for a symbol
func (c *BitunixClient) GetKlines(ctx context.Context, symbol, interval string, limit int, startTime, endTime int64) ([]model.Candle, error) {
	endpoint := "/api/v1/futures/market/kline"
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", interval)

	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	} else {
		params.Set("limit", "100")
	}
	if startTime > 0 {
		params.Set("startTime", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		params.Set("endTime", strconv.FormatInt(endTime, 10))
	}

	fullURL := fmt.Sprintf("%s%s?%s", c.BaseURL, endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bitunix error %d: %s", resp.StatusCode, string(body))
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Open     string `json:"open"`
			High     string `json:"high"`
			Low      string `json:"low"`
			Close    string `json:"close"`
			Time     string `json:"time"`
			QuoteVol string `json:"quoteVol"`
			BaseVol  string `json:"baseVol"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return nil, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return nil, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	if len(bitunixResponse.Data) == 0 {
		log.Printf("Bitunix returned empty array (code=%d, msg=%s, URL=%s)",
			bitunixResponse.Code, bitunixResponse.Msg, fullURL)
	}

	intervalDuration := parseInterval(interval)

	candles := make([]model.Candle, len(bitunixResponse.Data))
	for i, k := range bitunixResponse.Data {
		open, err := strconv.ParseFloat(k.Open, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing open price: %w", err)
		}
		high, err := strconv.ParseFloat(k.High, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing high price: %w", err)
		}
		low, err := strconv.ParseFloat(k.Low, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing low price: %w", err)
		}
		closePrice, err := strconv.ParseFloat(k.Close, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing close price: %w", err)
		}

		timeInt, err := strconv.ParseInt(k.Time, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing time: %w", err)
		}

		openTime := time.UnixMilli(timeInt)
		closeTime := openTime.Add(intervalDuration)

		volume, err := strconv.ParseFloat(k.BaseVol, 64)
		if err != nil {
			volume, _ = strconv.ParseFloat(k.QuoteVol, 64)
		}

		candles[i] = model.Candle{
			OpenTime:  openTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			CloseTime: closeTime,
		}
	}

	// Sort candles by OpenTime ascending (oldest first) for backtesting
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].OpenTime.Before(candles[j].OpenTime)
	})

	return candles, nil
}

func parseInterval(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "3m":
		return 3 * time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "8h":
		return 8 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "1d":
		return 24 * time.Hour
	case "3d":
		return 3 * 24 * time.Hour
	case "1w":
		return 7 * 24 * time.Hour
	case "1M":
		return 30 * 24 * time.Hour
	default:
		return time.Hour
	}
}

// GetAccountInfo returns account information
func (c *BitunixClient) GetAccountInfo(ctx context.Context) (*model.AccountInfo, error) {
	params := map[string]string{
		"marginCoin": "USDT",
	}

	body, err := c.doRequest(ctx, "GET", "/api/v1/futures/account", params, nil, true)
	if err != nil {
		return nil, fmt.Errorf("error getting account info: %w", err)
	}

	var bitunixResponse struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return nil, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return nil, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	var accountArray []struct {
		MarginCoin             string `json:"marginCoin"`
		Available              string `json:"available"`
		Frozen                 string `json:"frozen"`
		Margin                 string `json:"margin"`
		CrossUnrealizedPNL     string `json:"crossUnrealizedPNL"`
		IsolationUnrealizedPNL string `json:"isolationUnrealizedPNL"`
		PositionMode           string `json:"positionMode"`
	}

	var accountData struct {
		MarginCoin             string `json:"marginCoin"`
		Available              string `json:"available"`
		Frozen                 string `json:"frozen"`
		Margin                 string `json:"margin"`
		CrossUnrealizedPNL     string `json:"crossUnrealizedPNL"`
		IsolationUnrealizedPNL string `json:"isolationUnrealizedPNL"`
		PositionMode           string `json:"positionMode"`
	}

	if err := json.Unmarshal(bitunixResponse.Data, &accountArray); err == nil && len(accountArray) > 0 {
		accountData = accountArray[0]
	} else {
		if err := json.Unmarshal(bitunixResponse.Data, &accountData); err != nil {
			return nil, fmt.Errorf("error parsing account data: %w", err)
		}
	}

	availableBalance, _ := strconv.ParseFloat(accountData.Available, 64)
	frozen, _ := strconv.ParseFloat(accountData.Frozen, 64)
	margin, _ := strconv.ParseFloat(accountData.Margin, 64)

	var unrealizedPNL float64
	if accountData.PositionMode == "HEDGE" {
		unrealizedPNL, _ = strconv.ParseFloat(accountData.IsolationUnrealizedPNL, 64)
	} else {
		unrealizedPNL, _ = strconv.ParseFloat(accountData.CrossUnrealizedPNL, 64)
	}

	totalBalance := availableBalance + frozen + margin + unrealizedPNL

	log.Printf("Account Info - Available: %f, Frozen: %f, Margin: %f, UnrealizedPNL: %f, Total: %f",
		availableBalance, frozen, margin, unrealizedPNL, totalBalance)

	return &model.AccountInfo{
		TotalBalance:     totalBalance,
		AvailableBalance: availableBalance,
		MarginBalance:    margin,
		UnrealizedPnl:    unrealizedPNL,
	}, nil
}

// PlaceOrder places a buy or sell order
func (c *BitunixClient) PlaceOrder(ctx context.Context, order *model.OrderRequest) (*model.OrderResponse, error) {
	qtyStr := strconv.FormatFloat(order.Quantity, 'f', order.QuantityPrecision, 64)
	log.Printf("Order quantity for %s: %s", order.Symbol, qtyStr)

	// Validate SL/TP direction based on order side
	// For OPEN orders: BUY = LONG, SELL = SHORT
	isShort := strings.ToUpper(order.Side) == "SELL"

	// Get current price for validation
	currentPrice, priceErr := c.GetPrice(ctx, order.Symbol)
	if priceErr == nil && currentPrice > 0 {
		if isShort {
			// SHORT: SL must be ABOVE current price, TP must be BELOW
			if order.SL > 0 && order.SL <= currentPrice {
				log.Printf("[%s] WARNING: SL (%.6f) must be above current price (%.6f) for SHORT. Adjusting...",
					order.Symbol, order.SL, currentPrice)
				// Adjust SL to be above current price by a small margin
				order.SL = currentPrice * 1.02 // 2% above
			}
			if order.TP > 0 && order.TP >= currentPrice {
				log.Printf("[%s] WARNING: TP (%.6f) must be below current price (%.6f) for SHORT. Adjusting...",
					order.Symbol, order.TP, currentPrice)
				order.TP = currentPrice * 0.98 // 2% below
			}
		} else {
			// LONG: SL must be BELOW current price, TP must be ABOVE
			if order.SL > 0 && order.SL >= currentPrice {
				log.Printf("[%s] WARNING: SL (%.6f) must be below current price (%.6f) for LONG. Adjusting...",
					order.Symbol, order.SL, currentPrice)
				order.SL = currentPrice * 0.98 // 2% below
			}
			if order.TP > 0 && order.TP <= currentPrice {
				log.Printf("[%s] WARNING: TP (%.6f) must be above current price (%.6f) for LONG. Adjusting...",
					order.Symbol, order.TP, currentPrice)
				order.TP = currentPrice * 1.02 // 2% above
			}
		}
	}

	tradeSide := order.TradeSide
	if tradeSide == "" {
		tradeSide = "OPEN"
	}

	orderBody := map[string]interface{}{
		"symbol":    order.Symbol,
		"side":      strings.ToUpper(order.Side),
		"tradeSide": tradeSide,
		"orderType": strings.ToUpper(order.Type),
		"qty":       qtyStr,
	}

	// Add positionSide for hedge mode (required for closing positions)
	if order.PositionSide != "" {
		orderBody["positionSide"] = strings.ToUpper(order.PositionSide)
	}

	// Add reduceOnly flag if set
	if order.ReduceOnly {
		orderBody["reduceOnly"] = true
	}

	if order.Type == "LIMIT" {
		if order.Price <= 0 {
			return nil, fmt.Errorf("price is required for LIMIT orders")
		}
		priceStr := strconv.FormatFloat(order.Price, 'f', order.PricePrecision, 64)
		orderBody["price"] = priceStr
		orderBody["effect"] = "GTC"
	}

	if order.TP > 0 {
		// Use higher precision for TP/SL to avoid rounding issues
		tpPrecision := order.PricePrecision
		if tpPrecision < 6 {
			tpPrecision = 6 // Minimum 6 decimal places for TP
		}
		tpPriceStr := strconv.FormatFloat(order.TP, 'f', tpPrecision, 64)
		orderBody["tpPrice"] = tpPriceStr
		orderBody["tpStopType"] = "LAST_PRICE"
		orderBody["tpOrderType"] = "MARKET"
		log.Printf("Setting TP for %s: price=%s (precision=%d), type=LAST_PRICE, orderType=MARKET", order.Symbol, tpPriceStr, tpPrecision)
	}

	if order.SL > 0 {
		// Use higher precision for TP/SL to avoid rounding issues
		slPrecision := order.PricePrecision
		if slPrecision < 6 {
			slPrecision = 6 // Minimum 6 decimal places for SL
		}
		slPriceStr := strconv.FormatFloat(order.SL, 'f', slPrecision, 64)
		orderBody["slPrice"] = slPriceStr
		orderBody["slStopType"] = "LAST_PRICE"
		orderBody["slOrderType"] = "MARKET"
		log.Printf("Setting SL for %s: price=%s (precision=%d), type=LAST_PRICE, orderType=MARKET", order.Symbol, slPriceStr, slPrecision)
	}

	jsonBody, err := json.Marshal(orderBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling order body: %w", err)
	}

	log.Printf("Order body: %s", string(jsonBody))

	body, err := c.doRequest(ctx, "POST", "/api/v1/futures/trade/place_order", nil, strings.NewReader(string(jsonBody)), true)
	if err != nil {
		return nil, err
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderID  string `json:"orderId"`
			ClientID string `json:"clientId"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return nil, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return nil, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	orderID, err := strconv.ParseInt(bitunixResponse.Data.OrderID, 10, 64)
	if err != nil {
		log.Printf("Warning: could not parse orderId as int64: %s", bitunixResponse.Data.OrderID)
		orderID = 0
	}

	mainOrder := model.OrderResponse{
		OrderID:       orderID,
		Symbol:        order.Symbol,
		Status:        "NEW",
		Side:          order.Side,
		Type:          order.Type,
		ClientOrderID: bitunixResponse.Data.ClientID,
	}

	log.Printf("Order placed: OrderID=%s, ClientID=%s", bitunixResponse.Data.OrderID, bitunixResponse.Data.ClientID)

	return &mainOrder, nil
}

// SetLeverage sets the leverage for a symbol
func (c *BitunixClient) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	leverageBody := map[string]interface{}{
		"symbol":     symbol,
		"leverage":   leverage,
		"marginCoin": "USDT",
	}

	jsonBody, err := json.Marshal(leverageBody)
	if err != nil {
		return fmt.Errorf("error marshaling leverage body: %w", err)
	}

	var compactJSON bytes.Buffer
	if err := json.Compact(&compactJSON, jsonBody); err != nil {
		return fmt.Errorf("error compacting JSON: %w", err)
	}
	jsonBodyStr := compactJSON.String()

	log.Printf("Setting leverage for %s to %dx", symbol, leverage)

	body, err := c.doRequest(ctx, "POST", "/api/v1/futures/account/change_leverage", nil, strings.NewReader(jsonBodyStr), true)
	if err != nil {
		log.Printf("change_leverage failed, trying set_leverage: %v", err)
		body, err = c.doRequest(ctx, "POST", "/api/v1/futures/account/set_leverage", nil, strings.NewReader(jsonBodyStr), true)
	}
	if err != nil {
		return fmt.Errorf("error setting leverage: %w", err)
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	log.Printf("Leverage set to %dx for %s", leverage, symbol)
	return nil
}

// SetMarginType sets the margin type (isolated or cross) for a symbol
func (c *BitunixClient) SetMarginType(ctx context.Context, symbol string, marginType string) error {
	marginTypeUpper := strings.ToUpper(marginType)
	if marginTypeUpper != "ISOLATED" && marginTypeUpper != "CROSS" {
		return fmt.Errorf("invalid margin type: %s (must be 'isolated' or 'cross')", marginType)
	}

	marginBody := map[string]interface{}{
		"symbol":     symbol,
		"marginType": marginTypeUpper,
		"marginCoin": "USDT",
	}

	jsonBody, err := json.Marshal(marginBody)
	if err != nil {
		return fmt.Errorf("error marshaling margin type body: %w", err)
	}

	var compactJSON bytes.Buffer
	if err := json.Compact(&compactJSON, jsonBody); err != nil {
		return fmt.Errorf("error compacting JSON: %w", err)
	}
	jsonBodyStr := compactJSON.String()

	log.Printf("Setting margin type for %s to %s", symbol, marginTypeUpper)

	body, err := c.doRequest(ctx, "POST", "/api/v1/futures/account/set_margin_type", nil, strings.NewReader(jsonBodyStr), true)
	if err != nil {
		log.Printf("set_margin_type failed, trying change_margin_type: %v", err)
		body, err = c.doRequest(ctx, "POST", "/api/v1/futures/account/change_margin_type", nil, strings.NewReader(jsonBodyStr), true)
	}
	if err != nil {
		return fmt.Errorf("error setting margin type: %w", err)
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	log.Printf("Margin type set to %s for %s", marginTypeUpper, symbol)
	return nil
}

// SetTPSL sets Take Profit and Stop Loss for an existing position
func (c *BitunixClient) SetTPSL(ctx context.Context, symbol string, tpPrice, slPrice float64, pricePrecision int) error {
	positions, err := c.GetPositions(ctx)
	if err != nil {
		return fmt.Errorf("error getting positions for TP/SL: %w", err)
	}

	var currentPosition *model.Position
	for i := range positions {
		if positions[i].Symbol == symbol && positions[i].PositionAmt != 0 {
			currentPosition = &positions[i]
			break
		}
	}

	if currentPosition == nil {
		return fmt.Errorf("no open position found for %s", symbol)
	}

	log.Printf("[%s] TP/SL configuration after opening position is not fully supported in Bitunix API", symbol)
	log.Printf("   Expected TP: %.2f, SL: %.2f", tpPrice, slPrice)
	log.Printf("   Please configure TP/SL manually on the Bitunix platform")

	return nil
}

// GetOrderStatus checks the status of an order
func (c *BitunixClient) GetOrderStatus(ctx context.Context, symbol, orderID string) (*model.OrderStatus, error) {
	params := map[string]string{
		"orderId": orderID,
	}

	body, err := c.doRequest(ctx, "GET", "/api/v1/futures/trade/get_order_detail", params, nil, true)
	if err != nil {
		return nil, err
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderID     string        `json:"orderId"`
			Symbol      string        `json:"symbol"`
			Qty         string        `json:"qty"`
			TradeQty    string        `json:"tradeQty"`
			Price       string        `json:"price"`
			Side        string        `json:"side"`
			OrderType   string        `json:"orderType"`
			Effect      string        `json:"effect"`
			ClientID    string        `json:"clientId"`
			Status      string        `json:"status"`
			TPPrice     string        `json:"tpPrice"`
			SLPrice     string        `json:"slPrice"`
			CTime       FlexibleInt64 `json:"ctime"`
			MTime       FlexibleInt64 `json:"mtime"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return nil, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return nil, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	data := bitunixResponse.Data

	orderIDInt, err := strconv.ParseInt(data.OrderID, 10, 64)
	if err != nil {
		orderIDInt = 0
	}

	origQty, _ := strconv.ParseFloat(data.Qty, 64)
	tradeQty, _ := strconv.ParseFloat(data.TradeQty, 64)
	remainingQty := origQty - tradeQty

	return &model.OrderStatus{
		OrderID:       orderIDInt,
		Symbol:        data.Symbol,
		Status:        data.Status,
		ClientOrderID: data.ClientID,
		Price:         data.Price,
		OrigQty:       data.Qty,
		ExecutedQty:   data.TradeQty,
		TimeInForce:   data.Effect,
		Type:          data.OrderType,
		Side:          data.Side,
		Time:          data.CTime.Int64(),
		Quantity:      origQty,
		FilledQty:     tradeQty,
		RemainingQty:  remainingQty,
		UpdateTime:    data.MTime.Int64(),
		TPPrice:       data.TPPrice,
		SLPrice:       data.SLPrice,
	}, nil
}

// CancelOrder cancels an existing order
func (c *BitunixClient) CancelOrder(ctx context.Context, symbol, orderID string) error {
	orderList := []map[string]string{
		{"orderId": orderID},
	}

	cancelBody := map[string]interface{}{
		"symbol":    symbol,
		"orderList": orderList,
	}

	jsonBody, err := json.Marshal(cancelBody)
	if err != nil {
		return fmt.Errorf("error marshaling cancel order body: %w", err)
	}

	body, err := c.doRequest(ctx, "POST", "/api/v1/futures/trade/cancel_orders", nil, strings.NewReader(string(jsonBody)), true)
	if err != nil {
		return err
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			SuccessList []struct {
				OrderID  string `json:"orderId"`
				ClientID string `json:"clientId"`
			} `json:"successList"`
			FailureList []struct {
				OrderID   string `json:"orderId"`
				ClientID  string `json:"clientId"`
				ErrorMsg  string `json:"errorMsg"`
				ErrorCode string `json:"errorCode"`
			} `json:"failureList"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	for _, success := range bitunixResponse.Data.SuccessList {
		if success.OrderID == orderID {
			log.Printf("Order %s cancelled successfully", orderID)
			return nil
		}
	}

	for _, failure := range bitunixResponse.Data.FailureList {
		if failure.OrderID == orderID {
			return fmt.Errorf("failed to cancel order %s: %s (code: %s)", orderID, failure.ErrorMsg, failure.ErrorCode)
		}
	}

	log.Printf("Order %s cancellation response received, verify status via GetOrderStatus", orderID)
	return nil
}

// GetOpenOrders returns all open orders for a given symbol
func (c *BitunixClient) GetOpenOrders(ctx context.Context, symbol string) ([]model.OrderStatus, error) {
	params := map[string]string{}
	if symbol != "" {
		params["symbol"] = symbol
	}
	params["limit"] = "100"

	body, err := c.doRequest(ctx, "GET", "/api/v1/futures/trade/get_pending_orders", params, nil, true)
	if err != nil {
		return nil, err
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OrderList []struct {
				OrderID   string        `json:"orderId"`
				Symbol    string        `json:"symbol"`
				Qty       string        `json:"qty"`
				TradeQty  string        `json:"tradeQty"`
				Price     string        `json:"price"`
				Side      string        `json:"side"`
				OrderType string        `json:"orderType"`
				Effect    string        `json:"effect"`
				ClientID  string        `json:"clientId"`
				Status    string        `json:"status"`
				CTime     FlexibleInt64 `json:"ctime"`
				MTime     FlexibleInt64 `json:"mtime"`
			} `json:"orderList"`
			Total string `json:"total"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return nil, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return nil, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	openOrders := make([]model.OrderStatus, len(bitunixResponse.Data.OrderList))
	for i, orderData := range bitunixResponse.Data.OrderList {
		orderIDInt, _ := strconv.ParseInt(orderData.OrderID, 10, 64)
		origQty, _ := strconv.ParseFloat(orderData.Qty, 64)
		tradeQty, _ := strconv.ParseFloat(orderData.TradeQty, 64)
		remainingQty := origQty - tradeQty

		openOrders[i] = model.OrderStatus{
			OrderID:       orderIDInt,
			Symbol:        orderData.Symbol,
			Status:        orderData.Status,
			ClientOrderID: orderData.ClientID,
			Price:         orderData.Price,
			OrigQty:       orderData.Qty,
			ExecutedQty:   orderData.TradeQty,
			TimeInForce:   orderData.Effect,
			Type:          orderData.OrderType,
			Side:          orderData.Side,
			Time:          orderData.CTime.Int64(),
			Quantity:      origQty,
			FilledQty:     tradeQty,
			RemainingQty:  remainingQty,
			UpdateTime:    orderData.MTime.Int64(),
		}
	}

	totalInt, _ := strconv.ParseInt(bitunixResponse.Data.Total, 10, 64)
	log.Printf("Retrieved %d pending orders (total: %d)", len(openOrders), totalInt)
	return openOrders, nil
}

// GetBalance returns account balances
func (c *BitunixClient) GetBalance(ctx context.Context) ([]model.Balance, error) {
	params := map[string]string{
		"marginCoin": "USDT",
	}

	body, err := c.doRequest(ctx, "GET", "/api/v1/futures/account", params, nil, true)
	if err != nil {
		return nil, fmt.Errorf("error getting account info for balance: %w", err)
	}

	var bitunixResponse struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return nil, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return nil, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	var accountArray []struct {
		MarginCoin             string `json:"marginCoin"`
		Available              string `json:"available"`
		Frozen                 string `json:"frozen"`
		Margin                 string `json:"margin"`
		CrossUnrealizedPNL     string `json:"crossUnrealizedPNL"`
		IsolationUnrealizedPNL string `json:"isolationUnrealizedPNL"`
		PositionMode           string `json:"positionMode"`
	}

	if err := json.Unmarshal(bitunixResponse.Data, &accountArray); err != nil {
		var singleAccount struct {
			MarginCoin             string `json:"marginCoin"`
			Available              string `json:"available"`
			Frozen                 string `json:"frozen"`
			Margin                 string `json:"margin"`
			CrossUnrealizedPNL     string `json:"crossUnrealizedPNL"`
			IsolationUnrealizedPNL string `json:"isolationUnrealizedPNL"`
			PositionMode           string `json:"positionMode"`
		}
		if err2 := json.Unmarshal(bitunixResponse.Data, &singleAccount); err2 != nil {
			return nil, fmt.Errorf("error parsing account data: %w", err2)
		}
		accountArray = append(accountArray, singleAccount)
	}

	var balances []model.Balance
	for _, accountData := range accountArray {
		available, _ := strconv.ParseFloat(accountData.Available, 64)
		frozen, _ := strconv.ParseFloat(accountData.Frozen, 64)
		margin, _ := strconv.ParseFloat(accountData.Margin, 64)

		var unrealizedPNL float64
		if accountData.PositionMode == "HEDGE" {
			unrealizedPNL, _ = strconv.ParseFloat(accountData.IsolationUnrealizedPNL, 64)
		} else {
			unrealizedPNL, _ = strconv.ParseFloat(accountData.CrossUnrealizedPNL, 64)
		}

		totalBalance := available + frozen + margin + unrealizedPNL

		balance := model.Balance{
			Asset:              accountData.MarginCoin,
			Balance:            totalBalance,
			AvailableBalance:   available,
			CrossWalletBalance: totalBalance,
			CrossUnPnl:         unrealizedPNL,
			MaxWithdrawAmount:  available,
			MarginAvailable:    true,
			UpdateTime:         time.Now().UnixMilli(),
		}

		balances = append(balances, balance)
	}

	log.Printf("Retrieved %d balance entries", len(balances))
	return balances, nil
}

// GetPositions returns all open positions
func (c *BitunixClient) GetPositions(ctx context.Context) ([]model.Position, error) {
	params := map[string]string{}

	body, err := c.doRequest(ctx, "GET", "/api/v1/futures/position/get_pending_positions", params, nil, true)
	if err != nil {
		return nil, fmt.Errorf("error getting positions from Bitunix: %w", err)
	}

	var bitunixResponse struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			PositionID   string        `json:"positionId"`
			Symbol       string        `json:"symbol"`
			Qty          string        `json:"qty"`
			EntryValue   string        `json:"entryValue"`
			Side         string        `json:"side"`
			MarginMode   string        `json:"marginMode"`
			Leverage     int           `json:"leverage"`
			Margin       string        `json:"margin"`
			UnrealizedPNL string       `json:"unrealizedPNL"`
			LiqPrice     string        `json:"liqPrice"`
			AvgOpenPrice string        `json:"avgOpenPrice"`
			CTime        FlexibleInt64 `json:"ctime"`
			MTime        FlexibleInt64 `json:"mtime"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &bitunixResponse); err != nil {
		return nil, fmt.Errorf("error decoding Bitunix response: %w", err)
	}

	if bitunixResponse.Code != 0 {
		return nil, fmt.Errorf("bitunix API error: code=%d, msg=%s", bitunixResponse.Code, bitunixResponse.Msg)
	}

	var domainPositions []model.Position
	for _, p := range bitunixResponse.Data {
		qty, err := strconv.ParseFloat(p.Qty, 64)
		if err != nil {
			log.Printf("Warning: could not parse qty for position %s: %s", p.Symbol, p.Qty)
			continue
		}

		if qty == 0 {
			continue
		}

		entryPrice, _ := strconv.ParseFloat(p.AvgOpenPrice, 64)
		if entryPrice == 0 {
			entryValue, _ := strconv.ParseFloat(p.EntryValue, 64)
			if entryValue > 0 && qty > 0 {
				entryPrice = entryValue / qty
			}
		}

		unrealizedPNL, _ := strconv.ParseFloat(p.UnrealizedPNL, 64)
		liqPrice, _ := strconv.ParseFloat(p.LiqPrice, 64)
		margin, _ := strconv.ParseFloat(p.Margin, 64)

		side := p.Side
		if side == "" {
			if qty > 0 {
				side = "LONG"
			} else {
				side = "SHORT"
				qty = -qty
			}
		}

		domainPositions = append(domainPositions, model.Position{
			PositionID:    p.PositionID,
			Symbol:        p.Symbol,
			Side:          side,
			PositionAmt:   qty,
			EntryPrice:    entryPrice,
			UnrealizedPnl: unrealizedPNL,
			LiqPrice:      liqPrice,
			Leverage:      p.Leverage,
			MarginMode:    p.MarginMode,
			Margin:        margin,
		})
	}

	log.Printf("Retrieved %d open positions", len(domainPositions))
	return domainPositions, nil
}

// FlashClosePosition closes a position using the flash close endpoint
func (c *BitunixClient) FlashClosePosition(ctx context.Context, positionID string) error {
	closeBody := map[string]interface{}{
		"positionId": positionID,
	}

	jsonBody, err := json.Marshal(closeBody)
	if err != nil {
		return fmt.Errorf("error marshaling close body: %w", err)
	}

	log.Printf("Flash close body: %s", string(jsonBody))

	body, err := c.doRequest(ctx, "POST", "/api/v1/futures/trade/flash_close_position", nil, strings.NewReader(string(jsonBody)), true)
	if err != nil {
		return fmt.Errorf("error flash closing position: %w", err)
	}

	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("error decoding response: %w", err)
	}

	if response.Code != 0 {
		return fmt.Errorf("bitunix API error: code=%d, msg=%s", response.Code, response.Msg)
	}

	log.Printf("Position closed successfully for positionId=%s", positionID)
	return nil
}

// GetSymbolInfo returns information about a symbol
func (c *BitunixClient) GetSymbolInfo(ctx context.Context, symbol string) (*model.SymbolInfo, error) {
	var minQuantity, stepSize float64 = 0.001, 0.001
	var tickSize float64 = 0.0001 // Default tick size

	// Get current price to determine appropriate tick size
	price, err := c.GetPrice(ctx, symbol)
	if err == nil && price > 0 {
		// Determine tick size based on price magnitude
		// This ensures we have enough precision for SL/TP
		tickSize = calculateTickSize(price)
	}

	if strings.HasSuffix(symbol, "USDT") {
		baseSymbol := strings.TrimSuffix(symbol, "USDT")
		if baseSymbol == "BTC" || baseSymbol == "ETH" {
			minQuantity = 0.0001
			stepSize = 0.0001
		} else {
			minQuantity = 0.001
			stepSize = 0.001
		}
	}

	log.Printf("[%s] Symbol info: minQuantity=%.4f, stepSize=%.4f, tickSize=%.8f (price=%.6f)",
		symbol, minQuantity, stepSize, tickSize, price)

	return &model.SymbolInfo{
		Symbol:      symbol,
		MinQuantity: minQuantity,
		StepSize:    stepSize,
		MinPrice:    tickSize,
		TickSize:    tickSize,
	}, nil
}

// calculateTickSize determines the appropriate tick size based on price
func calculateTickSize(price float64) float64 {
	if price <= 0 {
		return 0.0001
	}

	// For very small prices (like PEPE at 0.004)
	if price < 0.01 {
		return 0.0000001
	}
	// For small prices (like DOGE at 0.10)
	if price < 0.1 {
		return 0.000001
	}
	// For medium-small prices (like DOGE at 0.10-1.0)
	if price < 1 {
		return 0.00001
	}
	// For prices 1-10
	if price < 10 {
		return 0.0001
	}
	// For prices 10-100
	if price < 100 {
		return 0.001
	}
	// For prices 100-1000
	if price < 1000 {
		return 0.01
	}
	// For prices 1000-10000
	if price < 10000 {
		return 0.1
	}
	// For very high prices (like BTC)
	return 1.0
}

// Helper functions

func parseRetryAfter(retryAfterStr string) time.Duration {
	if retryAfterStr == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(retryAfterStr); err == nil {
		return time.Duration(seconds) * time.Second
	}

	if t, err := time.Parse(time.RFC1123, retryAfterStr); err == nil {
		now := time.Now()
		if t.After(now) {
			return t.Sub(now)
		}
	}

	return 0
}

func parseRateLimitRemaining(remainingStr string) int {
	if remainingStr == "" {
		return -1
	}
	if remaining, err := strconv.Atoi(remainingStr); err == nil {
		return remaining
	}
	return -1
}

func parseRateLimitReset(resetStr string) time.Time {
	if resetStr == "" {
		return time.Time{}
	}
	if timestamp, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
		return time.Unix(timestamp, 0)
	}
	return time.Time{}
}
