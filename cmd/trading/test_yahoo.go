package main

import (
	"fmt"
	"margraf/trading"
	"time"
)

// Test program to verify Yahoo Finance data fetching works
func main() {
	fmt.Println("Testing Yahoo Finance Data Fetcher...")
	fmt.Println("================================================================================")

	fetcher := trading.NewHistoricalDataFetcher()

	// Test with well-known US stocks
	testTickers := []string{"AAPL", "MSFT", "GOOGL"}

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30) // Last 30 days

	for _, ticker := range testTickers {
		fmt.Printf("\nFetching data for %s...\n", ticker)
		prices, err := fetcher.FetchYahooHistoricalData(ticker, startDate, endDate)

		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
		} else {
			fmt.Printf("  SUCCESS: Fetched %d data points\n", len(prices))
			if len(prices) > 0 {
				fmt.Printf("  First: %s - $%.2f\n", time.Unix(prices[0].Timestamp, 0).Format("2006-01-02"), prices[0].Price)
				fmt.Printf("  Last:  %s - $%.2f\n", time.Unix(prices[len(prices)-1].Timestamp, 0).Format("2006-01-02"), prices[len(prices)-1].Price)
			}
		}
	}

	fmt.Println("\n================================================================================")
}
