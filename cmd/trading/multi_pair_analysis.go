package main

import (
	"fmt"
	"margraf/trading"
	"time"
)

// Analyze and backtest multiple pairs
func main() {
	fmt.Println("================================================================================")
	fmt.Println("MULTI-PAIR CORRELATION ANALYSIS & BACKTESTING")
	fmt.Println("================================================================================")
	fmt.Println()

	// Define pairs to test
	pairs := []struct {
		ticker1 string
		ticker2 string
		name1   string
		name2   string
	}{
		{"JPM", "BAC", "JPMorgan", "Bank of America"},
		{"KO", "PEP", "Coca-Cola", "Pepsi"},
		{"AAPL", "MSFT", "Apple", "Microsoft"},
		{"XOM", "CVX", "Exxon", "Chevron"},
		{"WMT", "TGT", "Walmart", "Target"},
	}

	fetcher := trading.NewHistoricalDataFetcher()
	endDate := time.Now()
	startDate := endDate.AddDate(-1, 0, 0) // 1 year

	fmt.Printf("Fetching 1 year of historical data (2024-12-02 to 2025-12-02)...\n")
	fmt.Println()

	var results []struct {
		pair        trading.CorrelationPair
		prices1     []trading.PricePoint
		prices2     []trading.PricePoint
		correlation float64
	}

	// Fetch data and calculate correlations
	for _, p := range pairs {
		fmt.Printf("Analyzing %s (%s) vs %s (%s)...\n", p.name1, p.ticker1, p.name2, p.ticker2)

		prices1, err1 := fetcher.FetchYahooHistoricalData(p.ticker1, startDate, endDate)
		prices2, err2 := fetcher.FetchYahooHistoricalData(p.ticker2, startDate, endDate)

		if err1 != nil || err2 != nil {
			fmt.Printf("  ERROR: Could not fetch data\n")
			continue
		}

		corr, err := trading.CalculateCorrelation(prices1, prices2)
		if err != nil {
			fmt.Printf("  ERROR: Could not calculate correlation\n")
			continue
		}

		fmt.Printf("  Correlation: %.4f (%d data points)\n", corr, len(prices1))

		pair := trading.CorrelationPair{
			Asset1:      p.ticker1,
			Asset2:      p.ticker2,
			Ticker1:     p.ticker1,
			Ticker2:     p.ticker2,
			Correlation: corr,
			GraphDistance: 1,
			HasDirectEdge: false,
			EdgeWeight:    0,
		}

		results = append(results, struct {
			pair        trading.CorrelationPair
			prices1     []trading.PricePoint
			prices2     []trading.PricePoint
			correlation float64
		}{pair, prices1, prices2, corr})
	}

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("CORRELATION SUMMARY")
	fmt.Println("================================================================================")
	fmt.Println()

	for i, r := range results {
		fmt.Printf("%d. %s <-> %s: %.4f\n", i+1, r.pair.Ticker1, r.pair.Ticker2, r.correlation)
	}

	// Find best pair
	bestIdx := 0
	bestCorr := 0.0
	for i, r := range results {
		if r.correlation > bestCorr {
			bestCorr = r.correlation
			bestIdx = i
		}
	}

	if len(results) == 0 {
		fmt.Println("\nNo valid pairs to backtest")
		return
	}

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Printf("BACKTESTING BEST PAIR: %s <-> %s (correlation: %.4f)\n",
		results[bestIdx].pair.Ticker1, results[bestIdx].pair.Ticker2, results[bestIdx].correlation)
	fmt.Println("================================================================================")
	fmt.Println()

	// Backtest best pair
	strategy := trading.NewPairsTradingStrategy(
		results[bestIdx].pair,
		2.0,  // Entry threshold
		0.5,  // Exit threshold
		0.05, // Stop loss
		20,   // Lookback
	)

	backtester := trading.NewBacktester(100000, 10000, 0.001)

	result, err := backtester.RunBacktest(strategy, results[bestIdx].prices1, results[bestIdx].prices2)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	result.PrintReport()

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("SUMMARY OF ALL PAIRS")
	fmt.Println("================================================================================")
	fmt.Println()

	// Quick backtest all pairs
	for i, r := range results {
		strategy := trading.NewPairsTradingStrategy(r.pair, 2.0, 0.5, 0.05, 20)
		backtester := trading.NewBacktester(100000, 10000, 0.001)

		result, err := backtester.RunBacktest(strategy, r.prices1, r.prices2)
		if err != nil {
			continue
		}

		fmt.Printf("%d. %s <-> %s\n", i+1, r.pair.Ticker1, r.pair.Ticker2)
		fmt.Printf("   Correlation:  %.4f\n", r.correlation)
		fmt.Printf("   Total Return: $%.2f (%.2f%%)\n", result.TotalReturn, result.TotalReturnPct)
		fmt.Printf("   Trades:       %d (Win Rate: %.1f%%)\n", result.TotalTrades, result.WinRate)
		fmt.Printf("   Sharpe:       %.2f\n", result.SharpeRatio)
		fmt.Printf("   Max Drawdown: %.2f%%\n", result.MaxDrawdown)
		fmt.Println()
	}
}
