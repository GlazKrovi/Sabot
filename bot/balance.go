package bot

import (
	"errors"
	"fmt"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"
)

type Balance struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
	Total     string `json:"total"`
}

func (b Balance) Verbose() string {
	// sometimes Locked is empty, so we calculate it if needed
	total, err := strconv.ParseFloat(b.Total, 64)
	if err != nil {
		total = 0.0 // If parsing fails, default to 0
	}

	available, err := strconv.ParseFloat(b.Available, 64)
	if err != nil {
		available = 0.0 // If parsing fails, default to 0
	}

	locked := total - available
	return fmt.Sprintf("%-8s │ Available: %15s │Locked: %15s",
		b.Currency,
		b.Available,
		fmt.Sprintf("%.2f", locked),
	)
}

func NonEmpties(b []Balance) []Balance {
	var nonEmptyBalances []Balance
	for _, balance := range b {
		total, err := strconv.ParseFloat(balance.Total, 64)
		if err != nil {
			continue // Skip if parsing fails
		}
		if math.Ceil(total) > 0 {
			nonEmptyBalances = append(nonEmptyBalances, balance)
		}
	}
	return nonEmptyBalances
}

func printBalancesTab(logger *log.Logger, balances []Balance) {
	// Tab of available balances
	logger.Println("                                           BALANCES")
	for _, b := range balances {
		logger.Println(b.Verbose())
		logger.Println(strings.Repeat("-", 100))
	}
}

// FetchBalances fetches the account's non-empty balances from Revolut X.
// It performs a real authenticated request, so it can also be used as a
// lightweight check that the configured API key works and is authorized.
func FetchBalances(printInfo bool) ([]Balance, error) {
	balances, err := contactApi[[]Balance](
		"GET",
		"https://revx.revolut.com/api/1.0/balances",
		map[string]string{},
		printInfo,
	)
	if err != nil {
		return make([]Balance, 0), err
	}

	// Purge empty ones (we cannot deal with monney we don't have)
	balances = NonEmpties(balances)

	return balances, nil
}

// LogBalancesAfterOrder fetches the current USD balance and the crypto balance
// of every traded symbol, then prints them as a table. Intended to be called
// after a successful buy/sell order, regardless of printInfo.
func LogBalancesAfterOrder(logger *log.Logger, cryptos []string) {
	balances, err := FetchBalances(false)
	if err != nil {
		logger.Printf("Error fetching balances: %v\n", err)
		return
	}

	balancesToPrint := []Balance{}
	if usdBalance, err := balanceOfCurrency(balances, "USD"); usdBalance != nil && !errors.Is(err, ErrBalanceNotFound{}) {
		balancesToPrint = append(balancesToPrint, *usdBalance)
	}
	for _, symbol := range cryptos {
		ticker := symbol
		if idx := strings.Index(symbol, "-"); idx > 0 {
			ticker = symbol[:idx]
		}
		if cryptoBalance, err := balanceOfCurrency(balances, ticker); cryptoBalance != nil && !errors.Is(err, ErrBalanceNotFound{}) {
			balancesToPrint = append(balancesToPrint, *cryptoBalance)
		}
	}

	printBalancesTab(logger, balancesToPrint)
}

type ErrBalanceNotFound struct {
	Currency string
}

func (e ErrBalanceNotFound) Error() string {
	return fmt.Sprintf("Balance not found for currency '%s'", e.Currency)
}

func balanceOfCurrency(balances []Balance, currency string) (*Balance, error) {
	indexOfCurrencyBalance := slices.IndexFunc(balances, func(balance Balance) bool {
		return balance.Currency == currency
	})
	if indexOfCurrencyBalance == -1 {
		return nil, ErrBalanceNotFound{Currency: currency}
	}

	return &balances[indexOfCurrencyBalance], nil
}
