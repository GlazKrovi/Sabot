package bot

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	GapBetweenTwoCallsInSeconds = 3

	// Number of transactions we want to fetch for the Golden cross strategy
	StrategyTransactionNeeds = 200
)

// Sentinel errors for quantity validation
var (
	ErrNotEnoughMoney = fmt.Errorf("Not enough money to buy for now")
)

// SharedUSD represents a shared USD balance used by multiple goroutines.
type SharedUSD struct {
	mu        sync.Mutex
	Available float64
}

func (s *SharedUSD) Get() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Available
}

// Reserve tries to reserve up to amount USD and returns the actual reserved amount.
func (s *SharedUSD) Reserve(amount float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if amount <= 0 {
		return 0
	}
	if s.Available >= amount {
		s.Available -= amount
		return amount
	}
	// partial reservation
	reserved := s.Available
	s.Available = 0
	return reserved
}

func (s *SharedUSD) Add(amount float64) {
	s.mu.Lock()
	s.Available += amount
	s.mu.Unlock()
}

// SharedCryptoUnits tracks the most recently observed available crypto balance
// for each traded symbol's ticker, shared across goroutines.
type SharedCryptoUnits struct {
	mu    sync.Mutex
	Units map[string]float64
}

func (s *SharedCryptoUnits) Set(ticker string, amount float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Units[ticker] = amount
}

// AllZero returns true if no traded symbol currently holds any crypto units.
func (s *SharedCryptoUnits) AllZero() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, amount := range s.Units {
		if amount > 0 {
			return false
		}
	}
	return true
}

// Run starts the trading bot for the given cryptocurrency symbols (e.g., "ETH" or "DOT-USD").
// Pass "-v" or "--verbose" anywhere in args to enable detailed (printInfo) logging.
// It blocks forever, running one goroutine per symbol.
func Run(args []string) {
	// Separate the -v/--verbose flag from the cryptocurrency symbol arguments
	var cryptos []string
	printInfo := false
	for _, arg := range args {
		if arg == "-v" || arg == "--verbose" {
			printInfo = true
			continue
		}
		cryptos = append(cryptos, arg)
	}
	if len(cryptos) == 0 {
		fmt.Println("Please provide at least one cryptocurrency symbol as an argument (e.g., ETH).")
		return
	}
	for i, sym := range cryptos {
		if !strings.Contains(sym, "-") {
			cryptos[i] = strings.ToUpper(sym) + "-USD"
		}
	}
	fmt.Printf("Trading on the following cryptocurrencies: %v\n", cryptos)

	// Load persisted PnL history (cumulative since first launch)
	pnlFromDisk, err := ImportPnLFromJson("pnl_history.json")
	if err != nil {
		fmt.Println("No previous PnL history found, starting fresh.")
		pnlFromDisk = &SessionPnL{}
	}
	sharedPnL := NewSharedPnL(*pnlFromDisk)

	// Fetch initial balances once and share USD among goroutines
	balancesInit, err := FetchBalances(printInfo)
	if err != nil {
		panic(err)
	}
	initUsd := 0.0
	if currUsdBalance, err2 := balanceOfCurrency(balancesInit, "USD"); currUsdBalance != nil && !errors.Is(err2, ErrBalanceNotFound{}) {
		if v, err3 := strconv.ParseFloat(currUsdBalance.Available, 64); err3 == nil {
			initUsd = v
		}
	}
	sharedUSD := &SharedUSD{Available: initUsd}

	// Track the most recently observed crypto balance for each traded symbol,
	// shared across goroutines, so that a goroutine with nothing to trade can
	// tell whether *any* symbol still holds crypto before giving up entirely.
	sharedCrypto := &SharedCryptoUnits{Units: make(map[string]float64)}
	for _, symbol := range cryptos {
		ticker := symbol
		if idx := strings.Index(symbol, "-"); idx > 0 {
			ticker = symbol[:idx]
		}
		if cryptoBalance, err := balanceOfCurrency(balancesInit, ticker); cryptoBalance != nil && !errors.Is(err, ErrBalanceNotFound{}) {
			if v, err3 := strconv.ParseFloat(cryptoBalance.Available, 64); err3 == nil {
				sharedCrypto.Units[ticker] = v
			}
		}
	}

	// Launch one goroutine per symbol, each using the shared USD pool
	for _, symbol := range cryptos {
		go func(sym string, sharedUSD *SharedUSD, sharedCrypto *SharedCryptoUnits, sharedPnL *SharedPnL) {
			logger := log.New(os.Stdout, fmt.Sprintf("[%s] ", sym), log.Ltime)
			logger.Printf("Studying market of %s for the first time...\n", sym)
			prevAveragePriceOn200, prevAveragePriceOn50, err := StudyNext200Transactions(sym, printInfo)
			if err != nil {
				panic(err)
			}
			nbCandleCalculated := 1 // for this crypto only

			for {
				// Get balances (change every iteration, because of sales and purchases)
				balances, err := FetchBalances(printInfo)
				if err != nil {
					panic(err)
				}

				// We use the shared USD pool as authoritative for concurrent trading
				availableUsd := sharedUSD.Get()

				// extract crypto ticker (e.g., ETH from "ETH-USD")
				ticker := sym
				if idx := strings.Index(sym, "-"); idx > 0 {
					ticker = sym[:idx]
				}
				cryptoBalance, err := balanceOfCurrency(balances, ticker)
				availableCryptoUnits := 0.0
				if cryptoBalance != nil && !errors.Is(err, ErrBalanceNotFound{}) {
					dot, err := strconv.ParseFloat(cryptoBalance.Available, 64)
					if err != nil {
						panic(err)
					}
					availableCryptoUnits = dot
				}
				sharedCrypto.Set(ticker, availableCryptoUnits)

				currentSuggestion := GoldenCrossSuggestion{Action: ActionWait, QuantityToBuyOrSell: 0}
				for currentSuggestion.Action == ActionWait {
					if printInfo {
						logger.Printf("--- Candle / Iteration %d ---\n", nbCandleCalculated)
					}

					// Sleep random 5..15 seconds
					sleepSec := 5 + rand.Intn(11) // 5..15 inclusive
					sleepDur := time.Duration(sleepSec) * time.Second
					if printInfo {
						logger.Printf("Sleeping %v before calculating the next candle\n", sleepDur)
					}
					time.Sleep(sleepDur)

					currAveragePriceOn200, currAveragePriceOn50, err := StudyNext200Transactions(sym, printInfo)
					if err != nil {
						panic(err)
					}
					nbCandleCalculated++

					// Current price — use recent market trades (most recent trade price)
					mostRecentTrade, err := fetchMarketTrades(sym, 1, 0, 0, printInfo) // get the most recent trade
					if err != nil {
						panic(err)
					}
					if len(mostRecentTrade.Data) == 0 {
						panic(fmt.Sprintf("no trades returned for symbol %s", sym))
					}
					currentPrice, err := strconv.ParseFloat(mostRecentTrade.Data[0].Price, 64) // Most recent trade price
					if err != nil {
						panic(err)
					}
					if printInfo {
						logger.Printf("Current price of %s: %.2f\n", sym, currentPrice)
					}

					// If we have no USD and no units of this symbol, this goroutine can't trade
					// right now. Only stop the whole program if every traded symbol is in the
					// same situation (no USD left and no crypto units anywhere to sell for more USD).
					if availableUsd <= 0 && availableCryptoUnits <= 0 {
						if sharedCrypto.AllZero() {
							logger.Printf("No available USD and no available crypto units for any traded symbol. So, the program will never do anything. Stopping.\n")
							os.Exit(1)
						}
						if printInfo {
							logger.Printf("No available USD and no available %s units for now. Waiting in case another symbol frees up USD.\n", ticker)
						}
					}

					// Calculate suggestion based on the golden cross strategy
					currentSuggestion = NextGoldenCrossSuggestion(
						GoldenCrossParams{
							ma50Current:         currAveragePriceOn50,
							ma50Prev:            prevAveragePriceOn50,
							ma200Current:        currAveragePriceOn200,
							ma200Prev:           prevAveragePriceOn200,
							currPriceOfAsset:    currentPrice,
							currQuantityOfAsset: availableCryptoUnits,
							availableUsd:        availableUsd,
						},
						printInfo,
					)

					if currentSuggestion.Action != ActionWait {
						if printInfo {
							logger.Printf("Golden Cross Suggestion: Action = %s, Quantity = %.2f\n", currentSuggestion.Action, currentSuggestion.QuantityToBuyOrSell)
						}
						// Received actionable suggestion, break loop and act!
						break
					}
					logger.Printf("Golden Cross suggests to wait.\n")

					prevAveragePriceOn200 = currAveragePriceOn200
					prevAveragePriceOn50 = currAveragePriceOn50
				}

				switch currentSuggestion.Action {
				case ActionBuy:
					// Recalculate current price just before placing the order to ensure accuracy
					freshTrade, err := fetchMarketTrades(sym, 1, 0, 0, printInfo)
					if err != nil {
						panic(err)
					}
					if len(freshTrade.Data) == 0 {
						panic(fmt.Sprintf("no trades returned for symbol %s before buy", sym))
					}
					currentPrice, err := strconv.ParseFloat(freshTrade.Data[0].Price, 64)
					if err != nil {
						panic(err)
					}
					if printInfo {
						logger.Printf("Current price on market: %.2f\n", currentPrice)
					}

					desiredUnits := currentSuggestion.QuantityToBuyOrSell
					if desiredUnits <= 0 {
						if printInfo {
							logger.Printf("Calculated quantity to buy is %.4f, which is not positive. Cannot place buy order.\n", desiredUnits)
						}
						continue
					}
					// Revolut checks balance against the limit price (ImmediateBuyPriceMultiplier * currentPrice),
					// not the fill price, so affordability and reservation must use that limit price.
					limitPrice := currentPrice * ImmediateBuyPriceMultiplier
					maxAffordableUnits := sharedUSD.Get() / limitPrice
					willBuyUnits := desiredUnits
					if maxAffordableUnits < willBuyUnits {
						willBuyUnits = maxAffordableUnits
					}
					if willBuyUnits <= 0 {
						if printInfo {
							logger.Printf("Not enough USD available to buy any units of %s.\n", sym)
						}
						continue
					}

					reserveUsd := willBuyUnits * limitPrice
					reserved := sharedUSD.Reserve(reserveUsd)
					if reserved <= 0 {
						if printInfo {
							logger.Printf("Failed to reserve USD for buy order of %s.\n", sym)
						}
						continue
					}
					actualUnits := reserved / limitPrice

					if printInfo {
						logger.Printf("Trying to buy %.4f units of %s at market price %.2f (limit %.2f, reserved %.2f USD)\n", actualUnits, sym, currentPrice, limitPrice, reserved)
					}
					placedOrder, err := Buy(sym, actualUnits, currentPrice, true, printInfo)
					if errors.Is(err, ErrOrderTooSmall) {
						if printInfo {
							logger.Printf("Skipping buy order: %v\n", err)
						}
						sharedUSD.Add(reserved)
						continue
					} else if err != nil {
						sharedUSD.Add(reserved)
						panic(err)
					}

					details, err := AwaitFill(placedOrder.Data.VenueOrderID, printInfo)
					if err != nil {
						sharedUSD.Add(reserved)
						panic(err)
					}

					filledQuantity, _ := strconv.ParseFloat(details.Data.FilledQuantity, 64)
					if filledQuantity <= 0 {
						if printInfo {
							logger.Printf("Buy order %s did not fill (status: %s). Releasing reserved USD.\n", details.Data.ID, details.Data.Status)
						}
						sharedUSD.Add(reserved)
						continue
					}

					spentUsd, _ := strconv.ParseFloat(details.Data.FilledAmount, 64)
					fee := 0.0
					if totalFee, err := strconv.ParseFloat(details.Data.TotalFee, 64); err == nil && totalFee > 0 {
						if details.Data.FeeCurrency == "USD" {
							fee = totalFee
						} else {
							logger.Printf("Warning: buy order fee of %.8f %s is not in USD, not deducted from USD balance.\n", totalFee, details.Data.FeeCurrency)
						}
					}

					// Return unused reservation: limit price was higher than the actual fill price + fee
					if unusedReservation := reserved - spentUsd - fee; unusedReservation > 0 {
						sharedUSD.Add(unusedReservation)
					}

					logger.Printf("Bought %.8f units of %s for %.2f USD (fee: %.4f USD, status: %s)\n", filledQuantity, sym, spentUsd, fee, details.Data.Status)
					LogBalancesAfterOrder(logger, cryptos)

					trade := []TradeRecord{{
						Timestamp: time.Now(),
						Symbol:    sym,
						Side:      "buy",
						Quantity:  filledQuantity,
						AmountUsd: spentUsd,
						Fees:      fee,
					}}
					sharedPnL.Add(SessionPnL{Realized: -spentUsd, Fees: fee, Trades: trade})

				case ActionSell:
					if availableCryptoUnits <= 0 {
						if printInfo {
							logger.Printf("No available units to sell. Cannot place sell order.\n")
						}
						continue
					}

					if printInfo {
						logger.Printf("Placing market sell order for %.8f units of %s (100%% of holdings)\n", availableCryptoUnits, sym)
					}
					placedOrder, err := Sell(sym, availableCryptoUnits, printInfo)
					if errors.Is(err, ErrOrderTooSmall) {
						if printInfo {
							logger.Printf("Skipping sell order: %v\n", err)
						}
						continue
					} else if err != nil {
						panic(err)
					}

					details, err := AwaitFill(placedOrder.Data.VenueOrderID, printInfo)
					if err != nil {
						panic(err)
					}

					filledQuantity, _ := strconv.ParseFloat(details.Data.FilledQuantity, 64)
					if filledQuantity <= 0 {
						if printInfo {
							logger.Printf("Sell order %s did not fill (status: %s).\n", details.Data.ID, details.Data.Status)
						}
						continue
					}

					proceeds, _ := strconv.ParseFloat(details.Data.FilledAmount, 64)
					fee := 0.0
					if totalFee, err := strconv.ParseFloat(details.Data.TotalFee, 64); err == nil && totalFee > 0 {
						if details.Data.FeeCurrency == "USD" {
							fee = totalFee
						} else {
							logger.Printf("Warning: sell order fee of %.8f %s is not in USD, not deducted from USD balance.\n", totalFee, details.Data.FeeCurrency)
						}
					}

					netProceeds := proceeds - fee
					sharedUSD.Add(netProceeds)

					logger.Printf("Sold %.8f units of %s for %.2f USD (fee: %.4f USD, status: %s)\n", filledQuantity, sym, proceeds, fee, details.Data.Status)
					LogBalancesAfterOrder(logger, cryptos)

					trade := []TradeRecord{{
						Timestamp: time.Now(),
						Symbol:    sym,
						Side:      "sell",
						Quantity:  filledQuantity,
						AmountUsd: proceeds,
						Fees:      fee,
					}}
					sharedPnL.Add(SessionPnL{Realized: netProceeds, Fees: fee, Trades: trade})
				default:
					continue
				}

				// Determines profit and loss
				if printInfo {
					logger.Printf("Cumulative realized PnL: %.2f USD\n", sharedPnL.Realized())
				}
				if err := sharedPnL.Save("pnl_history.json"); err != nil {
					logger.Printf("Error saving PnL history to JSON: %v\n", err)
				} else if printInfo {
					logger.Printf("PnL history saved to pnl_history.json\n")
				}
			}
		}(symbol, sharedUSD, sharedCrypto, sharedPnL)
	}

	// Keep main alive (goroutines run indefinitely)
	select {}
}
