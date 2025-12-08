package main

import (
	"fmt"
	"margraf/trading"
	"time"
)

// Real backtest with actual stock data
func main() {
	fmt.Println("================================================================================")
	fmt.Println("REAL DATA BACKTEST - Using Yahoo Finance Historical Data")
	fmt.Println("================================================================================")
	fmt.Println()

	fetcher := trading.NewHistoricalDataFetcher()

	// Test with well-known correlated stocks
	// Example: Banks tend to move together
	ticker1 := "JPM"  // JPMorgan Chase
	ticker2 := "BAC"  // Bank of America

	fmt.Printf("Testing correlation between %s and %s\n", ticker1, ticker2)
	fmt.Println()

	// Fetch 1 year of data
	endDate := time.Now()
	startDate := endDate.AddDate(-1, 0, 0)

	fmt.Printf("Fetching historical data from %s to %s...\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	fmt.Println()

	// Fetch data
	fmt.Printf("Fetching %s...\n", ticker1)
	prices1, err := fetcher.FetchYahooHistoricalData(ticker1, startDate, endDate)
	if err != nil {
		fmt.Printf("Error fetching %s: %v\n", ticker1, err)
		return
	}
	fmt.Printf("  Success: %d data points\n", len(prices1))

	fmt.Printf("Fetching %s...\n", ticker2)
	prices2, err := fetcher.FetchYahooHistoricalData(ticker2, startDate, endDate)
	if err != nil {
		fmt.Printf("Error fetching %s: %v\n", ticker2, err)
		return
	}
	fmt.Printf("  Success: %d data points\n", len(prices2))
	fmt.Println()

	// Calculate correlation
	corr, err := trading.CalculateCorrelation(prices1, prices2)
	if err != nil {
		fmt.Printf("Error calculating correlation: %v\n", err)
		return
	}

	fmt.Printf("Correlation: %.4f\n", corr)
	fmt.Println()

	if corr < 0.5 {
		fmt.Println("Warning: Correlation is low. Pairs trading works best with correlation > 0.7")
		fmt.Println("Continuing anyway for demonstration...")
		fmt.Println()
	}

	// Create correlation pair
	pair := trading.CorrelationPair{
		Asset1:      ticker1,
		Asset2:      ticker2,
		Ticker1:     ticker1,
		Ticker2:     ticker2,
		Correlation: corr,
		GraphDistance: 1,
		HasDirectEdge: false,
		EdgeWeight:    0,
	}

	// Create strategy
	fmt.Println("Initializing pairs trading strategy...")
	strategy := trading.NewPairsTradingStrategy(
		pair,
		2.0,  // Entry threshold
		0.5,  // Exit threshold
		0.05, // Stop loss 5%
		20,   // Lookback window
	)

	// Create backtester
	backtester := trading.NewBacktester(
		100000, // $100k initial capital
		10000,  // $10k per trade
		0.001,  // 0.1% commission
	)

	fmt.Println("Running backtest...")
	fmt.Println()

	// Run backtest
	result, err := backtester.RunBacktest(strategy, prices1, prices2)
	if err != nil {
		fmt.Printf("Error running backtest: %v\n", err)
		return
	}

	// Print results
	result.PrintReport()
}
