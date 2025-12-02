package news

import (
	"encoding/json"
	"fmt"
	"margraf/discovery"
	"margraf/graph"
	"margraf/llm"
	"margraf/logger"
	"margraf/server"
	"margraf/simulation"
	"margraf/social"
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
	EntityName      string   `json:"entity"`
	EntityType      string   `json:"type"`
	ImpactScore     float64  `json:"impact"`
	Reason          string   `json:"reason"`
	IsNewEntitiy    bool     `json:"is_new"`
	RelatedEntities []string `json:"related_entities,omitempty"`
	SentimentScore  float64  `json:"sentiment,omitempty"`
}

func (e *Engine) Monitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	logger.Info(logger.StatusNews, "News Monitor active. Polling %s every %v...", e.FeedURL, interval)

	for range ticker.C {
		e.FetchAndProcess()
	}
}

func (e *Engine) FetchAndProcess() {
	logger.Info(logger.StatusNews, "Checking for news...")
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
	logger.InfoDepth(1, logger.StatusNews, "Analyzing: %s", item.Title)
	e.Hub.Broadcast("news_alert", item.Title)
	
	prompt := fmt.Sprintf(`
Analyze this financial news headline: "%s"
Identify:
1. The MAIN entity involved (Nation, Corporation, or RawMaterial)
2. The economic impact score (-1.0 for catastrophic, 0.0 for neutral, 1.0 for boom)
3. Any related entities mentioned (up to 3 other companies, nations, or commodities)
4. The overall sentiment score (-1.0 to 1.0)

Return ONLY a JSON object with this exact format:
{"entity": "EntityName", "type": "Nation", "impact": -0.5, "reason": "Brief reason", "related_entities": ["Entity1", "Entity2"], "sentiment": 0.5}
`, item.Title)

	resp, err := e.Client.Complete(prompt)
	if err != nil {
		logger.ErrorDepth(2, logger.StatusErr, "LLM Error: %v", err)
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
		logger.InfoDepth(2, logger.StatusNew, "New Entity Discovered in News: %s. Triggering Recursive Seeder...", impact.EntityName)
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
				logger.InfoDepth(2, logger.StatusChk, "Expanding Knowledge Graph for new nation: %s...", name)
				if err := e.Seeder.ProcessNation(e.Graph, name, 0); err != nil {
					logger.WarnDepth(2, logger.StatusWarn, "Failed to expand nation %s: %v", name, err)
				}
			}(impact.EntityName)
		}

	} else {
		logger.SuccessDepth(2, "Entity Found: %s", node.Name)
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

	// Update edge weights based on news sentiment
	e.updateEdgeWeightsFromNews(id, impact, item.Title)
}

// updateEdgeWeightsFromNews updates weights of edges connected to the affected entity
func (e *Engine) updateEdgeWeightsFromNews(entityID string, impact NewsImpact, newsTitle string) {
	// Get all outgoing edges from the entity
	outgoingEdges := e.Graph.GetOutgoingEdges(entityID)

	// Determine relevance score based on news credibility (BBC is high credibility)
	relevanceScore := 0.8

	// Use sentiment score if provided, otherwise derive from impact
	sentimentScore := impact.SentimentScore
	if sentimentScore == 0 && impact.ImpactScore != 0 {
		sentimentScore = impact.ImpactScore
	}

	eventID := fmt.Sprintf("news_%d", time.Now().Unix())

	// Update weights for all outgoing edges
	for _, edge := range outgoingEdges {
		err := e.Graph.UpdateEdgeWeight(
			edge.SourceID,
			edge.TargetID,
			edge.Type,
			sentimentScore,
			relevanceScore,
			eventID,
		)
		if err != nil {
			logger.WarnDepth(2, logger.StatusWarn, "Failed to update edge weight: %v", err)
		} else {
			logger.SuccessDepth(2, "Updated edge %s->%s weight based on news", edge.SourceID, edge.TargetID)
		}
	}

	// Also update edges to related entities if they exist
	for _, relatedEntity := range impact.RelatedEntities {
		relatedID := cleanID(relatedEntity)

		// Check if this entity exists in the graph
		if _, exists := e.Graph.GetNode(relatedID); !exists {
			continue
		}

		// Try to find edges between the main entity and related entities
		for _, edge := range e.Graph.GetOutgoingEdges(entityID) {
			if edge.TargetID == relatedID {
				err := e.Graph.UpdateEdgeWeight(
					edge.SourceID,
					edge.TargetID,
					edge.Type,
					sentimentScore * 0.7, // Reduced impact for related entities
					relevanceScore,
					eventID,
				)
				if err == nil {
					logger.SuccessDepth(2, "Updated related edge %s->%s", edge.SourceID, edge.TargetID)
				}
			}
		}

		// Also check reverse direction
		for _, edge := range e.Graph.GetOutgoingEdges(relatedID) {
			if edge.TargetID == entityID {
				err := e.Graph.UpdateEdgeWeight(
					edge.SourceID,
					edge.TargetID,
					edge.Type,
					sentimentScore * 0.7,
					relevanceScore,
					eventID,
				)
				if err == nil {
					logger.SuccessDepth(2, "Updated related edge %s->%s", edge.SourceID, edge.TargetID)
				}
			}
		}
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
