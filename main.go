package main

import (
	"bufio"
	"fmt"
	"margraf/config"
	"margraf/discovery"
	"margraf/graph"
	"margraf/llm"
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
		// return // Or continue with defaults? For now, exit is safer to notice error
		// Actually, let's just print and proceed if we want robustness, but better to fail fast.
		os.Exit(1)
	}

	fmt.Println("==================================================")
	fmt.Printf("   %s v%s\n", config.Global.App.Name, config.Global.App.Version)
	fmt.Println("==================================================")

	// 1. Setup
	g := graph.NewGraph()
	g.EnableAutoSave("margraf_graph.json", 10) // Auto-save every 10 changes
	client := llm.NewClient()
	seeder := discovery.NewSeeder(client)
	
	// 1b. Setup Websocket Server & Social Monitor
	hub := server.NewHub()
	go hub.Run()
	server.StartServer(hub, config.Global.Server.Port)

	socialMonitor := social.NewMonitor(client, hub, g)
	marketMonitor := simulation.NewMarketMonitor(g, hub)

	// 2. Discovery Phase
	fmt.Println("\n[1] Initializing Graph via LLM/API...")
	if err := seeder.Seed(g); err != nil {
		fmt.Printf("Error seeding graph: %v\n", err)
	}
	fmt.Printf("\nGraph Ready: %s\n", g.String())

	// 3. Interactive or Demo Mode
	reader := bufio.NewReader(os.Stdin)
	sim := simulation.NewSimulator(g)
	
	// 4. Start Engines
	newsEngine := news.NewEngine(g, client, seeder, sim, hub, socialMonitor)
	
	newsInterval := time.Duration(config.Global.News.PollInterval) * time.Second
	marketInterval := time.Duration(config.Global.Market.PollInterval) * time.Second
	
	go newsEngine.Monitor(newsInterval)
	go marketMonitor.Start(marketInterval)
	
	// Broadcast Graph Pulse (Keep UI in sync)
	go func() {
		for range time.Tick(5 * time.Second) {
			hub.Broadcast("graph_dot", g.ToDOT())
		}
	}()
	
	// AutoSave (Every 5 mins)
	go func() {
		for range time.Tick(5 * time.Minute) {
			if err := g.Save("margraf_autosave.json"); err != nil {
				fmt.Printf("AutoSave Failed: %v\n", err)
			} else {
				// Quietly save
			}
		}
	}()

	for {
		fmt.Println("\n--------------------------------------------------")
		fmt.Println("Available Commands:")
		fmt.Println("  show       - Show all nodes and edges")
		fmt.Println("  shock <ID> - Simulate a trade ban/shock on a Node ID")
		fmt.Println("  news       - Force check for latest news")
		fmt.Println("  social <T> - Simulate social pulse for Topic T")
		fmt.Println("  save <F>   - Save graph to file F")
		fmt.Println("  load <F>   - Load graph from file F")
		fmt.Println("  export <F> - Export graph to DOT file F")
		fmt.Println("  exit       - Quit")
		fmt.Print("> ")

		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		parts := strings.Split(input, " ")

		switch parts[0] {
		case "show":
			printGraph(g)
		case "shock":
			if len(parts) < 2 {
				fmt.Println("Usage: shock <NodeID> (e.g., shock india)")
				continue
			}
			targetID := parts[1]
			sim.RunShock(simulation.ShockEvent{
				TargetNodeID: targetID,
				Description:  "Trade Ban / Supply Chain Failure",
				ImpactFactor: 0.1, // 90% reduction
			})
		case "news":
			newsEngine.FetchAndProcess()
		case "social":
			if len(parts) < 2 {
				fmt.Println("Usage: social <Topic>")
				continue
			}
			topic := strings.Join(parts[1:], " ")
			go socialMonitor.CrawlReal(topic)
		case "save":
			if len(parts) < 2 {
				fmt.Println("Usage: save <filename.json>")
				continue
			}
			if err := g.Save(parts[1]); err != nil {
				fmt.Printf("Error saving graph: %v\n", err)
			} else {
				fmt.Printf("Graph saved to %s\n", parts[1])
			}
		case "load":
			if len(parts) < 2 {
				fmt.Println("Usage: load <filename.json>")
				continue
			}
			newG, err := graph.Load(parts[1])
			if err != nil {
				fmt.Printf("Error loading graph: %v\n", err)
			} else {
				g.Replace(newG)
				fmt.Printf("Graph loaded from %s (%s)\n", parts[1], g.String())
			}
		case "export":
			if len(parts) < 2 {
				fmt.Println("Usage: export <filename.dot>")
				continue
			}
			if err := os.WriteFile(parts[1], []byte(g.ToDOT()), 0644); err != nil {
				fmt.Printf("Error exporting DOT: %v\n", err)
			} else {
				fmt.Printf("Graph exported to %s\n", parts[1])
			}
		case "exit":
			fmt.Println("Goodbye.")
			return
		default:
			fmt.Println("Unknown command.")
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
	fmt.Println("\n--- Nodes ---")
	for _, n := range g.Nodes {
		fmt.Printf("[%s] %s (%s)\n", n.ID, n.Name, n.Type)
	}
	fmt.Println("\n--- Edges ---")
	for _, e := range g.Edges {
		fmt.Printf("%s --(%.2f)--> %s [%s]\n", e.SourceID, e.Weight, e.TargetID, e.Type)
	}
}
