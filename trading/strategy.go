package trading

import (
	"fmt"
	"math"
)

// Signal represents a trading signal
type Signal struct {
	Timestamp int64
	Asset1    string
	Asset2    string
	Ticker1   string
	Ticker2   string
	Action    string  // "LONG_1_SHORT_2", "LONG_2_SHORT_1", "CLOSE"
	ZScore    float64
	Price1    float64
	Price2    float64
	Spread    float64
}

// Position represents an open trading position
type Position struct {
	EntryTimestamp int64
	Asset1         string
	Asset2         string
	Ticker1        string
	Ticker2        string
	Direction      string  // "LONG_1_SHORT_2" or "LONG_2_SHORT_1"
	EntryPrice1    float64
	EntryPrice2    float64
	EntrySpread    float64
	EntryZScore    float64
	Quantity       float64 // Position size
}

// PairsTradingStrategy implements a statistical arbitrage pairs trading strategy
type PairsTradingStrategy struct {
	Pair              CorrelationPair
	EntryThreshold    float64 // Z-score threshold for entry (e.g., 2.0)
	ExitThreshold     float64 // Z-score threshold for exit (e.g., 0.5)
	StopLoss          float64 // Stop loss as percentage (e.g., 0.05 for 5%)
	LookbackWindow    int     // Number of periods for calculating spread statistics
	CurrentPosition   *Position
	PriceHistory1     []PricePoint
	PriceHistory2     []PricePoint
}

// NewPairsTradingStrategy creates a new pairs trading strategy
func NewPairsTradingStrategy(pair CorrelationPair, entryThreshold, exitThreshold, stopLoss float64, lookbackWindow int) *PairsTradingStrategy {
	return &PairsTradingStrategy{
		Pair:           pair,
		EntryThreshold: entryThreshold,
		ExitThreshold:  exitThreshold,
		StopLoss:       stopLoss,
		LookbackWindow: lookbackWindow,
	}
}

// UpdatePrices adds new price observations
func (s *PairsTradingStrategy) UpdatePrices(timestamp int64, price1, price2 float64) {
	s.PriceHistory1 = append(s.PriceHistory1, PricePoint{Timestamp: timestamp, Price: price1})
	s.PriceHistory2 = append(s.PriceHistory2, PricePoint{Timestamp: timestamp, Price: price2})

	// Keep only the lookback window + some buffer
	maxLen := s.LookbackWindow * 2
	if len(s.PriceHistory1) > maxLen {
		s.PriceHistory1 = s.PriceHistory1[len(s.PriceHistory1)-maxLen:]
		s.PriceHistory2 = s.PriceHistory2[len(s.PriceHistory2)-maxLen:]
	}
}

// CalculateSpread calculates the spread between two price series
// Using the ratio method: spread = price1 / price2
func (s *PairsTradingStrategy) CalculateSpread() []float64 {
	minLen := len(s.PriceHistory1)
	if len(s.PriceHistory2) < minLen {
		minLen = len(s.PriceHistory2)
	}

	spreads := make([]float64, minLen)
	for i := 0; i < minLen; i++ {
		if s.PriceHistory2[i].Price != 0 {
			spreads[i] = s.PriceHistory1[i].Price / s.PriceHistory2[i].Price
		}
	}

	return spreads
}

// CalculateZScore calculates the z-score of the current spread
func (s *PairsTradingStrategy) CalculateZScore() (float64, error) {
	spreads := s.CalculateSpread()

	if len(spreads) < s.LookbackWindow {
		return 0, fmt.Errorf("insufficient data: have %d, need %d", len(spreads), s.LookbackWindow)
	}

	// Use only the lookback window
	recentSpreads := spreads[len(spreads)-s.LookbackWindow:]

	// Calculate mean and std dev
	var sum float64
	for _, spread := range recentSpreads {
		sum += spread
	}
	mean := sum / float64(len(recentSpreads))

	var variance float64
	for _, spread := range recentSpreads {
		diff := spread - mean
		variance += diff * diff
	}
	variance /= float64(len(recentSpreads) - 1)
	stdDev := math.Sqrt(variance)

	if stdDev == 0 {
		return 0, fmt.Errorf("zero standard deviation")
	}

	// Current spread
	currentSpread := spreads[len(spreads)-1]
	zScore := (currentSpread - mean) / stdDev

	return zScore, nil
}

// GenerateSignal generates a trading signal based on current market conditions
func (s *PairsTradingStrategy) GenerateSignal(timestamp int64) (*Signal, error) {
	if len(s.PriceHistory1) < s.LookbackWindow || len(s.PriceHistory2) < s.LookbackWindow {
		return nil, fmt.Errorf("insufficient data for signal generation")
	}

	zScore, err := s.CalculateZScore()
	if err != nil {
		return nil, err
	}

	currentPrice1 := s.PriceHistory1[len(s.PriceHistory1)-1].Price
	currentPrice2 := s.PriceHistory2[len(s.PriceHistory2)-1].Price
	currentSpread := currentPrice1 / currentPrice2

	signal := &Signal{
		Timestamp: timestamp,
		Asset1:    s.Pair.Asset1,
		Asset2:    s.Pair.Asset2,
		Ticker1:   s.Pair.Ticker1,
		Ticker2:   s.Pair.Ticker2,
		ZScore:    zScore,
		Price1:    currentPrice1,
		Price2:    currentPrice2,
		Spread:    currentSpread,
	}

	// Check if we have an open position
	if s.CurrentPosition != nil {
		// Check stop loss
		pnl := s.CalculatePnL(currentPrice1, currentPrice2)
		pnlPercent := pnl / (s.CurrentPosition.EntryPrice1 + s.CurrentPosition.EntryPrice2)

		if pnlPercent < -s.StopLoss {
			signal.Action = "CLOSE"
			return signal, nil
		}

		// Check exit conditions
		if math.Abs(zScore) < s.ExitThreshold {
			signal.Action = "CLOSE"
			return signal, nil
		}

		// Check reversal (z-score crossed zero - spread mean reverted too much)
		if (s.CurrentPosition.Direction == "LONG_1_SHORT_2" && zScore < 0) ||
			(s.CurrentPosition.Direction == "LONG_2_SHORT_1" && zScore > 0) {
			signal.Action = "CLOSE"
			return signal, nil
		}

		return nil, nil // Hold current position
	}

	// Check entry conditions
	if zScore > s.EntryThreshold {
		// Spread is high: short asset1, long asset2
		signal.Action = "LONG_2_SHORT_1"
		return signal, nil
	}

	if zScore < -s.EntryThreshold {
		// Spread is low: long asset1, short asset2
		signal.Action = "LONG_1_SHORT_2"
		return signal, nil
	}

	return nil, nil // No signal
}

// ExecuteSignal executes a trading signal
func (s *PairsTradingStrategy) ExecuteSignal(signal *Signal, positionSize float64) {
	if signal.Action == "CLOSE" && s.CurrentPosition != nil {
		s.CurrentPosition = nil
		return
	}

	if signal.Action == "LONG_1_SHORT_2" || signal.Action == "LONG_2_SHORT_1" {
		s.CurrentPosition = &Position{
			EntryTimestamp: signal.Timestamp,
			Asset1:         signal.Asset1,
			Asset2:         signal.Asset2,
			Ticker1:        signal.Ticker1,
			Ticker2:        signal.Ticker2,
			Direction:      signal.Action,
			EntryPrice1:    signal.Price1,
			EntryPrice2:    signal.Price2,
			EntrySpread:    signal.Spread,
			EntryZScore:    signal.ZScore,
			Quantity:       positionSize,
		}
	}
}

// CalculatePnL calculates the current P&L for an open position
func (s *PairsTradingStrategy) CalculatePnL(currentPrice1, currentPrice2 float64) float64 {
	if s.CurrentPosition == nil {
		return 0
	}

	var pnl float64

	if s.CurrentPosition.Direction == "LONG_1_SHORT_2" {
		// Long asset1, short asset2
		pnl1 := (currentPrice1 - s.CurrentPosition.EntryPrice1) * s.CurrentPosition.Quantity
		pnl2 := (s.CurrentPosition.EntryPrice2 - currentPrice2) * s.CurrentPosition.Quantity
		pnl = pnl1 + pnl2
	} else {
		// Long asset2, short asset1
		pnl1 := (s.CurrentPosition.EntryPrice1 - currentPrice1) * s.CurrentPosition.Quantity
		pnl2 := (currentPrice2 - s.CurrentPosition.EntryPrice2) * s.CurrentPosition.Quantity
		pnl = pnl1 + pnl2
	}

	return pnl
}

// HasOpenPosition returns whether there's an open position
func (s *PairsTradingStrategy) HasOpenPosition() bool {
	return s.CurrentPosition != nil
}

// GetCurrentPosition returns the current position if any
func (s *PairsTradingStrategy) GetCurrentPosition() *Position {
	return s.CurrentPosition
}

// Reset resets the strategy state
func (s *PairsTradingStrategy) Reset() {
	s.CurrentPosition = nil
	s.PriceHistory1 = []PricePoint{}
	s.PriceHistory2 = []PricePoint{}
}
