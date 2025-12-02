package scraper

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type StockData struct {
	Ticker   string
	Price    float64
	Change   float64
	Currency string
}

// FinanceScraper fetches data from Yahoo Finance.
type FinanceScraper struct {
	Client *http.Client
}

func NewFinanceScraper() *FinanceScraper {
	return &FinanceScraper{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetTicker tries to find the ticker symbol for a company name via DuckDuckGo (RAG-lite)
// because we don't have a direct symbol database.
func (s *FinanceScraper) GetTicker(companyName string) (string, error) {
	// Simplified: In a real app, we'd use a lookup API.
	// Here we assume the node name might ALREADY be a ticker if it's short,
	// or we search for "CompanyName ticker yahoo finance"
	
	ws := NewWebSearcher()
	query := fmt.Sprintf("%s ticker symbol yahoo finance", companyName)
	results, err := ws.Search(query)
	if err != nil {
		return "", err
	}

	for _, res := range results {
		// Look for patterns like (AAPL) or "Symbol: AAPL" or finance.yahoo.com/quote/AAPL
		if strings.Contains(res.Link, "finance.yahoo.com/quote/") {
			parts := strings.Split(res.Link, "/quote/")
			if len(parts) > 1 {
				ticker := strings.Split(parts[1], "/")[0]
				ticker = strings.Split(ticker, "?")[0]
				return ticker, nil
			}
		}
	}
	return "", fmt.Errorf("ticker not found")
}

// FetchStockData scrapes the price from Yahoo Finance.
func (s *FinanceScraper) FetchStockData(ticker string) (*StockData, error) {
	url := fmt.Sprintf("https://finance.yahoo.com/quote/%s", ticker)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("yahoo status: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	// Yahoo Finance selectors change often. We look for specific data-field attributes or standard fin-streamer classes.
	// Strategy: Look for <fin-streamer data-field="regularMarketPrice">
	
	var price float64
	var change float64
	var currency string

	doc.Find("fin-streamer").Each(func(i int, s *goquery.Selection) {
		field, _ := s.Attr("data-field")
		valStr, _ := s.Attr("value") // Yahoo often stores raw value in 'value' attr
		
		// Fallback to text if value is empty
		if valStr == "" {
			valStr = s.Text()
		}

		if field == "regularMarketPrice" {
			fmt.Sscanf(valStr, "%f", &price)
			curr, exists := s.Attr("data-currency") // Sometimes currency is here
			if exists { currency = curr }
		}
		if field == "regularMarketChangePercent" {
			fmt.Sscanf(valStr, "%f", &change)
		}
	})

	// Fallback for currency if not found in streamer
	if currency == "" {
		currency = "USD" // Default assumption
	}

	if price == 0 {
		return nil, fmt.Errorf("could not parse price")
	}

	return &StockData{
		Ticker:   ticker,
		Price:    price,
		Change:   change,
		Currency: currency,
	}, nil
}
