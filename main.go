package main

import (
	"bufio"
	"fmt"
	"margraf/config"
	"margraf/discovery"
	"margraf/graph"
	"margraf/llm"
	"margraf/logger"
	"margraf/news"
	"margraf/server"
	"margraf/simulation"
	"margraf/social"
	"os"
	"strings"
	"time"
)

func main() {
	loadEnv()
	
	if err := config.Load(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger with config settings
	logger.Init(config.Global.Logging.Level, config.Global.Logging.EnableColors)

	logger.Section(fmt.Sprintf("%s v%s", config.Global.App.Name, config.Global.App.Version))
	logger.Plain("Financial Dynamic Knowledge Graph - Real-time Trade Disruption Analysis")

	// 1. Setup
	var g *graph.Graph
	graphFile := "margraf_graph.json"

	// Try to load existing graph first
	if _, err := os.Stat(graphFile); err == nil {
		logger.Info(logger.StatusInit, "Found existing graph file: %s", graphFile)
		logger.Info(logger.StatusInit, "Loading saved graph...")
		loadedGraph, err := graph.Load(graphFile)
		if err != nil {
			logger.Warn(logger.StatusWarn, "Failed to load graph: %v", err)
			logger.Info(logger.StatusInit, "Creating new graph instead...")
			g = graph.NewGraph()
		} else {
			g = loadedGraph
			logger.Success("Graph loaded successfully: %s", g.String())
			logger.Info(logger.StatusInit, "Tip: Use 'show' to see loaded nodes and edges")
		}
	} else {
		logger.Info(logger.StatusInit, "No existing graph found. Creating new graph...")
		g = graph.NewGraph()
	}

	g.EnableAutoSave(graphFile, 10) // Auto-save every 10 changes
	client := llm.NewClient()
	seeder := discovery.NewSeeder(client)

	// 1b. Setup Websocket Server & Social Monitor
	hub := server.NewHub()
	go hub.Run()
	server.StartServer(hub, config.Global.Server.Port)

	socialMonitor := social.NewMonitor(client, hub, g)
	marketMonitor := simulation.NewMarketMonitor(g, hub)

	// 2. Discovery Phase - Only run seeder if graph is empty or user wants to reseed
	if len(g.Nodes) == 0 {
		logger.Info(logger.StatusInit, "Empty graph detected. Initializing via LLM/API...")
		if err := seeder.Seed(g); err != nil {
			logger.Error(logger.StatusErr, "Error seeding graph: %v", err)
		}
		logger.Success("Graph Ready: %s", g.String())
	} else {
		logger.Success("Using existing graph with %d nodes and %d edges", len(g.Nodes), len(g.Edges))
		logger.Info(logger.StatusInit, "Use 'reseed' command to rebuild graph from scratch")
	}

	// 3. Interactive or Demo Mode
	reader := bufio.NewReader(os.Stdin)
	sim := simulation.NewSimulator(g)
	
	// 4. Start Engines
	newsEngine := news.NewEngine(g, client, seeder, sim, hub, socialMonitor)

	newsInterval := time.Duration(config.Global.News.PollInterval) * time.Second
	marketInterval := time.Duration(config.Global.Market.PollInterval) * time.Second

	// Start temporal decay worker (applies decay every 30 minutes with lambda=0.05)
	g.StartTemporalDecayWorker(30*time.Minute, 0.05)
	logger.Info(logger.StatusInit, "Temporal decay worker started (λ=0.05, interval=30min)")

	go newsEngine.Monitor(newsInterval)
	go marketMonitor.Start(marketInterval)
	
	// Broadcast Graph Pulse (Keep UI in sync)
	go func() {
		for range time.Tick(2 * time.Second) {
			graphJSON, err := g.ToJSON()
			if err != nil {
				logger.Warn(logger.StatusWarn, "Error converting graph to JSON: %v", err)
				continue
			}
			hub.Broadcast("graph_update", graphJSON)
		}
	}()
	
	// AutoSave (Every 5 mins)
	go func() {
		for range time.Tick(5 * time.Minute) {
			if err := g.Save("margraf_autosave.json"); err != nil {
				logger.Error(logger.StatusErr, "AutoSave Failed: %v", err)
			}
		}
	}()

	for {
		logger.Plain("")
		logger.Section("Available Commands")
		logger.Plain("  show          - Show all nodes and edges")
		logger.Plain("  shock <ID>    - Simulate a trade ban/shock on a Node ID")
		logger.Plain("  boost <ID>    - Simulate positive news boost for a Node ID")
		logger.Plain("  news          - Force check for latest news")
		logger.Plain("  simulate <ID> <sentiment> - Test news impact (sentiment: -1.0 to 1.0)")
		logger.Plain("  reseed        - Rebuild graph from scratch (WARNING: loses all data)")
		logger.Plain("  social <T>    - Crawl real social media for Topic T")
		logger.Plain("  save <F>      - Save graph to file F")
		logger.Plain("  load <F>      - Load graph from file F")
		logger.Plain("  export <F>    - Export graph to DOT file F")
		logger.Plain("  exit          - Quit")
		fmt.Print("> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		parts := strings.Split(input, " ")

		switch parts[0] {
		case "show":
			printGraph(g)
		case "shock":
			if len(parts) < 2 {
				logger.Warn(logger.StatusWarn, "Usage: shock <NodeID> (e.g., shock india)")
				continue
			}
			targetID := parts[1]
			sim.RunShock(simulation.ShockEvent{
				TargetNodeID: targetID,
				Description:  "Trade Ban / Supply Chain Failure",
				ImpactFactor: 0.1, // 90% reduction
			})
			// Also update edge weights negatively
			updateEdgesForTest(g, targetID, -0.8, "Negative shock simulation")
		case "boost":
			if len(parts) < 2 {
				logger.Warn(logger.StatusWarn, "Usage: boost <NodeID> (e.g., boost india)")
				continue
			}
			targetID := parts[1]
			sim.RunShock(simulation.ShockEvent{
				TargetNodeID: targetID,
				Description:  "Positive Economic Boom / Trade Agreement",
				ImpactFactor: 1.5, // 50% increase
			})
			hub.Broadcast("shock_event", map[string]interface{}{
				"type":   "boost",
				"target": targetID,
				"impact": 1.5,
			})
			// Update edge weights positively
			updateEdgesForTest(g, targetID, 0.8, "Positive boost simulation")
		case "simulate":
			if len(parts) < 3 {
				logger.Warn(logger.StatusWarn, "Usage: simulate <NodeID> <sentiment> (e.g., simulate india 0.5)")
				continue
			}
			targetID := parts[1]
			sentiment := 0.0
			fmt.Sscanf(parts[2], "%f", &sentiment)
			if sentiment < -1.0 || sentiment > 1.0 {
				logger.Warn(logger.StatusWarn, "Sentiment must be between -1.0 and 1.0")
				continue
			}

			// Apply to node health
			impactFactor := 1.0 + sentiment
			sim.RunShock(simulation.ShockEvent{
				TargetNodeID: targetID,
				Description:  fmt.Sprintf("Simulated news event (sentiment: %.2f)", sentiment),
				ImpactFactor: impactFactor,
			})

			// Update edge weights
			updateEdgesForTest(g, targetID, sentiment, fmt.Sprintf("Test simulation (%.2f)", sentiment))
			logger.Success("Simulated news event for %s with sentiment %.2f", targetID, sentiment)
		case "news":
			newsEngine.FetchAndProcess()
		case "reseed":
			logger.Warn(logger.StatusWarn, "WARNING: This will delete all current graph data!")
			fmt.Print("Are you sure? (yes/no): ")
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			if confirm == "yes" {
				logger.Info(logger.StatusInit, "Clearing graph and reseeding...")
				g = graph.NewGraph()
				g.EnableAutoSave(graphFile, 10)
				if err := seeder.Seed(g); err != nil {
					logger.Error(logger.StatusErr, "Error seeding graph: %v", err)
				} else {
					logger.Success("Graph rebuilt: %s", g.String())
				}
			} else {
				logger.Info(logger.StatusInit, "Reseed cancelled")
			}
		case "social":
			if len(parts) < 2 {
				logger.Warn(logger.StatusWarn, "Usage: social <Topic>")
				continue
			}
			topic := strings.Join(parts[1:], " ")
			go socialMonitor.CrawlReal(topic)
		case "save":
			if len(parts) < 2 {
				logger.Warn(logger.StatusWarn, "Usage: save <filename.json>")
				continue
			}
			if err := g.Save(parts[1]); err != nil {
				logger.Error(logger.StatusErr, "Error saving graph: %v", err)
			} else {
				logger.Success("Graph saved to %s", parts[1])
			}
		case "load":
			if len(parts) < 2 {
				logger.Warn(logger.StatusWarn, "Usage: load <filename.json>")
				continue
			}
			newG, err := graph.Load(parts[1])
			if err != nil {
				logger.Error(logger.StatusErr, "Error loading graph: %v", err)
			} else {
				g.Replace(newG)
				logger.Success("Graph loaded from %s (%s)", parts[1], g.String())
			}
		case "export":
			if len(parts) < 2 {
				logger.Warn(logger.StatusWarn, "Usage: export <filename.dot>")
				continue
			}
			if err := os.WriteFile(parts[1], []byte(g.ToDOT()), 0644); err != nil {
				logger.Error(logger.StatusErr, "Error exporting DOT: %v", err)
			} else {
				logger.Success("Graph exported to %s", parts[1])
			}
		case "exit":
			logger.Plain("Goodbye.")
			return
		default:
			logger.Warn(logger.StatusWarn, "Unknown command")
		}
	}
}

func loadEnv() {
	file, err := os.Open(".env")
	if err != nil {
		// .env file is optional in some environments, so we just return if not found
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove inline comments
		if idx := strings.Index(value, "#"); idx != -1 {
			value = strings.TrimSpace(value[:idx])
		}

		// Remove quotes
		value = strings.Trim(value, `"'`)

		os.Setenv(key, value)
	}
}

func printGraph(g *graph.Graph) {
	logger.Plain("")
	logger.Section("Nodes")
	for _, n := range g.Nodes {
		logger.Plain("[%s] %s (%s) - Health: %.2f", n.ID, n.Name, n.Type, n.Health)
	}
	logger.Plain("")
	logger.Section("Edges")
	for _, e := range g.Edges {
		logger.Plain("%s --(%.2f)--> %s [%s] Status: %s", e.SourceID, e.Weight, e.TargetID, e.Type, e.Status)
	}
}

// updateEdgesForTest updates edge weights for testing purposes
func updateEdgesForTest(g *graph.Graph, nodeID string, sentiment float64, reason string) {
	// Get all outgoing edges
	outgoingEdges := g.GetOutgoingEdges(nodeID)
	relevance := 0.9 // High relevance for test events
	eventID := fmt.Sprintf("test_%d", time.Now().Unix())

	updatedCount := 0
	for _, edge := range outgoingEdges {
		err := g.UpdateEdgeWeight(
			edge.SourceID,
			edge.TargetID,
			edge.Type,
			sentiment,
			relevance,
			eventID,
		)
		if err != nil {
			logger.Warn(logger.StatusWarn, "Failed to update edge: %v", err)
		} else {
			updatedCount++
		}
	}

	if updatedCount > 0 {
		logger.Success("Updated %d edge weights for node %s (%s)", updatedCount, nodeID, reason)
	} else {
		logger.Warn(logger.StatusWarn, "No edges found for node %s", nodeID)
	}
}
