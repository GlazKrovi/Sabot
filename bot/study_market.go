package bot

import (
	"fmt"
	"strconv"
	"time"
)

const (
	NbTradesNeeded = 200
)

// Return the average price on these 200 trades,
// and the average price on the 50 most recent trades (the side of the transaction doesn't matter)
func StudyNext200Transactions(symbol string, printInfo bool) (float64, float64, error) {
	// Get 200 trades
	trades := make(map[string]string) // avoid duplicates, we use the trade ID as key
	nbTrades := 0
	remainingNeeds := NbTradesNeeded - nbTrades
	max_date := time.Now().UnixMilli()
	min_date := max_date - 24*60*60*1000 // 24 hours ago
	for nbTrades < StrategyTransactionNeeds {
		if printInfo {
			fmt.Print("Fetching market trades...\n")
		}

		tradesResp, err := fetchMarketTrades(symbol, remainingNeeds, min_date, max_date, printInfo)
		if err != nil {
			panic(err)
		}

		for _, tr := range tradesResp.Data {
			trades[tr.Id] = tr.Price

			if printInfo {
				fmt.Printf("Fetched trade ID %s with price %s\n", tr.Id, tr.Price)
			}
		}

		// It's very unlikely that we'll need multiple iterations, but just to be safe, on new or unfamiliar peers
		nbTrades = len(trades)
		remainingNeeds = StrategyTransactionNeeds - nbTrades
		min_date = max_date - 24*60*60*1000 // take +24h ago, each time, to avoid getting the same trades again
	}
	if printInfo {
		fmt.Printf("Successfully fetched %d trades (unique) for %s\n", nbTrades, symbol)
	}

	// Calculate mobile averages of prices (the side of the transaction doesn't matter)
	most200RecentTransactions := make([]float64, 0, 200)
	for _, tr := range trades {
		price, err := strconv.ParseFloat(tr, 64)
		if err != nil {
			return 0, 0, err
		}
		most200RecentTransactions = append(most200RecentTransactions, price)
	}

	// The 50 most recent transactions are the 50 last of the 200 most recent transactions
	most50RecentTransactions := most200RecentTransactions[:50]

	averagePriceOn200, err := averagePriceOn200(most200RecentTransactions)
	if err != nil {
		return 0, 0, err
	}
	if printInfo {
		fmt.Printf("Average Price on 200 transactions: %.4f\n", averagePriceOn200)
	}

	averagePriceOn50, err := averagePriceOn50(most50RecentTransactions)
	if err != nil {
		return 0, 0, err
	}
	if printInfo {
		fmt.Printf("Average Price on 50 transactions: %.4f\n", averagePriceOn50)
	}

	return averagePriceOn200, averagePriceOn50, nil
}

// Quantity to buy, according to the risks and stop loss (the "max" we want to lose if the trade goes wrong)
// In a simple strategy, we you sell, you sell 100% of your position. So, the quantity to sell is the quantity you have in your balance.
// Indépendant de la stratégie golden cross
func calculateEntryQuantity(capital float64, riskPercent float64, price float64, stopLoss float64) float64 {
	montantRisque := capital * riskPercent
	distanceStopLoss := price - stopLoss
	if distanceStopLoss <= 0 {
		return 0
	}
	quantite := montantRisque / distanceStopLoss
	return quantite
}
