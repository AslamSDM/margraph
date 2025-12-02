package social

import (
	"encoding/json"
	"fmt"
	"margraf/graph"
	"margraf/llm"
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
	fmt.Printf("📱 Crawling Real Social Media for topic: '%s'...\n", topic)

	var allPosts []scraper.SocialPost

	// 1. Reddit (Direct API)
	if posts, err := s.Scraper.FetchRedditPosts(topic, 3); err == nil {
		allPosts = append(allPosts, posts...)
	} else {
		fmt.Printf("  ⚠️ Reddit crawl failed: %v\n", err)
	}

	// 2. X/Twitter (via Web Search)
	if posts, err := s.Scraper.FetchWebMentions("X (Twitter)", "twitter.com", topic, 3); err == nil {
		allPosts = append(allPosts, posts...)
	}

	// 3. TikTok (via Web Search)
	if posts, err := s.Scraper.FetchWebMentions("TikTok", "tiktok.com", topic, 3); err == nil {
		allPosts = append(allPosts, posts...)
	}

	// 4. Instagram (via Web Search)
	if posts, err := s.Scraper.FetchWebMentions("Instagram", "instagram.com", topic, 3); err == nil {
		allPosts = append(allPosts, posts...)
	}

	if len(allPosts) == 0 {
		fmt.Println("  No recent posts found on any platform.")
		return
	}

	s.analyzeAndBroadcast(topic, allPosts)
}

func (s *SocialMonitor) analyzeAndBroadcast(topic string, posts []scraper.SocialPost) {
	type Analysis struct {
		Sentiment float64 `json:"sentiment"`
	}

	var totalSentiment float64
	var count float64

	for _, p := range posts {
		// Use LLM to analyze the REAL sentiment
		prompt := fmt.Sprintf(`
Analyze the sentiment of this social media post about "%s".
Content: "%s"
Return ONLY a JSON object: {"sentiment": -0.8} (Range -1.0 to 1.0)
`, topic, p.Content)

		resp, err := s.Client.Complete(prompt)
		if err != nil {
			continue
		}

		var analysis Analysis
		cleaned := cleanJSON(resp)
		json.Unmarshal([]byte(cleaned), &analysis)

		comment := SocialComment{
			Platform:  Platform(p.Platform),
			User:      p.User,
			Content:   p.Content,
			Sentiment: analysis.Sentiment,
			URL:       p.URL,
		}

		fmt.Printf("    [%s] %s: %.2f (URL: %s)\n", comment.Platform, comment.User, comment.Sentiment, comment.URL)
		s.Hub.Broadcast("social_pulse", comment)
		
		totalSentiment += analysis.Sentiment
		count++
	}

	if count > 0 {
		avgSentiment := totalSentiment / count
		s.applySentimentToGraph(topic, avgSentiment)
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
		fmt.Printf("    📉 Social Sentiment Impact: %s health adjusted by %.3f -> %.3f\n", topic, impact, newHealth)
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
