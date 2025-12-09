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
	"margraf/tui"
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

	// Initialize TUI
	tuiApp := tui.New()

	// Start TUI in background early so it can receive logs
	go func() {
		if err := tuiApp.Start(); err != nil {
			fmt.Printf("TUI Error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Give TUI a moment to initialize
	time.Sleep(100 * time.Millisecond)

	// Set up logger to write to TUI
	logger.SetOutput(tuiApp.NewWriter())
	logger.SetTUIMode(true)

	logger.Info(logger.StatusInit, "%s v%s", config.Global.App.Name, config.Global.App.Version)
	logger.Info(logger.StatusInit, "Financial Dynamic Knowledge Graph - Real-time Trade Disruption Analysis")

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
	hub.SetGraph(g) // Set graph reference for handling company relations requests
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

		// Migrate existing edges to have directionality
		migrated := g.MigrateEdgeDirectionality()
		if migrated > 0 {
			logger.Info(logger.StatusInit, "Migrated %d edges to have directionality", migrated)
			// Save the migrated graph
			if err := g.Save(graphFile); err != nil {
				logger.Warn(logger.StatusWarn, "Failed to save migrated graph: %v", err)
			} else {
				logger.Success("Saved migrated graph with edge directionality")
			}
		}
	}

	// 3. Setup simulator
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
	// Only broadcast when there are actual changes
	go func() {
		lastNodeCount := len(g.Nodes)
		lastEdgeCount := len(g.Edges)
		lastBroadcast := time.Now()

		for range time.Tick(5 * time.Second) {
			currentNodeCount := len(g.Nodes)
			currentEdgeCount := len(g.Edges)

			// Only broadcast if there are changes or it's been more than 30 seconds
			if currentNodeCount != lastNodeCount ||
			   currentEdgeCount != lastEdgeCount ||
			   time.Since(lastBroadcast) > 30*time.Second {

				graphJSON, err := g.ToJSON()
				if err != nil {
					logger.Warn(logger.StatusWarn, "Error converting graph to JSON: %v", err)
					continue
				}
				hub.Broadcast("graph_update", graphJSON)

				lastNodeCount = currentNodeCount
				lastEdgeCount = currentEdgeCount
				lastBroadcast = time.Now()
			}
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

	// Update TUI stats periodically
	go func() {
		for range time.Tick(2 * time.Second) {
			tuiApp.UpdateStats(len(g.Nodes), len(g.Edges))
		}
	}()

	// Process commands from TUI
	// Handle commands from TUI (blocks until TUI exits)
	for input := range tuiApp.GetCommandChannel() {
		handleCommand(input, g, sim, hub, newsEngine, socialMonitor, graphFile, tuiApp)
	}
}

func handleCommand(input string, g *graph.Graph, sim *simulation.Simulator, hub *server.Hub, newsEngine *news.Engine, socialMon *social.SocialMonitor, graphFile string, tuiApp *tui.TUI) {
	parts := strings.Split(strings.TrimSpace(input), " ")
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "show":
		printGraph(g)
	case "edges":
		printEdgeDirectionality()
	case "discover":
		logger.Info(logger.StatusInit, "Discovering supplier/client relationships...")
		addedEdges := g.DiscoverSupplyChainRelations()
		if addedEdges > 0 {
			logger.Success("Added %d supply chain edges", addedEdges)
			if err := g.Save(graphFile); err != nil {
				logger.Error(logger.StatusErr, "Error saving graph: %v", err)
			} else {
				logger.Success("Graph saved to %s", graphFile)
			}
		} else {
			logger.Info(logger.StatusInit, "No new relationships discovered")
		}
	case "companies":
		companies := g.GetAllCompanies()
		logger.Plain("")
		logger.Section(fmt.Sprintf("Companies (%d)", len(companies)))
		for _, company := range companies {
			ticker := ""
			if company.Ticker != "" {
				ticker = fmt.Sprintf(" [%s]", company.Ticker)
			}
			logger.Plain("  [%s] %s%s - Health: %.2f", company.ID, company.Name, ticker, company.Health)
		}
	case "relations":
		if len(parts) < 2 {
			logger.Warn(logger.StatusWarn, "Usage: relations <CompanyID>")
			return
		}
		companyID := parts[1]
		relations, err := g.GetCompanyRelations(companyID)
		if err != nil {
			logger.Error(logger.StatusErr, "Error: %v", err)
			return
		}
		printCompanyRelations(relations)
	case "migrate":
		migrateEdges(g, graphFile)
	case "shock":
		if len(parts) < 2 {
			logger.Warn(logger.StatusWarn, "Usage: shock <NodeID> (e.g., shock india)")
			return
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
			return
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
			return
		}
		targetID := parts[1]
		sentiment := 0.0
		fmt.Sscanf(parts[2], "%f", &sentiment)
		if sentiment < -1.0 || sentiment > 1.0 {
			logger.Warn(logger.StatusWarn, "Sentiment must be between -1.0 and 1.0")
			return
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
		logger.Info(logger.StatusInit, "Type 'yes' to confirm or any other key to cancel")
		// Note: Confirmation would need to be handled via another command
		// For now, we'll skip the interactive confirmation in TUI mode
		logger.Warn(logger.StatusWarn, "Reseed cancelled - not supported in TUI mode. Use 'load' command instead.")
	case "social":
		if len(parts) < 2 {
			logger.Warn(logger.StatusWarn, "Usage: social <Topic>")
			return
		}
		topic := strings.Join(parts[1:], " ")
		go socialMon.CrawlReal(topic)
	case "save":
		if len(parts) < 2 {
			logger.Warn(logger.StatusWarn, "Usage: save <filename.json>")
			return
		}
		if err := g.Save(parts[1]); err != nil {
			logger.Error(logger.StatusErr, "Error saving graph: %v", err)
		} else {
			logger.Success("Graph saved to %s", parts[1])
		}
	case "load":
		if len(parts) < 2 {
			logger.Warn(logger.StatusWarn, "Usage: load <filename.json>")
			return
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
			return
		}
		if err := os.WriteFile(parts[1], []byte(g.ToDOT()), 0644); err != nil {
			logger.Error(logger.StatusErr, "Error exporting DOT: %v", err)
		} else {
			logger.Success("Graph exported to %s", parts[1])
		}
	case "exit", "quit", "q":
		logger.Info(logger.StatusOK, "Shutting down...")
		tuiApp.Stop()
	case "help", "?":
		logger.Plain("")
		logger.Section("Available Commands")
		logger.Plain("  show          - Show all nodes and edges")
		logger.Plain("  edges         - Show edge directionality rules")
		logger.Plain("  discover      - Discover and add supplier/client relationships")
		logger.Plain("  companies     - List all companies in the graph")
		logger.Plain("  relations <ID>- Show supplier/client relations for a company")
		logger.Plain("  shock <ID>    - Simulate a trade ban/shock on a Node ID")
		logger.Plain("  boost <ID>    - Simulate positive news boost for a Node ID")
		logger.Plain("  news          - Force check for latest news")
		logger.Plain("  simulate <ID> <sentiment> - Test news impact (sentiment: -1.0 to 1.0)")
		logger.Plain("  social <T>    - Crawl real social media for Topic T")
		logger.Plain("  save <F>      - Save graph to file F")
		logger.Plain("  load <F>      - Load graph from file F")
		logger.Plain("  export <F>    - Export graph to DOT file F")
		logger.Plain("  exit          - Quit")
	default:
		logger.Warn(logger.StatusWarn, "Unknown command: %s (type 'help' for commands)", parts[0])
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
		dir := "→"
		if graph.GetEdgeDirectionality(e.Type) == graph.DirectionalityReverse {
			dir = "←"
		} else if graph.GetEdgeDirectionality(e.Type) == graph.DirectionalityBidirectional {
			dir = "↔"
		}
		logger.Plain("%s %s %s (%.2f) [%s] Status: %s", e.SourceID, dir, e.TargetID, e.Weight, e.Type, e.Status)
	}
}

func printEdgeDirectionality() {
	logger.Plain("")
	logger.Section("Edge Directionality Rules")
	logger.Plain("")
	logger.Plain("How shocks propagate through different edge types:")
	logger.Plain("")

	edgeTypes := []graph.EdgeType{
		graph.EdgeTypeSupplies,
		graph.EdgeTypeProcuresFrom,
		graph.EdgeTypeManufactures,
		graph.EdgeTypeConsumes,
		graph.EdgeTypeProduces,
		graph.EdgeTypeDependsOn,
		graph.EdgeTypeRequires,
		graph.EdgeTypeTrade,
		graph.EdgeTypeCapital,
		graph.EdgeTypeCompetesWith,
		graph.EdgeTypeSubstituteFor,
		graph.EdgeTypeRegulatory,
		graph.EdgeTypeHasIndustry,
		graph.EdgeTypeHasCompany,
	}

	logger.Plain("%-25s %-40s", "Edge Type", "Directionality & Propagation")
	logger.Plain(strings.Repeat("-", 70))

	for _, et := range edgeTypes {
		desc := graph.EdgeDirectionalityDescription(et)
		logger.Plain("%-25s %s", et, desc)
	}

	logger.Plain("")
	logger.Plain("Legend:")
	logger.Plain("  → Unidirectional (shock flows supplier → client)")
	logger.Plain("  ← Reverse (shock flows client → supplier)")
	logger.Plain("  ↔ Bidirectional (shock flows both ways)")
	logger.Plain("")
	logger.Plain("Propagation: Percentage of shock energy that passes through the edge")
	logger.Plain("")
}

func migrateEdges(g *graph.Graph, graphFile string) {
	logger.Plain("")
	logger.Section("Edge Migration")
	logger.Plain("")

	// Check current state
	isValid, missing := g.ValidateEdgeDirectionality()
	if isValid {
		logger.Success("All edges already have directionality set!")
		g.PrintEdgeDirectionalityReport()
		return
	}

	logger.Info(logger.StatusInit, "Found %d edges without directionality", len(missing))
	logger.Plain("")
	logger.Plain("Migrating edges...")

	migrated := g.MigrateEdgeDirectionality()
	logger.Success("Migrated %d edges", migrated)
	logger.Plain("")

	// Show report
	g.PrintEdgeDirectionalityReport()

	// Save
	if err := g.Save(graphFile); err != nil {
		logger.Error(logger.StatusErr, "Failed to save: %v", err)
	} else {
		logger.Success("Saved migrated graph to %s", graphFile)
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

// printCompanyRelations prints detailed company relationships
func printCompanyRelations(relations *graph.CompanyRelations) {
	logger.Plain("")
	logger.Section(fmt.Sprintf("Company: %s [%s]", relations.CompanyName, relations.CompanyID))
	logger.Plain("")

	// Suppliers
	logger.Plain("Suppliers (%d):", len(relations.Suppliers))
	if len(relations.Suppliers) > 0 {
		for _, supplier := range relations.Suppliers {
			ticker := ""
			if supplier.Ticker != "" {
				ticker = fmt.Sprintf(" [%s]", supplier.Ticker)
			}
			logger.Plain("  • %s%s - Health: %.2f", supplier.Name, ticker, supplier.Health)
		}
	} else {
		logger.Plain("  (none)")
	}
	logger.Plain("")

	// Clients
	logger.Plain("Clients (%d):", len(relations.Clients))
	if len(relations.Clients) > 0 {
		for _, client := range relations.Clients {
			ticker := ""
			if client.Ticker != "" {
				ticker = fmt.Sprintf(" [%s]", client.Ticker)
			}
			logger.Plain("  • %s%s - Health: %.2f", client.Name, ticker, client.Health)
		}
	} else {
		logger.Plain("  (none)")
	}
	logger.Plain("")

	// Raw Materials
	logger.Plain("Raw Materials (%d):", len(relations.RawMaterials))
	if len(relations.RawMaterials) > 0 {
		for _, material := range relations.RawMaterials {
			logger.Plain("  • %s (%s) - Health: %.2f", material.Name, material.Type, material.Health)
		}
	} else {
		logger.Plain("  (none)")
	}
	logger.Plain("")

	// Products
	logger.Plain("Products (%d):", len(relations.Products))
	if len(relations.Products) > 0 {
		for _, product := range relations.Products {
			logger.Plain("  • %s - Health: %.2f", product.Name, product.Health)
		}
	} else {
		logger.Plain("  (none)")
	}
}
