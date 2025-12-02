package news

import (
	"encoding/json"
	"fmt"
	"margraf/discovery"
	"margraf/graph"
	"margraf/llm"
	"margraf/server"
	"margraf/simulation"
	"margraf/social" // We need to add this import, but circular dependency might be an issue if social imports news.
	// Wait, news imports social. social imports server. server is independent. OK.
	"strings"
	"time"
)

type Engine struct {
	Graph     *graph.Graph
	Client    *llm.Client
	Seeder    *discovery.Seeder
	Simulator *simulation.Simulator
	Hub       *server.Hub
	Social    *social.SocialMonitor
	FeedURL   string
	LastCheck time.Time
}

func NewEngine(g *graph.Graph, c *llm.Client, s *discovery.Seeder, sim *simulation.Simulator, h *server.Hub, soc *social.SocialMonitor) *Engine {
	return &Engine{
		Graph:     g,
		Client:    c,
		Seeder:    s,
		Simulator: sim,
		Hub:       h,
		Social:    soc,
		FeedURL:   "http://feeds.bbci.co.uk/news/business/rss.xml",
		LastCheck: time.Now().Add(-24 * time.Hour),
	}
}

type NewsImpact struct {
	EntityName   string  `json:"entity"`
	EntityType   string  `json:"type"`
	ImpactScore  float64 `json:"impact"`
	Reason       string  `json:"reason"`
	IsNewEntitiy bool    `json:"is_new"`
}

func (e *Engine) Monitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	fmt.Printf("📡 News Monitor active. Polling %s every %v...\n", e.FeedURL, interval)
	
	for range ticker.C {
		e.FetchAndProcess()
	}
}

func (e *Engine) FetchAndProcess() {
	fmt.Println("📡 Checking for news...")
	items, err := FetchRSS(e.FeedURL)
	if err != nil {
		fmt.Printf("Error fetching RSS: %v\n", err)
		return
	}

	count := 0
	for _, item := range items {
		if count >= 3 {
			break
		}
		
		pubDate, _ := time.Parse(time.RFC1123, item.PubDate)
		if pubDate.Before(e.LastCheck) {
			continue
		}
		
		e.processItem(item)
		count++
	}
	e.LastCheck = time.Now()
}

func (e *Engine) processItem(item RSSItem) {
	fmt.Printf("  📰 Analyzing: %s\n", item.Title)
	e.Hub.Broadcast("news_alert", item.Title)
	
	prompt := fmt.Sprintf(`
Analyze this financial news headline: "%s"
Identify the MAIN entity involved (Nation, Corporation, or RawMaterial).
Determine the economic impact score (-1.0 for catastrophic, 0.0 for neutral, 1.0 for boom).
Return ONLY a JSON object: {"entity": "EntityName", "type": "Nation", "impact": -0.5, "reason": "Brief reason"}
`, item.Title)

	resp, err := e.Client.Complete(prompt)
	if err != nil {
		fmt.Printf("    ❌ LLM Error: %v\n", err)
		return
	}

	var impact NewsImpact
	cleaned := cleanJSON(resp)
	if err := json.Unmarshal([]byte(cleaned), &impact); err != nil {
		return
	}
	
	// 1. Trigger Social Crawler (Real)
	go e.Social.CrawlReal(item.Title)

	id := cleanID(impact.EntityName)
	node, exists := e.Graph.GetNode(id)

	if !exists {
		fmt.Printf("    🆕 New Entity Discovered in News: %s. Triggering Recursive Seeder...\n", impact.EntityName)
		e.Hub.Broadcast("graph_update", fmt.Sprintf("New Node: %s", impact.EntityName))

		var nodeType graph.NodeType
		switch strings.ToLower(impact.EntityType) {
		case "nation", "country":
			nodeType = graph.NodeTypeNation
		case "corporation", "company":
			nodeType = graph.NodeTypeCorporation
		case "rawmaterial", "commodity":
			nodeType = graph.NodeTypeRawMaterial
		default:
			nodeType = graph.NodeTypeProduct
		}
		
		newNode := &graph.Node{ID: id, Type: nodeType, Name: impact.EntityName}
		e.Graph.AddNode(newNode)

		if nodeType == graph.NodeTypeNation {
			go func(name string) {
				fmt.Printf("    🔍 Expanding Knowledge Graph for new nation: %s...\n", name)
				if err := e.Seeder.ProcessNation(e.Graph, name, 0); err != nil {
					fmt.Printf("    ⚠️ Failed to expand nation %s: %v\n", name, err)
				}
			}(impact.EntityName)
		}
		
	} else {
		fmt.Printf("    ✅ Entity Found: %s\n", node.Name)
	}

	if impact.ImpactScore != 0 {
		evt := simulation.ShockEvent{
			TargetNodeID: id,
			Description:  fmt.Sprintf("News: %s (%s)", impact.Reason, item.Title),
			ImpactFactor: 1.0 + impact.ImpactScore,
		}
		e.Simulator.RunShock(evt)
		e.Hub.Broadcast("shock_event", evt)
	}
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func cleanID(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
}
