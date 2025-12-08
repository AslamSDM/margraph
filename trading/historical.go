package trading

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HistoricalDataFetcher fetches historical price data for backtesting
type HistoricalDataFetcher struct {
	Client *http.Client
}

// NewHistoricalDataFetcher creates a new historical data fetcher
func NewHistoricalDataFetcher() *HistoricalDataFetcher {
	return &HistoricalDataFetcher{
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchYahooHistoricalData fetches historical data from Yahoo Finance
// This uses Yahoo's download API which returns CSV data
func (h *HistoricalDataFetcher) FetchYahooHistoricalData(ticker string, startDate, endDate time.Time) ([]PricePoint, error) {
	// Convert dates to Unix timestamps
	period1 := startDate.Unix()
	period2 := endDate.Unix()

	// Yahoo Finance historical data URL - using query2 endpoint which is more reliable
	url := fmt.Sprintf("https://query2.finance.yahoo.com/v7/finance/download/%s?period1=%d&period2=%d&interval=1d&events=history&includeAdjustedClose=true",
		ticker, period1, period2)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set comprehensive headers to avoid 401/403 errors
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch data for %s: %w", ticker, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Try alternate approach - scrape from Yahoo Finance page directly
		return h.fetchFromYahooChartAPI(ticker, startDate, endDate)
	}

	// Parse CSV response
	reader := csv.NewReader(resp.Body)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Find column indices
	dateIdx, closeIdx := -1, -1
	for i, col := range header {
		col = strings.TrimSpace(col)
		if col == "Date" {
			dateIdx = i
		} else if col == "Close" || col == "Adj Close" {
			closeIdx = i
		}
	}

	if dateIdx == -1 || closeIdx == -1 {
		return nil, fmt.Errorf("could not find Date or Close columns in CSV")
	}

	var pricePoints []PricePoint

	// Read data rows
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // Skip malformed rows
		}

		if len(record) <= dateIdx || len(record) <= closeIdx {
			continue
		}

		// Parse date
		dateStr := strings.TrimSpace(record[dateIdx])
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		// Parse price
		priceStr := strings.TrimSpace(record[closeIdx])
		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			continue
		}

		pricePoints = append(pricePoints, PricePoint{
			Timestamp: t.Unix(),
			Price:     price,
		})
	}

	if len(pricePoints) == 0 {
		return nil, fmt.Errorf("no valid price data found for %s", ticker)
	}

	return pricePoints, nil
}

// fetchFromYahooChartAPI uses Yahoo's chart API as an alternative
func (h *HistoricalDataFetcher) fetchFromYahooChartAPI(ticker string, startDate, endDate time.Time) ([]PricePoint, error) {
	period1 := startDate.Unix()
	period2 := endDate.Unix()

	// Yahoo Finance Chart API
	url := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d&events=history",
		ticker, period1, period2)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chart API failed for %s: %w", ticker, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("chart API returned status %d for %s", resp.StatusCode, ticker)
	}

	// Parse JSON response
	var result struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON for %s: %w", ticker, err)
	}

	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo API error for %s: %s", ticker, result.Chart.Error.Description)
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data returned for %s", ticker)
	}

	data := result.Chart.Result[0]
	timestamps := data.Timestamp

	if len(data.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("no price data for %s", ticker)
	}

	closes := data.Indicators.Quote[0].Close

	if len(timestamps) != len(closes) {
		return nil, fmt.Errorf("mismatched data lengths for %s", ticker)
	}

	var pricePoints []PricePoint
	for i := 0; i < len(timestamps); i++ {
		// Skip null values
		if closes[i] == 0 {
			continue
		}

		pricePoints = append(pricePoints, PricePoint{
			Timestamp: timestamps[i],
			Price:     closes[i],
		})
	}

	if len(pricePoints) == 0 {
		return nil, fmt.Errorf("no valid price data for %s", ticker)
	}

	return pricePoints, nil
}

// FetchMultipleHistoricalData fetches data for multiple tickers
func (h *HistoricalDataFetcher) FetchMultipleHistoricalData(tickers []string, startDate, endDate time.Time) (map[string][]PricePoint, error) {
	results := make(map[string][]PricePoint)
	errors := []string{}

	for _, ticker := range tickers {
		prices, err := h.FetchYahooHistoricalData(ticker, startDate, endDate)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", ticker, err))
			continue
		}
		results[ticker] = prices
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("failed to fetch any data: %v", errors)
	}

	return results, nil
}

// GenerateMockHistoricalData generates mock price data for testing
// Simulates correlated price movements with mean-reverting spread
func GenerateMockHistoricalData(ticker1, ticker2 string, correlation float64, days int) ([]PricePoint, []PricePoint) {
	// Start date
	startDate := time.Now().AddDate(0, 0, -days)

	// Initial prices
	price1 := 100.0
	price2 := 50.0

	prices1 := []PricePoint{}
	prices2 := []PricePoint{}

	// Generate correlated random walk with mean-reverting spread
	// This ensures the spread oscillates, creating trading opportunities
	spreadTarget := price1 / price2 // Target spread ratio
	currentSpread := spreadTarget

	for i := 0; i < days; i++ {
		timestamp := startDate.AddDate(0, 0, i).Unix()

		// Generate base market movement
		baseReturn := (simpleRandom()*2.0 - 1.0) * 0.015 // -1.5% to +1.5%

		// Add mean reversion to spread
		spreadDrift := (spreadTarget - currentSpread) * 0.05 // Mean reversion force

		// Generate individual returns with correlation
		noise1 := (simpleRandom()*2.0 - 1.0) * 0.02
		noise2 := (simpleRandom()*2.0 - 1.0) * 0.02

		return1 := baseReturn*correlation + noise1 - spreadDrift*0.01
		return2 := baseReturn*correlation + noise2 + spreadDrift*0.01

		price1 *= (1 + return1)
		price2 *= (1 + return2)

		// Ensure prices stay positive
		if price1 < 1.0 {
			price1 = 1.0
		}
		if price2 < 1.0 {
			price2 = 1.0
		}

		currentSpread = price1 / price2

		prices1 = append(prices1, PricePoint{Timestamp: timestamp, Price: price1})
		prices2 = append(prices2, PricePoint{Timestamp: timestamp, Price: price2})
	}

	return prices1, prices2
}

// Simple pseudo-random number generator for mock data
// Returns value between 0 and 1
func simpleRandom() float64 {
	// Use time-based seed for variety
	nano := time.Now().UnixNano()
	// Simple linear congruential generator
	seed := (nano * 1103515245 + 12345) % 2147483648
	return float64(seed) / 2147483648.0
}

// Simple random normal generator (Box-Muller transform)
func randomNormal() float64 {
	// This is a simplified version - in production use math/rand properly
	u1 := float64(time.Now().UnixNano()%1000) / 1000.0
	u2 := float64((time.Now().UnixNano()/1000)%1000) / 1000.0

	if u1 < 0.001 {
		u1 = 0.001
	}
	if u2 < 0.001 {
		u2 = 0.001
	}

	z0 := (-2.0 * logApprox(u1))
	if z0 < 0 {
		z0 = 0
	}
	z0 = sqrtApprox(z0) * cosApprox(2.0*3.14159265359*u2)

	return z0
}

// Fast sqrt approximation
func sqrtApprox(x float64) float64 {
	if x < 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// Fast log approximation
func logApprox(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Simple approximation for x near 1
	return (x - 1) - (x-1)*(x-1)/2 + (x-1)*(x-1)*(x-1)/3
}

// Fast cos approximation
func cosApprox(x float64) float64 {
	// Taylor series approximation
	x2 := x * x
	return 1 - x2/2 + x2*x2/24
}
