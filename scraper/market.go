package scraper

import (
	"fmt"
	"margraf/logger"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// MarketScraper handles fetching economic data from public web sources.
type MarketScraper struct {
	Client *http.Client
}

func NewMarketScraper() *MarketScraper {
	return &MarketScraper{
		Client: &http.Client{},
	}
}

// FetchTopNations scrapes Wikipedia for top economies by GDP.
// Fallback: Returns a hardcoded list if scraping fails (to ensure app stability),
// but tries to fetch real data first.
func (s *MarketScraper) FetchTopNations(limit int) ([]string, error) {
	url := "https://en.wikipedia.org/wiki/List_of_countries_by_GDP_(nominal)"
	logger.InfoDepth(1, logger.StatusGlob, "Scraping real economic data from: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.google.com")

	res, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	var nations []string
	// Wikipedia tables are tricky. We look for the first wikitable sortable.
	// The column for country name usually changes, but is often the first text link in the row.
	doc.Find("table.wikitable.sortable tbody tr").Each(func(i int, s *goquery.Selection) {
		if len(nations) >= limit {
			return
		}
		
		// Skip header
		if i == 0 { return }

		// Try to find the country name in the first or second cell
		name := s.Find("td").Eq(0).Find("a").Text()
		if name == "" {
			name = s.Find("td").Eq(1).Find("a").Text() // Sometimes rank is first col
		}
		
		name = strings.TrimSpace(name)
		if name != "" && name != "World" {
			nations = append(nations, name)
		}
	})

	if len(nations) == 0 {
		return nil, fmt.Errorf("failed to extract nations from HTML")
	}
	return nations, nil
}

// FetchMajorCompanies scrapes a list of largest companies (simplified approach).
// Realistically, scraping specific industry lists per country is complex without a search engine.
// We will use a search-proxy approach: We can't easily search Google, but we can scrape
// "List of largest companies in [Country]" if a direct Wiki URL exists, or fallback to LLM.
// For this prototype, we will keep this method but note the limitation.
func (s *MarketScraper) FetchMajorCompanies(country string) ([]string, error) {
	// Attempt to find a specific wikipedia list
	country = strings.ReplaceAll(country, " ", "_")
	url := fmt.Sprintf("https://en.wikipedia.org/wiki/List_of_companies_of_%s", country)
	if country == "United_States" {
		url = "https://en.wikipedia.org/wiki/List_of_largest_companies_in_the_United_States_by_revenue"
	}

	logger.InfoDepth(1, logger.StatusCor, "Scraping company data from: %s", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.google.com")

	res, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		// Fallback for countries without a direct "List of largest..." URL
		// We might try "List_of_companies_of_[Country]"
		return nil, fmt.Errorf("404 or error")
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	var companies []string
	// Generic scraper for Wikipedia lists of companies
	doc.Find("table.wikitable tbody tr").Each(func(i int, s *goquery.Selection) {
		if len(companies) >= 5 { return }
		
		// Usually Name is in the first or second column
		name := s.Find("td").Eq(0).Find("a").Text()
		if name == "" {
			name = s.Find("td").Eq(1).Find("a").Text()
		}
		if name == "" {
			name = s.Find("td").Eq(0).Text()
		}

		name = strings.TrimSpace(name)
		// Filter out noise
		if len(name) > 2 && !strings.Contains(name, "[") {
			companies = append(companies, name)
		}
	})

	return companies, nil
}
