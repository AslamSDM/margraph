package social

import (
	"encoding/json"
	"fmt"
	"margraf/graph"
	"margraf/llm"
	"margraf/logger"
	"margraf/scraper"
	"margraf/server"
	"strings"
)

// Platform represents a social network
type Platform string

const (
	PlatformX        Platform = "X (Twitter)"
	PlatformTikTok   Platform = "TikTok"
	PlatformInstagram Platform = "Instagram"
	PlatformReddit    Platform = "Reddit"
)

type SocialComment struct {
	Platform Platform `json:"platform"`
	User     string   `json:"user"`
	Content  string   `json:"content"`
	Sentiment float64 `json:"sentiment"` // -1.0 to 1.0
	URL      string   `json:"url"`
}

type SocialMonitor struct {
	Client  *llm.Client
	Hub     *server.Hub
	Graph   *graph.Graph
	Scraper *scraper.SocialScraper
}

func NewMonitor(c *llm.Client, h *server.Hub, g *graph.Graph) *SocialMonitor {
	return &SocialMonitor{
		Client:  c,
		Hub:     h,
		Graph:   g,
		Scraper: scraper.NewSocialScraper(),
	}
}

// CrawlReal fetches real social media discussions and analyzes them with AI.
func (s *SocialMonitor) CrawlReal(topic string) {
	logger.Info(logger.StatusSoc, "Crawling Social Media for: '%s'", topic)

	var allPosts []scraper.SocialPost
	sources := 0

	// 1. Hacker News (Most reliable - official API)
	logger.InfoDepth(1, logger.StatusSoc, "Searching Hacker News...")
	if posts, err := s.Scraper.FetchHackerNewsPosts(topic, 3); err == nil && len(posts) > 0 {
		allPosts = append(allPosts, posts...)
		logger.SuccessDepth(2, "Found %d Hacker News posts", len(posts))
		sources++
	} else if err != nil {
		logger.WarnDepth(2, logger.StatusWarn, "Hacker News: %v", err)
	}

	// 2. Reddit (Official JSON API)
	logger.InfoDepth(1, logger.StatusSoc, "Searching Reddit...")
	if posts, err := s.Scraper.FetchRedditPosts(topic, 3); err == nil && len(posts) > 0 {
		allPosts = append(allPosts, posts...)
		logger.SuccessDepth(2, "Found %d Reddit posts", len(posts))
		sources++
	} else if err != nil {
		logger.WarnDepth(2, logger.StatusWarn, "Reddit: %v", err)
	}

	// 3. Twitter/X (via Nitter)
	logger.InfoDepth(1, logger.StatusSoc, "Searching Twitter/X...")
	if posts, err := s.Scraper.FetchTwitterViaNitter(topic, 3); err == nil && len(posts) > 0 {
		allPosts = append(allPosts, posts...)
		logger.SuccessDepth(2, "Found %d tweets", len(posts))
		sources++
	} else if err != nil {
		logger.WarnDepth(2, logger.StatusWarn, "Twitter: %v", err)
	}

	// 4. YouTube (via search)
	logger.InfoDepth(1, logger.StatusSoc, "Searching YouTube...")
	if posts, err := s.Scraper.FetchYouTubeComments(topic, 2); err == nil && len(posts) > 0 {
		allPosts = append(allPosts, posts...)
		logger.SuccessDepth(2, "Found %d YouTube videos", len(posts))
		sources++
	}

	if len(allPosts) == 0 {
		logger.Warn(logger.StatusWarn, "No posts found across any platform for '%s'", topic)
		return
	}

	logger.Success("Collected %d posts from %d sources", len(allPosts), sources)
	s.analyzeAndBroadcast(topic, allPosts)
}

func (s *SocialMonitor) analyzeAndBroadcast(topic string, posts []scraper.SocialPost) {
	type Analysis struct {
		Sentiment float64 `json:"sentiment"`
	}

	var totalSentiment float64
	var count float64

	logger.InfoDepth(1, logger.StatusSoc, "Analyzing sentiment with LLM...")

	for i, p := range posts {
		// Limit content length for LLM
		content := p.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}

		// Use LLM to analyze the REAL sentiment
		prompt := fmt.Sprintf(`
Analyze the sentiment of this social media post about "%s".
Platform: %s
Content: "%s"

Rate the sentiment from -1.0 (very negative) to 1.0 (very positive).
Return ONLY a JSON object: {"sentiment": 0.5}
`, topic, p.Platform, content)

		resp, err := s.Client.Complete(prompt)
		if err != nil {
			logger.WarnDepth(2, logger.StatusWarn, "LLM analysis failed for post %d: %v", i+1, err)
			continue
		}

		var analysis Analysis
		cleaned := cleanJSON(resp)
		if err := json.Unmarshal([]byte(cleaned), &analysis); err != nil {
			logger.WarnDepth(2, logger.StatusWarn, "JSON parse error: %v", err)
			continue
		}

		// Clamp sentiment to valid range
		if analysis.Sentiment < -1.0 {
			analysis.Sentiment = -1.0
		}
		if analysis.Sentiment > 1.0 {
			analysis.Sentiment = 1.0
		}

		comment := SocialComment{
			Platform:  Platform(p.Platform),
			User:      p.User,
			Content:   p.Content,
			Sentiment: analysis.Sentiment,
			URL:       p.URL,
		}

		// Format sentiment with color indicator
		sentimentStr := fmt.Sprintf("%.2f", comment.Sentiment)
		if comment.Sentiment > 0.3 {
			sentimentStr = "+" + sentimentStr + " (Positive)"
		} else if comment.Sentiment < -0.3 {
			sentimentStr = sentimentStr + " (Negative)"
		} else {
			sentimentStr = sentimentStr + " (Neutral)"
		}

		logger.InfoDepth(2, logger.StatusSoc, "[%s] @%s: %s", comment.Platform, comment.User, sentimentStr)
		s.Hub.Broadcast("social_pulse", comment)

		totalSentiment += analysis.Sentiment
		count++
	}

	if count > 0 {
		avgSentiment := totalSentiment / count
		logger.Success("Average sentiment: %.2f across %d posts", avgSentiment, int(count))
		s.applySentimentToGraph(topic, avgSentiment)
	} else {
		logger.Warn(logger.StatusWarn, "No sentiment data collected")
	}
}

func (s *SocialMonitor) applySentimentToGraph(topic string, sentiment float64) {
	// Simple mapping: Topic name -> Node ID
	// In a real system, we'd need Entity Linking (NER) to map "Apple" -> "apple_inc" or "apple_fruit"
	// Here we assume the topic IS the entity name for simplicity.
	id := strings.ToLower(strings.ReplaceAll(topic, " ", "_"))
	
	// Scale sentiment to health impact (e.g. sentiment -0.5 -> health -0.05)
	impact := sentiment * 0.1 
	
	newHealth, ok := s.Graph.UpdateNodeHealth(id, impact)
	if ok {
		logger.InfoDepth(2, logger.StatusTrend, "Social Sentiment Impact: %s health adjusted by %.3f -> %.3f", topic, impact, newHealth)
		s.Hub.Broadcast("graph_update", fmt.Sprintf("Node %s Health: %.2f", topic, newHealth))
	}
}


func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
