package simulation

import (
	"margraf/graph"
	"margraf/logger"
	"margraf/scraper"
	"margraf/server"
	"time"
)

type MarketMonitor struct {
	Graph   *graph.Graph
	Hub     *server.Hub
	Scraper *scraper.FinanceScraper
}

func NewMarketMonitor(g *graph.Graph, h *server.Hub) *MarketMonitor {
	return &MarketMonitor{
		Graph:   g,
		Hub:     h,
		Scraper: scraper.NewFinanceScraper(),
	}
}

func (m *MarketMonitor) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	logger.Info(logger.StatusMon, "Market Monitor active. Checking prices every %v...", interval)
	
	for range ticker.C {
		m.UpdatePrices()
	}
}

func (m *MarketMonitor) UpdatePrices() {
	// Iterate over all nodes, find Corporations
	// (Optimization: Maintain a separate list of corporate IDs)
	
	m.Graph.NodesRange(func(n *graph.Node) {
		if n.Type == graph.NodeTypeCorporation {
			go m.checkStock(n)
		}
	})
}

func (m *MarketMonitor) checkStock(n *graph.Node) {
	// If no ticker, try to find one
	ticker, _ := m.Graph.GetNodeTicker(n.ID)
	if ticker == "" {
		t, err := m.Scraper.GetTicker(n.Name)
		if err != nil {
			// fmt.Printf("    ⚠️ No ticker found for %s\n", n.Name)
			return
		}
		m.Graph.SetNodeTicker(n.ID, t)
		ticker = t
		logger.InfoDepth(2, logger.StatusTag, "Found Ticker for %s: %s", n.Name, t)
	}

	data, err := m.Scraper.FetchStockData(ticker)
	if err != nil {
		// fmt.Printf("    ⚠️ Failed to fetch price for %s (%s): %v\n", n.Name, ticker, err)
		return
	}

	// Update Node with thread-safe method
	if err := m.Graph.UpdateNodePrice(n.ID, data.Price, data.Currency, ""); err != nil {
		logger.WarnDepth(2, logger.StatusWarn, "Failed to update price for %s: %v", n.Name, err)
		return
	}

	// Adjust health based on daily change
	// e.g. +5% change = +0.05 health (Simplified logic)
	healthImpact := data.Change * 0.1 // Scale down
	newHealth, _ := m.Graph.UpdateNodeHealth(n.ID, healthImpact)

	logger.InfoDepth(2, logger.StatusFin, "%s (%s): %.2f %s (Change: %.2f%%)", n.Name, ticker, data.Price, data.Currency, data.Change*100)

	// Broadcast update
	m.Hub.Broadcast("market_update", map[string]interface{}{
		"id":       n.ID,
		"price":    data.Price,
		"currency": data.Currency,
		"health":   newHealth,
	})
}
