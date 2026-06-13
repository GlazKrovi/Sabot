package bot

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	// Given by https://developer.revolut.com/docs/x-api/place-order
	MaxDailyOrder int = 1000

	// Revolut requires the full limit price to be available in balance even if the order
	// fills at market price. These multipliers match what placeOrder submits in immediate mode.
	ImmediateBuyPriceMultiplier  = 1.20
	ImmediateSellPriceMultiplier = 0.80
)

// Sentinel errors for quantity validation
var (
	// ErrOrderTooSmall indicates the order is below Revolut's minimum tradable
	// size for the symbol — either the requested quantity is zero/rounds down
	// to zero at the symbol's precision, or the order's estimated USD value is
	// below the exchange's minimum notional. Such orders should be skipped
	// gracefully rather than retried or treated as fatal.
	ErrOrderTooSmall = errors.New("order is too small to be placed")
)

// PlaceOrderRequest represents the JSON body sent to the Revolut place-order endpoint.
type PlaceOrderRequest struct {
	ClientOrderID      string              `json:"client_order_id"`
	Symbol             string              `json:"symbol"`
	Side               string              `json:"side"`
	OrderConfiguration *OrderConfiguration `json:"order_configuration,omitempty"`
}

type OrderConfiguration struct {
	Limit  *LimitConfiguration  `json:"limit,omitempty"`
	Market *MarketConfiguration `json:"market,omitempty"`
}

type MarketConfiguration struct {
	BaseSize string `json:"base_size"` // crypto quantity to buy/sell at market price
}

type LimitConfiguration struct {
	BaseSize string `json:"base_size"` // Must be string according to API
	Price    string `json:"price"`     // Must be string according to API
}

// PlaceOrderResponse models the response from POST /api/1.0/orders.
// That endpoint only returns order identifiers and its initial state — no
// quantity/price/fee info. Those must be fetched separately via GetOrder.
type PlaceOrderResponse struct {
	Data struct {
		VenueOrderID  string `json:"venue_order_id"`
		ClientOrderID string `json:"client_order_id"`
		State         string `json:"state"` // pending_new|new|partially_filled|filled|cancelled|rejected|replaced
	} `json:"data"`
}

// insufficientBalancePattern matches the "(extra $X.XX is required)" suffix of
// Revolut's "Insufficient balance" error, returned when a buy order's required
// USD exceeds the available balance.
var insufficientBalancePattern = regexp.MustCompile(`extra \$([0-9]+(?:\.[0-9]+)?) is required`)

// MaxInsufficientBalanceRetries caps how many times placeOrder shrinks the
// order quantity and retries after an "insufficient balance" error.
const MaxInsufficientBalanceRetries = 3

// quantityTooSmallPattern matches Revolut's "quantity must be greater than 0,
// but is X" error, returned when the requested quantity rounds down to zero
// at the symbol's tradable precision (i.e., it's below the minimum tradable
// amount for that symbol).
var quantityTooSmallPattern = regexp.MustCompile(`quantity must be greater than 0, but is`)

// estimatedAmountTooSmallPattern matches Revolut's "Estimated amount for order
// is too small: QuoteAmount[amount=...]" error, returned when the order's
// total USD value is below the exchange's minimum notional for the symbol.
var estimatedAmountTooSmallPattern = regexp.MustCompile(`Estimated amount for order is too small`)

// isOrderTooSmallError reports whether err is one of Revolut's "order below
// minimum tradable size" errors (zero/rounded-down quantity, or USD value
// below the minimum notional).
func isOrderTooSmallError(err error) bool {
	return quantityTooSmallPattern.MatchString(err.Error()) || estimatedAmountTooSmallPattern.MatchString(err.Error())
}

// parseInsufficientBalanceExtra extracts the missing USD amount from a Revolut
// "Insufficient balance ... (extra $X.XX is required)" error, if present.
func parseInsufficientBalanceExtra(err error) (float64, bool) {
	matches := insufficientBalancePattern.FindStringSubmatch(err.Error())
	if matches == nil {
		return 0, false
	}
	extra, parseErr := strconv.ParseFloat(matches[1], 64)
	if parseErr != nil {
		return 0, false
	}
	return extra, true
}

// placeOrder is a helper that sends a POST to the Revolut place-order endpoint.
// Set immediate=true to use aggressive pricing for instant execution (simulates market order).
// If the API rejects the order for insufficient USD balance, it retries at the
// same limit price with the quantity reduced by the missing amount, up to
// MaxInsufficientBalanceRetries times.
func placeOrder(nomPair string, quantite float64, side string, currentPrice float64, immediate bool, printInfo bool) (PlaceOrderResponse, error) {
	if quantite <= 0 {
		return PlaceOrderResponse{}, ErrOrderTooSmall
	}

	// Calculate limit price based on execution mode
	var limitPrice float64
	if immediate {
		// Immediate mode: use aggressive pricing for instant execution
		if side == "buy" {
			limitPrice = currentPrice * ImmediateBuyPriceMultiplier
		} else {
			limitPrice = currentPrice * ImmediateSellPriceMultiplier
		}
	} else {
		// Normal mode: conservative pricing (may not execute immediately)
		limitPrice = currentPrice + 1.0
	}

	apiURL := "https://revx.revolut.com/api/1.0/orders"

	for attempt := 0; ; attempt++ {
		if quantite <= 0 {
			return PlaceOrderResponse{}, ErrOrderTooSmall
		}

		// Convert float values to strings as required by API
		config := PlaceOrderRequest{
			ClientOrderID: uuid.NewString(),
			Symbol:        nomPair,
			Side:          side,
			OrderConfiguration: &OrderConfiguration{
				Limit: &LimitConfiguration{
					BaseSize: fmt.Sprintf("%.8f", quantite),
					Price:    fmt.Sprintf("%.2f", limitPrice),
				},
			},
		}

		// Pass struct directly - will be marshalled to compact JSON
		result, err := contactApi[PlaceOrderResponse]("POST", apiURL, config, printInfo)
		if err == nil {
			return result, nil
		}

		if isOrderTooSmallError(err) {
			return PlaceOrderResponse{}, ErrOrderTooSmall
		}

		extra, retryable := parseInsufficientBalanceExtra(err)
		if !retryable || attempt >= MaxInsufficientBalanceRetries {
			return PlaceOrderResponse{}, fmt.Errorf("place order failed: %w", err)
		}

		quantite -= extra / limitPrice
		if printInfo {
			fmt.Printf("Insufficient balance (missing $%.2f), retrying at the same price %.2f with quantity reduced to %.8f\n", extra, limitPrice, quantite)
		}
	}
}

// Buy places a buy order for the given pair and quantity.
// Set immediate=true for instant execution at market price (uses aggressive limit pricing)
func Buy(nom_pair string, quantite float64, currentPrice float64, immediate bool, printInfo bool) (PlaceOrderResponse, error) {
	return placeOrder(nom_pair, quantite, "buy", currentPrice, immediate, printInfo)
}

// placeMarketOrder sends a market order (no price — fills at current market price).
func placeMarketOrder(nomPair string, quantite float64, side string, printInfo bool) (PlaceOrderResponse, error) {
	if quantite <= 0 {
		return PlaceOrderResponse{}, ErrOrderTooSmall
	}

	config := PlaceOrderRequest{
		ClientOrderID: uuid.NewString(),
		Symbol:        nomPair,
		Side:          side,
		OrderConfiguration: &OrderConfiguration{
			Market: &MarketConfiguration{
				BaseSize: fmt.Sprintf("%.8f", quantite),
			},
		},
	}

	result, err := contactApi[PlaceOrderResponse]("POST", "https://revx.revolut.com/api/1.0/orders", config, printInfo)
	if err != nil {
		if isOrderTooSmallError(err) {
			return PlaceOrderResponse{}, ErrOrderTooSmall
		}
		return PlaceOrderResponse{}, fmt.Errorf("place market order failed: %w", err)
	}
	return result, nil
}

// Sell places a market sell order for the given pair and quantity.
// Always sells exactly the requested crypto quantity at the current market price.
func Sell(nomPair string, quantite float64, printInfo bool) (PlaceOrderResponse, error) {
	return placeMarketOrder(nomPair, quantite, "sell", printInfo)
}

// OrderDetails models the response from GET /api/1.0/orders/{id}.
// All decimal values are returned by the API as strings.
type OrderDetails struct {
	Data struct {
		ID               string `json:"id"`
		Status           string `json:"status"` // pending_new|new|partially_filled|filled|cancelled|rejected|replaced
		FilledQuantity   string `json:"filled_quantity"`
		FilledAmount     string `json:"filled_amount"` // USD value of the fill
		AverageFillPrice string `json:"average_fill_price"`
		TotalFee         string `json:"total_fee"`
		FeeCurrency      string `json:"fee_currency"`
	} `json:"data"`
}

// GetOrder fetches the current state of an order by its venue order ID.
func GetOrder(orderID string, printInfo bool) (OrderDetails, error) {
	return contactApi[OrderDetails]("GET", "https://revx.revolut.com/api/1.0/orders/"+orderID, map[string]string{}, printInfo)
}

// orderNotFoundPattern matches Revolut's 404 "Order with ID '...' not found." error,
// which can happen briefly right after placing an order (eventual consistency).
var orderNotFoundPattern = regexp.MustCompile(`Order with ID '.+' not found`)

// MaxOrderNotFoundAttempts caps how many times getOrderWithRetry calls GetOrder
// when the order isn't found yet.
const MaxOrderNotFoundAttempts = 3

// getOrderWithRetry calls GetOrder, retrying after a random delay between 0.2
// and 1.5 seconds if the order isn't found yet (eventual consistency right
// after placing it). It gives up after MaxOrderNotFoundAttempts.
func getOrderWithRetry(orderID string, printInfo bool) (OrderDetails, error) {
	var details OrderDetails
	var err error
	for attempt := 1; attempt <= MaxOrderNotFoundAttempts; attempt++ {
		details, err = GetOrder(orderID, printInfo)
		if err == nil || !orderNotFoundPattern.MatchString(err.Error()) {
			return details, err
		}
		if attempt < MaxOrderNotFoundAttempts {
			sleepDur := time.Duration(200+rand.Intn(1301)) * time.Millisecond // 0.2s..1.5s
			time.Sleep(sleepDur)
		}
	}
	return details, err
}

// MaxFillPollAttempts caps how many times AwaitFill polls GetOrder before giving up.
const MaxFillPollAttempts = 5

// AwaitFill polls GetOrder every GapBetweenTwoCallsInSeconds until the order
// reaches a terminal state (filled, cancelled, rejected, replaced) or
// MaxFillPollAttempts is reached, then returns the last known OrderDetails
// (which may report partially_filled, or no fill at all).
func AwaitFill(orderID string, printInfo bool) (OrderDetails, error) {
	var details OrderDetails
	var err error
	for attempt := 0; attempt < MaxFillPollAttempts; attempt++ {
		details, err = getOrderWithRetry(orderID, printInfo)
		if err != nil {
			return details, err
		}

		switch details.Data.Status {
		case "filled", "cancelled", "rejected", "replaced":
			return details, nil
		}

		if attempt < MaxFillPollAttempts-1 {
			time.Sleep(GapBetweenTwoCallsInSeconds * time.Second)
		}
	}
	return details, nil
}
