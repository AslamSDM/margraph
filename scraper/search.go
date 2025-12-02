package scraper

import (
	"fmt"
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

// WebSearcher handles searching the web.
type WebSearcher struct {
	Client *http.Client
}

func NewWebSearcher() *WebSearcher {
	return &WebSearcher{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SearchDuckDuckGo performs a web search and returns top results.
// We use DDG html version because it is easier to scrape than Google.
func (s *WebSearcher) Search(query string) ([]SearchResult, error) {
	// Use html.duckduckgo.com for lighter non-JS version
	baseURL := "https://html.duckduckgo.com/html/"
	
	vals := url.Values{}
	vals.Add("q", query)
	
	req, err := http.NewRequest("POST", baseURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.114 Safari/537.36")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		if len(results) >= 5 {
			return
		}
		
		title := s.Find(".result__title a").Text()
		link, _ := s.Find(".result__title a").Attr("href")
		snippet := s.Find(".result__snippet").Text()

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
