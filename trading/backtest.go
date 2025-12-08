package trading

import (
	"fmt"
	"math"
	"time"
)

// Trade represents a completed trade
type Trade struct {
	EntryTime  int64
	ExitTime   int64
	Asset1     string
	Asset2     string
	Direction  string
	EntryPrice1 float64
	EntryPrice2 float64
	ExitPrice1  float64
	ExitPrice2  float64
	PnL         float64
	PnLPercent  float64
	Duration    time.Duration
}

// BacktestResult contains the results of a backtest
type BacktestResult struct {
	Strategy       string
	Pair           CorrelationPair
	StartDate      time.Time
	EndDate        time.Time
	InitialCapital float64
	FinalCapital   float64
	TotalReturn    float64
	TotalReturnPct float64

	// Trade statistics
	TotalTrades    int
	WinningTrades  int
	LosingTrades   int
	WinRate        float64

	// Performance metrics
	MaxDrawdown    float64
	SharpeRatio    float64
	ProfitFactor   float64
	AvgWin         float64
	AvgLoss        float64
	AvgTradeDuration time.Duration

	// All trades
	Trades         []Trade

	// Equity curve
	EquityCurve    []EquityPoint
}

// EquityPoint represents a point in the equity curve
type EquityPoint struct {
	Timestamp int64
	Equity    float64
	Drawdown  float64
}

// Backtester runs backtests on trading strategies
type Backtester struct {
	InitialCapital float64
	PositionSize   float64 // Size per trade (e.g., $10,000)
	Commission     float64 // Commission per trade (e.g., 0.001 for 0.1%)
}

// NewBacktester creates a new backtester
func NewBacktester(initialCapital, positionSize, commission float64) *Backtester {
	return &Backtester{
		InitialCapital: initialCapital,
		PositionSize:   positionSize,
		Commission:     commission,
	}
}

// RunBacktest runs a backtest on a pairs trading strategy
func (b *Backtester) RunBacktest(strategy *PairsTradingStrategy, prices1, prices2 []PricePoint) (*BacktestResult, error) {
	if len(prices1) != len(prices2) {
		return nil, fmt.Errorf("price series must have same length")
	}

	if len(prices1) < strategy.LookbackWindow {
		return nil, fmt.Errorf("insufficient data: need at least %d points", strategy.LookbackWindow)
	}

	// Initialize result
	result := &BacktestResult{
		Strategy:       "Pairs Trading",
		Pair:           strategy.Pair,
		InitialCapital: b.InitialCapital,
		StartDate:      time.Unix(prices1[0].Timestamp, 0),
		EndDate:        time.Unix(prices1[len(prices1)-1].Timestamp, 0),
		Trades:         []Trade{},
		EquityCurve:    []EquityPoint{},
	}

	// Reset strategy state
	strategy.Reset()

	// Current capital
	capital := b.InitialCapital
	maxCapital := capital

	// Track equity curve
	result.EquityCurve = append(result.EquityCurve, EquityPoint{
		Timestamp: prices1[0].Timestamp,
		Equity:    capital,
		Drawdown:  0,
	})

	// Simulate trading
	for i := 0; i < len(prices1); i++ {
		timestamp := prices1[i].Timestamp
		price1 := prices1[i].Price
		price2 := prices2[i].Price

		// Update strategy with new prices
		strategy.UpdatePrices(timestamp, price1, price2)

		// Generate signal
		signal, err := strategy.GenerateSignal(timestamp)
		if err != nil {
			// Not enough data yet, continue
			continue
		}

		if signal == nil {
			// No signal, but update equity if we have a position
			if strategy.HasOpenPosition() {
				pnl := strategy.CalculatePnL(price1, price2)
				currentEquity := capital + pnl

				drawdown := 0.0
				if maxCapital > 0 {
					drawdown = (maxCapital - currentEquity) / maxCapital
				}

				result.EquityCurve = append(result.EquityCurve, EquityPoint{
					Timestamp: timestamp,
					Equity:    currentEquity,
					Drawdown:  drawdown,
				})
			}
			continue
		}

		// Execute signal
		if signal.Action == "CLOSE" && strategy.HasOpenPosition() {
			// Close position
			pos := strategy.GetCurrentPosition()
			pnl := strategy.CalculatePnL(price1, price2)

			// Apply commission (both entry and exit)
			commissionCost := b.Commission * (pos.EntryPrice1 + pos.EntryPrice2 + price1 + price2) * pos.Quantity
			pnl -= commissionCost

			// Update capital
			capital += pnl

			// Record trade
			trade := Trade{
				EntryTime:   pos.EntryTimestamp,
				ExitTime:    timestamp,
				Asset1:      pos.Asset1,
				Asset2:      pos.Asset2,
				Direction:   pos.Direction,
				EntryPrice1: pos.EntryPrice1,
				EntryPrice2: pos.EntryPrice2,
				ExitPrice1:  price1,
				ExitPrice2:  price2,
				PnL:         pnl,
				PnLPercent:  pnl / (pos.EntryPrice1 + pos.EntryPrice2) * 100,
				Duration:    time.Unix(timestamp, 0).Sub(time.Unix(pos.EntryTimestamp, 0)),
			}
			result.Trades = append(result.Trades, trade)

			// Execute close
			strategy.ExecuteSignal(signal, b.PositionSize)

			// Update max capital
			if capital > maxCapital {
				maxCapital = capital
			}

		} else if (signal.Action == "LONG_1_SHORT_2" || signal.Action == "LONG_2_SHORT_1") && !strategy.HasOpenPosition() {
			// Open new position
			// Check if we have enough capital
			if capital < b.PositionSize {
				continue
			}

			// Calculate position quantity
			quantity := b.PositionSize / (signal.Price1 + signal.Price2)
			strategy.ExecuteSignal(signal, quantity)
		}

		// Record equity point
		currentEquity := capital
		if strategy.HasOpenPosition() {
			currentEquity += strategy.CalculatePnL(price1, price2)
		}

		drawdown := 0.0
		if maxCapital > 0 {
			drawdown = (maxCapital - currentEquity) / maxCapital
		}

		result.EquityCurve = append(result.EquityCurve, EquityPoint{
			Timestamp: timestamp,
			Equity:    currentEquity,
			Drawdown:  drawdown,
		})
	}

	// Close any remaining position
	if strategy.HasOpenPosition() {
		lastPrice1 := prices1[len(prices1)-1].Price
		lastPrice2 := prices2[len(prices2)-1].Price
		lastTimestamp := prices1[len(prices1)-1].Timestamp

		pos := strategy.GetCurrentPosition()
		pnl := strategy.CalculatePnL(lastPrice1, lastPrice2)
		commissionCost := b.Commission * (pos.EntryPrice1 + pos.EntryPrice2 + lastPrice1 + lastPrice2) * pos.Quantity
		pnl -= commissionCost
		capital += pnl

		trade := Trade{
			EntryTime:   pos.EntryTimestamp,
			ExitTime:    lastTimestamp,
			Asset1:      pos.Asset1,
			Asset2:      pos.Asset2,
			Direction:   pos.Direction,
			EntryPrice1: pos.EntryPrice1,
			EntryPrice2: pos.EntryPrice2,
			ExitPrice1:  lastPrice1,
			ExitPrice2:  lastPrice2,
			PnL:         pnl,
			PnLPercent:  pnl / (pos.EntryPrice1 + pos.EntryPrice2) * 100,
			Duration:    time.Unix(lastTimestamp, 0).Sub(time.Unix(pos.EntryTimestamp, 0)),
		}
		result.Trades = append(result.Trades, trade)
	}

	// Calculate final metrics
	result.FinalCapital = capital
	result.TotalReturn = capital - b.InitialCapital
	result.TotalReturnPct = (result.TotalReturn / b.InitialCapital) * 100
	result.TotalTrades = len(result.Trades)

	// Calculate trade statistics
	var totalWin, totalLoss float64
	var totalDuration time.Duration

	for _, trade := range result.Trades {
		if trade.PnL > 0 {
			result.WinningTrades++
			totalWin += trade.PnL
		} else {
			result.LosingTrades++
			totalLoss += math.Abs(trade.PnL)
		}
		totalDuration += trade.Duration
	}

	if result.TotalTrades > 0 {
		result.WinRate = float64(result.WinningTrades) / float64(result.TotalTrades) * 100
		result.AvgTradeDuration = totalDuration / time.Duration(result.TotalTrades)
	}

	if result.WinningTrades > 0 {
		result.AvgWin = totalWin / float64(result.WinningTrades)
	}
	if result.LosingTrades > 0 {
		result.AvgLoss = totalLoss / float64(result.LosingTrades)
	}

	// Calculate profit factor
	if totalLoss > 0 {
		result.ProfitFactor = totalWin / totalLoss
	}

	// Calculate max drawdown
	result.MaxDrawdown = b.calculateMaxDrawdown(result.EquityCurve)

	// Calculate Sharpe ratio
	result.SharpeRatio = b.calculateSharpeRatio(result.EquityCurve)

	return result, nil
}

// calculateMaxDrawdown calculates the maximum drawdown
func (b *Backtester) calculateMaxDrawdown(equityCurve []EquityPoint) float64 {
	if len(equityCurve) == 0 {
		return 0
	}

	maxDrawdown := 0.0
	peak := equityCurve[0].Equity

	for _, point := range equityCurve {
		if point.Equity > peak {
			peak = point.Equity
		}

		drawdown := (peak - point.Equity) / peak
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}

	return maxDrawdown * 100 // Return as percentage
}

// calculateSharpeRatio calculates the Sharpe ratio
func (b *Backtester) calculateSharpeRatio(equityCurve []EquityPoint) float64 {
	if len(equityCurve) < 2 {
		return 0
	}

	// Calculate returns
	returns := make([]float64, len(equityCurve)-1)
	for i := 1; i < len(equityCurve); i++ {
		if equityCurve[i-1].Equity != 0 {
			returns[i-1] = (equityCurve[i].Equity - equityCurve[i-1].Equity) / equityCurve[i-1].Equity
		}
	}

	// Calculate mean return
	var sum float64
	for _, r := range returns {
		sum += r
	}
	meanReturn := sum / float64(len(returns))

	// Calculate standard deviation
	var variance float64
	for _, r := range returns {
		diff := r - meanReturn
		variance += diff * diff
	}
	variance /= float64(len(returns) - 1)
	stdDev := math.Sqrt(variance)

	if stdDev == 0 {
		return 0
	}

	// Annualize (assuming daily returns)
	// Sharpe = (mean_return * 252) / (std_dev * sqrt(252))
	sharpe := (meanReturn * math.Sqrt(252)) / stdDev

	return sharpe
}

// PrintReport prints a formatted backtest report
func (r *BacktestResult) PrintReport() {
	separator := repeatString("=", 80)
	line := repeatString("-", 80)

	fmt.Println("\n" + separator)
	fmt.Println("BACKTEST RESULTS")
	fmt.Println(separator)

	fmt.Printf("\nStrategy: %s\n", r.Strategy)
	fmt.Printf("Pair: %s (%s) <-> %s (%s)\n", r.Pair.Asset1, r.Pair.Ticker1, r.Pair.Asset2, r.Pair.Ticker2)
	fmt.Printf("Correlation: %.4f\n", r.Pair.Correlation)
	fmt.Printf("Period: %s to %s\n", r.StartDate.Format("2006-01-02"), r.EndDate.Format("2006-01-02"))

	fmt.Println("\n" + line)
	fmt.Println("PERFORMANCE")
	fmt.Println(line)

	fmt.Printf("Initial Capital:    $%.2f\n", r.InitialCapital)
	fmt.Printf("Final Capital:      $%.2f\n", r.FinalCapital)
	fmt.Printf("Total Return:       $%.2f (%.2f%%)\n", r.TotalReturn, r.TotalReturnPct)
	fmt.Printf("Max Drawdown:       %.2f%%\n", r.MaxDrawdown)
	fmt.Printf("Sharpe Ratio:       %.2f\n", r.SharpeRatio)

	fmt.Println("\n" + line)
	fmt.Println("TRADE STATISTICS")
	fmt.Println(line)

	fmt.Printf("Total Trades:       %d\n", r.TotalTrades)
	fmt.Printf("Winning Trades:     %d (%.1f%%)\n", r.WinningTrades, r.WinRate)
	fmt.Printf("Losing Trades:      %d\n", r.LosingTrades)
	fmt.Printf("Profit Factor:      %.2f\n", r.ProfitFactor)
	fmt.Printf("Average Win:        $%.2f\n", r.AvgWin)
	fmt.Printf("Average Loss:       $%.2f\n", r.AvgLoss)
	fmt.Printf("Avg Trade Duration: %v\n", r.AvgTradeDuration.Round(time.Hour))

	if len(r.Trades) > 0 {
		fmt.Println("\n" + line)
		fmt.Println("RECENT TRADES (Last 10)")
		fmt.Println(line)

		start := len(r.Trades) - 10
		if start < 0 {
			start = 0
		}

		for i := start; i < len(r.Trades); i++ {
			t := r.Trades[i]
			entryTime := time.Unix(t.EntryTime, 0).Format("2006-01-02")
			exitTime := time.Unix(t.ExitTime, 0).Format("2006-01-02")

			fmt.Printf("\nTrade #%d: %s\n", i+1, t.Direction)
			fmt.Printf("  Entry: %s  Exit: %s  Duration: %v\n", entryTime, exitTime, t.Duration.Round(time.Hour*24))
			fmt.Printf("  P&L: $%.2f (%.2f%%)\n", t.PnL, t.PnLPercent)
		}
	}

	fmt.Println("\n" + separator)
}

func repeatString(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
