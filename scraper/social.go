package scraper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SocialScraper struct {
	Client      *http.Client
	WebSearcher *WebSearcher
}

func NewSocialScraper() *SocialScraper {
	return &SocialScraper{
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
		WebSearcher: NewWebSearcher(),
	}
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
	encoded := url.QueryEscape(topic)
	apiURL := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&sort=new&limit=%d", encoded, limit)
	
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	// User-Agent is CRITICAL for Reddit
	req.Header.Set("User-Agent", "Margraf/1.0 (MacOS; Golang bot) EducationalProject")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("reddit api status: %d", resp.StatusCode)
	}

	var listing RedditListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, err
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

// FetchWebMentions finds content on specific social platforms using web search.
func (s *SocialScraper) FetchWebMentions(platformName, domain, topic string, limit int) ([]SocialPost, error) {
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
		
		// Heuristic to extract user from title if possible (e.g. "User (@handle) on TikTok")
		user := "Unknown"
		if strings.Contains(res.Title, "@") {
			parts := strings.Split(res.Title, "@")
			if len(parts) > 1 {
				user = "@" + strings.Split(parts[1], " ")[0] // simple parser
			}
		}

		posts = append(posts, SocialPost{
			Platform: platformName,
			User:     user,
			Content:  res.Snippet, // Use snippet as proxy for content
			URL:      res.Link,
			Time:     time.Now(), // Search results don't always have clear dates
		})
	}
	return posts, nil
}
