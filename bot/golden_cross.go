package bot

import (
	"fmt"
)

// Action est l'enum des actions possibles.
type Action string

const (
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
	ActionWait Action = "wait"

	// 10% du capital à risque sur chaque achat.
	//
	// capital * riskPercent = argent que tu acceptes de perdre sur un trade.
	BuyRiskPercent = 0.1

	// 10% du capital réservé pour couvrir les commissions Revolut (non disponibles à l'avance via l'API).
	FeeReservePercent = 0.10
)

type GoldenCrossSuggestion struct {
	// Action: "buy", "sell" ou "wait"
	Action Action `json:"action"`

	// Quantity: quantité à acheter/vendre (0 si wait)
	QuantityToBuyOrSell float64 `json:"quantity"`
}

type GoldenCrossParams struct {
	// moyenne mobile 50 actuelle
	ma50Current float64

	// moyenne mobile 50 précédente
	ma50Prev float64

	// moyenne mobile 200 actuelle
	ma200Current float64

	// moyenne mobile 200 précédente
	ma200Prev float64

	// prix actuel de l'actif
	currPriceOfAsset float64

	// quantité actuelle de l'actif
	currQuantityOfAsset float64

	// solde actuel en USD
	availableUsd float64
}

// NextGoldenCrossSuggestion retourne le signal d'achat/vente/atte
// basé uniquement sur le Golden Cross (MA50 vs MA200).
//
// Entrées :
//   - ma50Current: moyenne mobile 50 actuelle
//   - ma50Prev:    moyenne mobile 50 précédente
//   - ma200Current: moyenne mobile 200 actuelle
//   - ma200Prev:   moyenne mobile 200 précédente
//   - currPriceOfAsset: prix actuel de l'actif
//   - currQuantityOfAsset: quantité actuelle de l'actif, en cas de vente l'algo suggère pour l'instant de votre cette quantité (vendre tout)
//   - availableUsd: solde actuel en USD, utilisé pour le calcul de la quantité à acheter
//   - printInfo: afficher les informations de débogage
//
// Sortie :
//   - Action: "buy", "sell" ou "wait"
//   - Quantity: quantité à acheter/vendre (0 si wait)
func NextGoldenCrossSuggestion(params GoldenCrossParams, printInfo bool) GoldenCrossSuggestion {
	result := GoldenCrossSuggestion{
		Action:              ActionWait,
		QuantityToBuyOrSell: 0,
	}

	// Ancienne position relative
	anciennePos := 0 // 1 = MA50 > MA200, -1 = MA50 < MA200
	if params.ma50Prev > params.ma200Prev {
		anciennePos = 1
		if printInfo {
			fmt.Println("Previous position: MA50 > MA200")
		}
	} else {
		anciennePos = -1
		if printInfo {
			fmt.Println("Previous position: MA50 < MA200")
		}
	}

	// Nouvelle position relative
	nouvellePos := 0
	if params.ma50Current > params.ma200Current {
		nouvellePos = 1
		if printInfo {
			fmt.Println("Current position: MA50 > MA200")
		}
	} else {
		nouvellePos = -1
		if printInfo {
			fmt.Println("Current position: MA50 < MA200")
		}
	}

	// Pas de croisement → on attend
	if anciennePos == nouvellePos {
		if printInfo {
			fmt.Println("No crossover detected. Waiting...")
		}
		return result
	}

	// Golden Cross: MA50 passe de < à > MA200 → achat
	if anciennePos == -1 && nouvellePos == 1 {
		if printInfo {
			fmt.Println("Golden Cross detected. Buying...")
		}
		result.Action = ActionBuy

		currentPrice := params.currPriceOfAsset
		stopLossPercent := 0.10
		stopLoss := currentPrice * (1 - stopLossPercent)

		capitalAfterFeeReserve := params.availableUsd * (1 - FeeReservePercent)
		quantityToBuy := calculateEntryQuantity(capitalAfterFeeReserve, BuyRiskPercent, currentPrice, stopLoss)
		result.QuantityToBuyOrSell = quantityToBuy

		if printInfo {
			fmt.Printf("Calculated quantity to buy based on risk management: %.4f units\n", quantityToBuy)
		}

		return result
	}

	// Death Cross: MA50 passe de > à < MA200 → vente
	if anciennePos == 1 && nouvellePos == -1 {
		result.Action = ActionSell
		result.QuantityToBuyOrSell = params.currQuantityOfAsset
		if printInfo {
			fmt.Println("Death Cross detected. Selling...")
		}

		return result
	}

	return result
}
