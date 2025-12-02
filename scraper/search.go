package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// SearchResult represents a single entry from a search engine.
type SearchResult struct {
	Title   string
	Link    string
	Snippet string
}

// WebSearcher handles searching the web with multiple fallback methods.
type WebSearcher struct {
	Client        *http.Client
	lastRequestAt time.Time
	requestCount  int
}

func NewWebSearcher() *WebSearcher {
	return &WebSearcher{
		Client: &http.Client{
			Timeout: 15 * time.Second,
		},
		lastRequestAt: time.Time{},
		requestCount:  0,
	}
}

// rateLimit applies a simple rate limiting mechanism
func (s *WebSearcher) rateLimit() {
	// Wait at least 1 second between requests to avoid being blocked
	if !s.lastRequestAt.IsZero() {
		elapsed := time.Since(s.lastRequestAt)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}
	s.lastRequestAt = time.Now()
	s.requestCount++
}

// Search performs a web search using multiple methods with fallbacks
func (s *WebSearcher) Search(query string) ([]SearchResult, error) {
	s.rateLimit()

	// Try Wikipedia API first for entity searches
	if strings.Contains(strings.ToLower(query), "companies") ||
	   strings.Contains(strings.ToLower(query), "industries") ||
	   strings.Contains(strings.ToLower(query), "wikipedia") {
		results, err := s.searchWikipedia(query)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}

	// Try DuckDuckGo with retry
	for attempt := 0; attempt < 2; attempt++ {
		results, err := s.searchDuckDuckGo(query)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if attempt < 1 {
			time.Sleep(time.Second * 2)
		}
	}

	// Fallback to direct Wikipedia search
	results, err := s.searchWikipediaFallback(query)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	return nil, fmt.Errorf("all search methods failed")
}

// searchWikipedia searches Wikipedia API directly
func (s *WebSearcher) searchWikipedia(query string) ([]SearchResult, error) {
	// Extract main search term
	searchTerm := strings.TrimSpace(query)
	searchTerm = strings.ReplaceAll(searchTerm, "wikipedia", "")
	searchTerm = strings.TrimSpace(searchTerm)

	apiURL := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=opensearch&search=%s&limit=5&format=json",
		url.QueryEscape(searchTerm))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MargrafBot/1.0 (Educational Research)")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wikipedia api status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Wikipedia OpenSearch returns: [query, [titles], [descriptions], [urls]]
	var apiResponse []interface{}
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, err
	}

	if len(apiResponse) < 4 {
		return nil, fmt.Errorf("unexpected wikipedia response format")
	}

	titles, ok1 := apiResponse[1].([]interface{})
	descriptions, ok2 := apiResponse[2].([]interface{})
	urls, ok3 := apiResponse[3].([]interface{})

	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("failed to parse wikipedia response")
	}

	var results []SearchResult
	for i := 0; i < len(titles) && i < 5; i++ {
		title, _ := titles[i].(string)
		desc, _ := descriptions[i].(string)
		link, _ := urls[i].(string)

		if title != "" {
			results = append(results, SearchResult{
				Title:   title,
				Link:    link,
				Snippet: desc,
			})
		}
	}

	return results, nil
}

// searchWikipediaFallback does a direct HTML search on Wikipedia
func (s *WebSearcher) searchWikipediaFallback(query string) ([]SearchResult, error) {
	searchTerm := strings.TrimSpace(query)
	searchTerm = strings.ReplaceAll(searchTerm, "wikipedia", "")
	searchURL := fmt.Sprintf("https://en.wikipedia.org/w/index.php?search=%s", url.QueryEscape(searchTerm))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MargrafBot/1.0 (Educational Research)")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	doc.Find(".mw-search-result").Each(func(i int, sel *goquery.Selection) {
		if len(results) >= 5 {
			return
		}

		title := sel.Find(".mw-search-result-heading a").Text()
		link, _ := sel.Find(".mw-search-result-heading a").Attr("href")
		snippet := sel.Find(".searchresult").Text()

		if title != "" && link != "" {
			fullLink := "https://en.wikipedia.org" + link
			results = append(results, SearchResult{
				Title:   strings.TrimSpace(title),
				Link:    fullLink,
				Snippet: strings.TrimSpace(snippet),
			})
		}
	})

	return results, nil
}

// searchDuckDuckGo performs a DuckDuckGo search
func (s *WebSearcher) searchDuckDuckGo(query string) ([]SearchResult, error) {
	baseURL := "https://html.duckduckgo.com/html/"

	vals := url.Values{}
	vals.Add("q", query)

	req, err := http.NewRequest("POST", baseURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ddg status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	doc.Find(".result").Each(func(i int, sel *goquery.Selection) {
		if len(results) >= 5 {
			return
		}

		title := sel.Find(".result__title a").Text()
		link, _ := sel.Find(".result__title a").Attr("href")
		snippet := sel.Find(".result__snippet").Text()

		if title != "" && link != "" {
			results = append(results, SearchResult{
				Title:   strings.TrimSpace(title),
				Link:    link,
				Snippet: strings.TrimSpace(snippet),
			})
		}
	})

	return results, nil
}
