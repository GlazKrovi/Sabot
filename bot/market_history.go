package bot

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Trade represents a single trade from the public market history endpoint.
type Trade struct {
	Id        string
	Price     string
	Quantity  string
	Side      string
	Timestamp string
}

// custom unmarshal to be tolerant to different field names used by the API
func (t *Trade) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// helpers
	getString := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok && v != nil {
				switch x := v.(type) {
				case string:
					return x
				case float64:
					return fmt.Sprintf("%v", x)
				default:
					return fmt.Sprintf("%v", x)
				}
			}
		}
		return ""
	}

	t.Id = getString("tid", "id")
	t.Price = getString("p", "price", "unit_price")
	t.Quantity = getString("q", "quantity")
	t.Side = getString("s", "side")
	t.Timestamp = getString("pdt", "timestamp", "time")
	return nil
}

type TradesResponse struct {
	Data     []Trade `json:"data"`
	Metadata struct {
		Timestamp any `json:"timestamp"`
	} `json:"metadata"`
}

// Fetch recent trades (market history) for a given trading pair using the
// Revolut "Get all public trades" endpoint. The `limit` parameter controls
// how many trades to fetch (e.g., 400). Optionally, start_date and end_date (RFC3339 or ms/seconds since epoch) can be provided.
func fetchMarketTrades(symbol string, limit int, startDate int64, endDate int64, printInfo bool) (TradesResponse, error) {
	var resp TradesResponse

	// Build query params
	params := []string{fmt.Sprintf("limit=%d", limit)}
	if startDate != 0 {
		params = append(params, fmt.Sprintf("start_date=%d", startDate))
	}
	if endDate != 0 {
		params = append(params, fmt.Sprintf("end_date=%d", endDate))
	}
	url := fmt.Sprintf("https://revx.revolut.com/api/1.0/trades/all/%s?%s", symbol, strings.Join(params, "&"))

	r, err := contactApi[TradesResponse]("GET", url, map[string]string{}, printInfo)
	if err != nil {
		return resp, err
	}
	return r, nil
}

func (tr TradesResponse) Verbose() string {
	var out strings.Builder
	// format metadata timestamp which can be string or number
	var headerTime string
	switch v := tr.Metadata.Timestamp.(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			headerTime = parsed.Format("2006-01-02 15:04:05")
		} else if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			if ts > 1e11 {
				headerTime = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
			} else {
				headerTime = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
			}
		} else {
			headerTime = v
		}
	case float64:
		ts := int64(v)
		if ts > 1e11 {
			headerTime = time.UnixMilli(ts).Format("2006-01-02 15:04:05")
		} else {
			headerTime = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
		}
	default:
		headerTime = fmt.Sprintf("%v", v)
	}

	out.WriteString(fmt.Sprintf("MARKET TRADES (at %s)\n", headerTime))
	for _, trade := range tr.Data {
		out.WriteString(fmt.Sprintf("%s Price: %s Qty: %s Time: %s\n", trade.Side, trade.Price, trade.Quantity, trade.Timestamp))
	}
	return out.String()
}
