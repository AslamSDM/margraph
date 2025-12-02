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

type SocialScraper struct {
	Client         *http.Client
	WebSearcher    *WebSearcher
	lastRequestAt  time.Time
	redditRequests int
}

func NewSocialScraper() *SocialScraper {
	return &SocialScraper{
		Client: &http.Client{
			Timeout: 15 * time.Second,
		},
		WebSearcher:    NewWebSearcher(),
		lastRequestAt:  time.Time{},
		redditRequests: 0,
	}
}

// rateLimit ensures we don't hammer APIs
func (s *SocialScraper) rateLimit(minDelay time.Duration) {
	if !s.lastRequestAt.IsZero() {
		elapsed := time.Since(s.lastRequestAt)
		if elapsed < minDelay {
			time.Sleep(minDelay - elapsed)
		}
	}
	s.lastRequestAt = time.Now()
}

type RedditListing struct {
	Data struct {
		Children []struct {
			Data struct {
				Title     string  `json:"title"`
				Selftext  string  `json:"selftext"`
				Author    string  `json:"author"`
				Subreddit string  `json:"subreddit"`
				Created   float64 `json:"created_utc"`
				Permalink string  `json:"permalink"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type SocialPost struct {
	Platform string
	User     string
	Content  string
	URL      string
	Time     time.Time
}

// FetchRedditPosts searches Reddit for a topic and returns recent posts.
func (s *SocialScraper) FetchRedditPosts(topic string, limit int) ([]SocialPost, error) {
	s.rateLimit(2 * time.Second) // Reddit requires 2s between requests
	s.redditRequests++

	encoded := url.QueryEscape(topic)
	apiURL := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&sort=new&limit=%d&t=week", encoded, limit)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Reddit requires a unique, descriptive User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MargrafBot/2.0; +Educational Research)")
	req.Header.Set("Accept", "application/json")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("reddit rate limited (too many requests)")
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reddit api status %d: %s", resp.StatusCode, string(body))
	}

	var listing RedditListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, fmt.Errorf("reddit json decode error: %w", err)
	}

	var posts []SocialPost
	for _, child := range listing.Data.Children {
		d := child.Data
		content := d.Title
		if len(d.Selftext) > 0 {
			if len(d.Selftext) > 200 {
				content += " - " + d.Selftext[:200] + "..."
			} else {
				content += " - " + d.Selftext
			}
		}

		// Skip if content is too short or looks like spam
		if len(content) < 10 {
			continue
		}

		posts = append(posts, SocialPost{
			Platform: "Reddit",
			User:     "u/" + d.Author,
			Content:  content,
			URL:      "https://reddit.com" + d.Permalink,
			Time:     time.Unix(int64(d.Created), 0),
		})
	}

	return posts, nil
}

// FetchHackerNewsPosts searches Hacker News using Algolia API
func (s *SocialScraper) FetchHackerNewsPosts(topic string, limit int) ([]SocialPost, error) {
	s.rateLimit(1 * time.Second)

	encoded := url.QueryEscape(topic)
	apiURL := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=(story,comment)&hitsPerPage=%d", encoded, limit)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MargrafBot/2.0")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hacker news request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hn api status: %d", resp.StatusCode)
	}

	var hnResponse struct {
		Hits []struct {
			Title      string    `json:"title"`
			Author     string    `json:"author"`
			CommentText string   `json:"comment_text"`
			StoryText  string    `json:"story_text"`
			CreatedAt  time.Time `json:"created_at"`
			ObjectID   string    `json:"objectID"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&hnResponse); err != nil {
		return nil, fmt.Errorf("hn json decode error: %w", err)
	}

	var posts []SocialPost
	for _, hit := range hnResponse.Hits {
		content := hit.Title
		if content == "" {
			content = hit.CommentText
		}
		if content == "" {
			content = hit.StoryText
		}

		// Skip empty or very short content
		if len(content) < 15 {
			continue
		}

		// Limit content length
		if len(content) > 300 {
			content = content[:300] + "..."
		}

		posts = append(posts, SocialPost{
			Platform: "Hacker News",
			User:     hit.Author,
			Content:  content,
			URL:      fmt.Sprintf("https://news.ycombinator.com/item?id=%s", hit.ObjectID),
			Time:     hit.CreatedAt,
		})
	}

	return posts, nil
}

// FetchTwitterViaNitter uses Nitter (Twitter frontend) to get tweets without API
func (s *SocialScraper) FetchTwitterViaNitter(topic string, limit int) ([]SocialPost, error) {
	s.rateLimit(2 * time.Second)

	// Try multiple Nitter instances in case one is down
	nitterInstances := []string{
		"nitter.net",
		"nitter.poast.org",
		"nitter.privacydev.net",
	}

	var lastErr error
	for _, instance := range nitterInstances {
		posts, err := s.fetchFromNitterInstance(instance, topic, limit)
		if err == nil && len(posts) > 0 {
			return posts, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("all nitter instances failed: %w", lastErr)
}

func (s *SocialScraper) fetchFromNitterInstance(instance, topic string, limit int) ([]SocialPost, error) {
	searchURL := fmt.Sprintf("https://%s/search?f=tweets&q=%s", instance, url.QueryEscape(topic))

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("nitter status: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var posts []SocialPost
	doc.Find(".timeline-item").Each(func(i int, sel *goquery.Selection) {
		if len(posts) >= limit {
			return
		}

		username := sel.Find(".username").Text()
		tweetText := sel.Find(".tweet-content").Text()
		tweetLink, _ := sel.Find(".tweet-link").Attr("href")

		if len(tweetText) > 10 {
			posts = append(posts, SocialPost{
				Platform: "Twitter/X",
				User:     strings.TrimSpace(username),
				Content:  strings.TrimSpace(tweetText),
				URL:      "https://" + instance + tweetLink,
				Time:     time.Now(),
			})
		}
	})

	return posts, nil
}

// FetchYouTubeComments searches YouTube for videos and extracts comments from description
func (s *SocialScraper) FetchYouTubeComments(topic string, limit int) ([]SocialPost, error) {
	s.rateLimit(1 * time.Second)

	// Use web search to find YouTube videos
	query := fmt.Sprintf("site:youtube.com %s", topic)
	results, err := s.WebSearcher.Search(query)
	if err != nil {
		return nil, err
	}

	var posts []SocialPost
	for _, res := range results {
		if len(posts) >= limit {
			break
		}

		if strings.Contains(res.Link, "youtube.com/watch") {
			posts = append(posts, SocialPost{
				Platform: "YouTube",
				User:     "Video",
				Content:  res.Title + " - " + res.Snippet,
				URL:      res.Link,
				Time:     time.Now(),
			})
		}
	}

	return posts, nil
}

// FetchWebMentions finds content on specific social platforms using web search.
func (s *SocialScraper) FetchWebMentions(platformName, domain, topic string, limit int) ([]SocialPost, error) {
	s.rateLimit(1 * time.Second)

	query := fmt.Sprintf("site:%s \"%s\"", domain, topic)
	results, err := s.WebSearcher.Search(query)
	if err != nil {
		return nil, err
	}

	var posts []SocialPost
	for i, res := range results {
		if i >= limit {
			break
		}

		// Heuristic to extract user from title if possible
		user := "User"
		if strings.Contains(res.Title, "@") {
			parts := strings.Split(res.Title, "@")
			if len(parts) > 1 {
				user = "@" + strings.Fields(parts[1])[0]
			}
		}

		// Skip if snippet is too short
		if len(res.Snippet) < 10 {
			continue
		}

		posts = append(posts, SocialPost{
			Platform: platformName,
			User:     user,
			Content:  res.Snippet,
			URL:      res.Link,
			Time:     time.Now(),
		})
	}
	return posts, nil
}
