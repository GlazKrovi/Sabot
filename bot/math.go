package bot

import "fmt"

func averagePrice(prices []float64) float64 {
	if len(prices) == 0 {
		return 0
	}
	sum := 0.0
	for _, price := range prices {
		sum += price
	}
	return sum / float64(len(prices))
}

func averagePriceOn50(prices []float64) (float64, error) {
	if len(prices) == 50 {
		return averagePrice(prices), nil
	}
	return 0, fmt.Errorf("MA50 requires exactly 50 data points")
}

func averagePriceOn200(prices []float64) (float64, error) {
	if len(prices) == 200 {
		return averagePrice(prices), nil
	}
	return 0, fmt.Errorf("MA200 requires exactly 200 data points")
}
