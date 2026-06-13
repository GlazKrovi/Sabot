package bot

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Profit and Loss
// aka ~ Gain latent
type SessionPnL struct {
	Realized float64       `json:"realized"` // Profit and Loss from closed positions (i.e. from sell orders)
	Fees     float64       `json:"fees"`     // Total fees paid across all trades, in USD
	Trades   []TradeRecord `json:"trades"`   // History of trades executed
}

// TradeRecord represents a single buy or sell order executed by the bot.
type TradeRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Symbol    string    `json:"symbol"`     // e.g., "ETH-USD"
	Side      string    `json:"side"`       // "buy" or "sell"
	Quantity  float64   `json:"quantity"`   // amount of crypto bought/sold
	AmountUsd float64   `json:"amount_usd"` // USD spent (buy) or received (sell)
	Fees      float64   `json:"fees"`       // fees paid for this trade, in USD
}

func ImportPnLFromJson(path string) (*SessionPnL, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var pnl SessionPnL
	err = json.Unmarshal(data, &pnl)
	if err != nil {
		return nil, err
	}

	return &pnl, nil
}

// SaveToJson writes the SessionPnL structure to a JSON file.
func (p *SessionPnL) SaveToJson(filePath string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// Addition adds the values of another SessionPnL to the current one.
func (p *SessionPnL) Addition(pnl SessionPnL) {
	p.Realized += pnl.Realized
	p.Fees += pnl.Fees
	p.Trades = append(p.Trades, pnl.Trades...)
}

// SharedPnL guards a SessionPnL accumulator shared across goroutines.
type SharedPnL struct {
	mu  sync.Mutex
	pnl SessionPnL
}

// NewSharedPnL creates a SharedPnL initialized with the given starting value
// (e.g. loaded from a previous run's pnl_history.json).
func NewSharedPnL(initial SessionPnL) *SharedPnL {
	return &SharedPnL{pnl: initial}
}

// Add merges delta into the shared PnL accumulator.
func (s *SharedPnL) Add(delta SessionPnL) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pnl.Addition(delta)
}

// Save persists the current shared PnL accumulator to filePath.
func (s *SharedPnL) Save(filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pnl.SaveToJson(filePath)
}

// Realized returns the cumulative realized PnL.
func (s *SharedPnL) Realized() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pnl.Realized
}
