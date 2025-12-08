package main

import (
	"flag"
	"fmt"
	"margraf/graph"
	"margraf/trading"
	"os"
	"time"
)

func main() {
	// Command line flags
	graphFile := flag.String("graph", "margraf_graph.json", "Path to graph JSON file")
	mode := flag.String("mode", "analyze", "Mode: analyze, backtest, mock")
	minCorrelation := flag.Float64("min-correlation", 0.7, "Minimum correlation threshold")
	daysBack := flag.Int("days", 365, "Number of days for historical data")
	initialCapital := flag.Float64("capital", 100000, "Initial capital for backtesting")
	positionSize := flag.Float64("position", 10000, "Position size per trade")
	entryThreshold := flag.Float64("entry", 2.0, "Z-score entry threshold")
	exitThreshold := flag.Float64("exit", 0.5, "Z-score exit threshold")
	stopLoss := flag.Float64("stoploss", 0.05, "Stop loss percentage")
	lookback := flag.Int("lookback", 20, "Lookback window for strategy")

	flag.Parse()

	fmt.Println("================================================================================")
	fmt.Println("MARGRAF CORRELATION TRADING SYSTEM")
	fmt.Println("================================================================================")
	fmt.Println()

	// Load graph
	fmt.Printf("Loading graph from %s...\n", *graphFile)
	g, err := graph.Load(*graphFile)
	if err != nil {
		fmt.Printf("Error loading graph: %v\n", err)
		fmt.Println("Creating new graph...")
		g = graph.NewGraph()
	} else {
		fmt.Printf("Graph loaded: %d nodes, %d edges\n\n", len(g.Nodes), len(g.Edges))
	}

	switch *mode {
	case "analyze":
		analyzeMode(g, *minCorrelation, *daysBack)
	case "backtest":
		backtestMode(g, *minCorrelation, *daysBack, *initialCapital, *positionSize, *entryThreshold, *exitThreshold, *stopLoss, *lookback)
	case "mock":
		mockBacktestMode(*minCorrelation, *initialCapital, *positionSize, *entryThreshold, *exitThreshold, *stopLoss, *lookback)
	default:
		fmt.Printf("Unknown mode: %s\n", *mode)
		flag.Usage()
		os.Exit(1)
	}
}

func analyzeMode(g *graph.Graph, minCorrelation float64, daysBack int) {
	fmt.Println("MODE: CORRELATION ANALYSIS")
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println()

	// Find all corporations with tickers
	var tickerNodes []*graph.Node
	for _, node := range g.Nodes {
		if node.Type == graph.NodeTypeCorporation && node.Ticker != "" {
			tickerNodes = append(tickerNodes, node)
		}
	}

	fmt.Printf("Found %d corporations with ticker symbols\n", len(tickerNodes))

	if len(tickerNodes) < 2 {
		fmt.Println("Not enough assets with tickers for correlation analysis")
		fmt.Println("\nTip: The graph needs at least 2 corporations with ticker symbols.")
		fmt.Println("Try running with -mode=mock for a demonstration with synthetic data.")
		return
	}

	// Fetch historical data
	fmt.Printf("Fetching %d days of historical data...\n", daysBack)
	fetcher := trading.NewHistoricalDataFetcher()

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -daysBack)

	priceHistories := make(map[string]*trading.AssetPriceHistory)

	for _, node := range tickerNodes {
		fmt.Printf("  Fetching %s (%s)...\n", node.Name, node.Ticker)
		prices, err := fetcher.FetchYahooHistoricalData(node.Ticker, startDate, endDate)
		if err != nil {
			fmt.Printf("    Warning: %v\n", err)
			continue
		}

		priceHistories[node.ID] = &trading.AssetPriceHistory{
			AssetID: node.ID,
			Ticker:  node.Ticker,
			Prices:  prices,
		}
		fmt.Printf("    Success: %d data points\n", len(prices))
	}

	if len(priceHistories) < 2 {
		fmt.Println("\nError: Failed to fetch sufficient historical data")
		fmt.Println("Try running with -mode=mock for a demonstration with synthetic data.")
		return
	}

	// Analyze correlations
	fmt.Println("\nAnalyzing correlations...")
	analyzer := trading.NewCorrelationAnalyzer(g)

	pairs, err := analyzer.FindCorrelatedPairs(priceHistories, minCorrelation)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("\nFound %d correlated pairs (correlation >= %.2f)\n\n", len(pairs), minCorrelation)

	// Print top pairs
	displayLimit := 10
	if len(pairs) < displayLimit {
		displayLimit = len(pairs)
	}

	fmt.Println("TOP CORRELATED PAIRS:")
	fmt.Println("--------------------------------------------------------------------------------")

	for i := 0; i < displayLimit; i++ {
		pair := pairs[i]
		fmt.Printf("\n%d. %s (%s) <-> %s (%s)\n", i+1, pair.Asset1, pair.Ticker1, pair.Asset2, pair.Ticker2)
		fmt.Printf("   Correlation:    %.4f\n", pair.Correlation)
		fmt.Printf("   Graph Distance: %d\n", pair.GraphDistance)
		fmt.Printf("   Direct Edge:    %v\n", pair.HasDirectEdge)
		if pair.HasDirectEdge {
			fmt.Printf("   Edge Weight:    %.4f\n", pair.EdgeWeight)
		}
	}

	fmt.Println("\n================================================================================")
	fmt.Println("Use -mode=backtest to run a backtest on these pairs")
	fmt.Println("================================================================================")
}

func backtestMode(g *graph.Graph, minCorrelation float64, daysBack int, initialCapital, positionSize, entryThreshold, exitThreshold, stopLoss float64, lookback int) {
	fmt.Println("MODE: BACKTEST")
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println()

	// Find all corporations with tickers
	var tickerNodes []*graph.Node
	for _, node := range g.Nodes {
		if node.Type == graph.NodeTypeCorporation && node.Ticker != "" {
			tickerNodes = append(tickerNodes, node)
		}
	}

	if len(tickerNodes) < 2 {
		fmt.Println("Not enough assets with tickers for backtesting")
		return
	}

	// Fetch historical data
	fmt.Printf("Fetching %d days of historical data...\n", daysBack)
	fetcher := trading.NewHistoricalDataFetcher()

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -daysBack)

	priceHistories := make(map[string]*trading.AssetPriceHistory)

	for _, node := range tickerNodes {
		fmt.Printf("  Fetching %s (%s)...\n", node.Name, node.Ticker)
		prices, err := fetcher.FetchYahooHistoricalData(node.Ticker, startDate, endDate)
		if err != nil {
			fmt.Printf("    Warning: %v\n", err)
			continue
		}

		priceHistories[node.ID] = &trading.AssetPriceHistory{
			AssetID: node.ID,
			Ticker:  node.Ticker,
			Prices:  prices,
		}
	}

	if len(priceHistories) < 2 {
		fmt.Println("\nError: Failed to fetch sufficient historical data")
		return
	}

	// Find correlated pairs
	analyzer := trading.NewCorrelationAnalyzer(g)
	pairs, err := analyzer.FindCorrelatedPairs(priceHistories, minCorrelation)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(pairs) == 0 {
		fmt.Println("No correlated pairs found")
		return
	}

	// Backtest the top pair
	fmt.Printf("\nBacktesting top pair: %s <-> %s\n", pairs[0].Ticker1, pairs[0].Ticker2)

	strategy := trading.NewPairsTradingStrategy(
		pairs[0],
		entryThreshold,
		exitThreshold,
		stopLoss,
		lookback,
	)

	backtester := trading.NewBacktester(initialCapital, positionSize, 0.001)

	hist1 := priceHistories[pairs[0].Asset1]
	hist2 := priceHistories[pairs[0].Asset2]

	result, err := backtester.RunBacktest(strategy, hist1.Prices, hist2.Prices)
	if err != nil {
		fmt.Printf("Error running backtest: %v\n", err)
		return
	}

	result.PrintReport()
}

func mockBacktestMode(minCorrelation float64, initialCapital, positionSize, entryThreshold, exitThreshold, stopLoss float64, lookback int) {
	fmt.Println("MODE: MOCK BACKTEST (Synthetic Data)")
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println()

	// Generate mock correlated data
	fmt.Println("Generating synthetic correlated price data...")

	prices1, prices2 := trading.GenerateMockHistoricalData("MOCK1", "MOCK2", 0.85, 365)

	fmt.Printf("Generated %d days of data with 0.85 target correlation\n\n", len(prices1))

	// Calculate actual correlation
	actualCorr, _ := trading.CalculateCorrelation(prices1, prices2)
	fmt.Printf("Actual correlation: %.4f\n\n", actualCorr)

	// Create a mock pair
	pair := trading.CorrelationPair{
		Asset1:      "mock_asset_1",
		Asset2:      "mock_asset_2",
		Ticker1:     "MOCK1",
		Ticker2:     "MOCK2",
		Correlation: actualCorr,
		GraphDistance: 1,
		HasDirectEdge: true,
		EdgeWeight:    0.8,
	}

	// Run backtest
	fmt.Println("Running backtest...")

	strategy := trading.NewPairsTradingStrategy(
		pair,
		entryThreshold,
		exitThreshold,
		stopLoss,
		lookback,
	)

	backtester := trading.NewBacktester(initialCapital, positionSize, 0.001)

	result, err := backtester.RunBacktest(strategy, prices1, prices2)
	if err != nil {
		fmt.Printf("Error running backtest: %v\n", err)
		return
	}

	result.PrintReport()

	fmt.Println("\nNOTE: This is a demonstration using synthetic data.")
	fmt.Println("For real backtesting, use -mode=backtest with actual market data.")
}
