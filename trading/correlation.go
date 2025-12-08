package trading

import (
	"fmt"
	"margraf/graph"
	"math"
	"sort"
)

// PricePoint represents a single price observation
type PricePoint struct {
	Timestamp int64
	Price     float64
}

// AssetPriceHistory stores historical price data for an asset
type AssetPriceHistory struct {
	AssetID string
	Ticker  string
	Prices  []PricePoint
}

// CorrelationPair represents a pair of correlated assets
type CorrelationPair struct {
	Asset1         string
	Asset2         string
	Ticker1        string
	Ticker2        string
	Correlation    float64
	GraphDistance  int     // Distance in the knowledge graph
	HasDirectEdge  bool    // Whether there's a direct edge between them
	EdgeWeight     float64 // Weight of the edge if exists
}

// CorrelationAnalyzer analyzes correlations between assets
type CorrelationAnalyzer struct {
	Graph *graph.Graph
}

// NewCorrelationAnalyzer creates a new correlation analyzer
func NewCorrelationAnalyzer(g *graph.Graph) *CorrelationAnalyzer {
	return &CorrelationAnalyzer{
		Graph: g,
	}
}

// CalculateCorrelation computes Pearson correlation coefficient between two price series
func CalculateCorrelation(prices1, prices2 []PricePoint) (float64, error) {
	// Align the time series by timestamp
	aligned1, aligned2 := alignTimeSeries(prices1, prices2)

	if len(aligned1) < 2 {
		return 0, fmt.Errorf("insufficient data points: %d", len(aligned1))
	}

	// Calculate means
	var sum1, sum2 float64
	n := float64(len(aligned1))
	for i := 0; i < len(aligned1); i++ {
		sum1 += aligned1[i]
		sum2 += aligned2[i]
	}
	mean1 := sum1 / n
	mean2 := sum2 / n

	// Calculate correlation
	var numerator, denom1, denom2 float64
	for i := 0; i < len(aligned1); i++ {
		diff1 := aligned1[i] - mean1
		diff2 := aligned2[i] - mean2
		numerator += diff1 * diff2
		denom1 += diff1 * diff1
		denom2 += diff2 * diff2
	}

	if denom1 == 0 || denom2 == 0 {
		return 0, fmt.Errorf("zero variance in price series")
	}

	correlation := numerator / math.Sqrt(denom1*denom2)
	return correlation, nil
}

// alignTimeSeries aligns two time series by matching timestamps
// Returns two slices of prices with matching timestamps
func alignTimeSeries(prices1, prices2 []PricePoint) ([]float64, []float64) {
	// Create maps for fast lookup
	map1 := make(map[int64]float64)
	map2 := make(map[int64]float64)

	for _, p := range prices1 {
		map1[p.Timestamp] = p.Price
	}
	for _, p := range prices2 {
		map2[p.Timestamp] = p.Price
	}

	// Find common timestamps
	var aligned1, aligned2 []float64
	for ts, price1 := range map1 {
		if price2, exists := map2[ts]; exists {
			aligned1 = append(aligned1, price1)
			aligned2 = append(aligned2, price2)
		}
	}

	return aligned1, aligned2
}

// FindCorrelatedPairs finds all correlated asset pairs
func (ca *CorrelationAnalyzer) FindCorrelatedPairs(priceHistories map[string]*AssetPriceHistory, minCorrelation float64) ([]CorrelationPair, error) {
	var pairs []CorrelationPair

	// Get all asset IDs
	assetIDs := make([]string, 0, len(priceHistories))
	for id := range priceHistories {
		assetIDs = append(assetIDs, id)
	}

	// Compare all pairs
	for i := 0; i < len(assetIDs); i++ {
		for j := i + 1; j < len(assetIDs); j++ {
			asset1 := assetIDs[i]
			asset2 := assetIDs[j]

			hist1 := priceHistories[asset1]
			hist2 := priceHistories[asset2]

			// Calculate statistical correlation
			corr, err := CalculateCorrelation(hist1.Prices, hist2.Prices)
			if err != nil {
				// Skip pairs with insufficient data
				continue
			}

			// Only include pairs meeting minimum correlation threshold
			if math.Abs(corr) >= minCorrelation {
				// Get graph structure information
				distance, hasEdge, weight := ca.getGraphRelationship(asset1, asset2)

				pair := CorrelationPair{
					Asset1:        asset1,
					Asset2:        asset2,
					Ticker1:       hist1.Ticker,
					Ticker2:       hist2.Ticker,
					Correlation:   corr,
					GraphDistance: distance,
					HasDirectEdge: hasEdge,
					EdgeWeight:    weight,
				}
				pairs = append(pairs, pair)
			}
		}
	}

	// Sort by absolute correlation (highest first)
	sort.Slice(pairs, func(i, j int) bool {
		return math.Abs(pairs[i].Correlation) > math.Abs(pairs[j].Correlation)
	})

	return pairs, nil
}

// getGraphRelationship returns the distance and edge information between two nodes
func (ca *CorrelationAnalyzer) getGraphRelationship(asset1, asset2 string) (distance int, hasEdge bool, weight float64) {
	// Check for direct edge
	edges := ca.Graph.GetOutgoingEdges(asset1)
	for _, e := range edges {
		if e.TargetID == asset2 {
			return 1, true, e.Weight
		}
	}

	// Check reverse direction
	edges = ca.Graph.GetOutgoingEdges(asset2)
	for _, e := range edges {
		if e.TargetID == asset1 {
			return 1, true, e.Weight
		}
	}

	// For now, use BFS to find shortest path (limited depth for performance)
	distance = ca.bfsDistance(asset1, asset2, 3)
	return distance, false, 0
}

// bfsDistance performs BFS to find shortest path distance (limited depth)
func (ca *CorrelationAnalyzer) bfsDistance(start, target string, maxDepth int) int {
	if start == target {
		return 0
	}

	visited := make(map[string]bool)
	queue := []struct {
		nodeID string
		depth  int
	}{{start, 0}}

	visited[start] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth >= maxDepth {
			continue
		}

		edges := ca.Graph.GetOutgoingEdges(current.nodeID)
		for _, e := range edges {
			if e.TargetID == target {
				return current.depth + 1
			}

			if !visited[e.TargetID] {
				visited[e.TargetID] = true
				queue = append(queue, struct {
					nodeID string
					depth  int
				}{e.TargetID, current.depth + 1})
			}
		}
	}

	return -1 // Not connected within maxDepth
}

// CalculateReturns converts prices to returns
func CalculateReturns(prices []PricePoint) []float64 {
	if len(prices) < 2 {
		return []float64{}
	}

	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		if prices[i-1].Price != 0 {
			returns[i-1] = (prices[i].Price - prices[i-1].Price) / prices[i-1].Price
		}
	}

	return returns
}

// CalculateVolatility calculates the standard deviation of returns
func CalculateVolatility(prices []PricePoint) float64 {
	returns := CalculateReturns(prices)
	if len(returns) < 2 {
		return 0
	}

	// Calculate mean return
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// Calculate variance
	var variance float64
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(returns) - 1)

	return math.Sqrt(variance)
}
