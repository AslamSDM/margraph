package discovery

import (
	"encoding/json"
	"fmt"
	"margraf/config"
	"margraf/datasources"
	"margraf/graph"
	"margraf/llm"
	"margraf/logger"
	"margraf/scraper"
	"strings"
	"sync"
)

type Seeder struct {
	Client          *llm.Client
	MarketScraper   *scraper.MarketScraper
	WebSearcher     *scraper.WebSearcher
	ComtradeClient  *datasources.ComtradeClient
	WorldBankClient *datasources.WorldBankClient
	visited         map[string]bool
	mu              sync.Mutex
}

func NewSeeder(client *llm.Client) *Seeder {
	return &Seeder{
		Client:          client,
		MarketScraper:   scraper.NewMarketScraper(),
		WebSearcher:     scraper.NewWebSearcher(),
		ComtradeClient:  datasources.NewComtradeClient(),
		WorldBankClient: datasources.NewWorldBankClient(),
		visited:         make(map[string]bool),
	}
}

func (s *Seeder) Seed(g *graph.Graph) error {
	logger.Info(logger.StatusInit, "Starting Recursive Graph Discovery (Real Data + AI)...")

	if s.Client.ApiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is not set. Cannot fetch live data")
	}

	// 1. Start with major economies via Scraping
	logger.InfoDepth(1, logger.StatusGlob, "[Root] Fetching Top Global Economies from Wikipedia...")
	nations, err := s.MarketScraper.FetchTopNations(10)
	if err != nil {
		logger.WarnDepth(2, logger.StatusWarn, "Scraping failed (%v). Falling back to LLM...", err)
		nations, err = s.fetchList("List the top 10 major global economies covering all continents. Return ONLY a JSON array of strings.")
		if err != nil {
			return fmt.Errorf("failed to fetch nations: %v", err)
		}
	} else {
		logger.SuccessDepth(2, "Scraped %d nations successfully", len(nations))
	}

	for _, name := range nations {
		// We start recursion at depth 0
		if err := s.ProcessNation(g, name, 0); err != nil {
			fmt.Printf("Error processing nation %s: %v\n", name, err)
		}
	}

	// 3. Discover Relationships (Cross-Nation Trade) - Simplified for now, usually part of deeper logic
	// We can try to find major trade partners for the top nations found.
	// For this prototype, we will do a targeted discovery for the first few nations to link them.
	if len(nations) > 1 {
		s.discoverTradeLinks(g, nations)
	}

	return nil
}

func (s *Seeder) discoverTradeLinks(g *graph.Graph, nations []string) {
	logger.Info(logger.StatusLink, "Discovering Major Trade Relationships (UN Comtrade + World Bank)...")

	// Limit to first 5 to avoid N^2 explosion and API rate limits
	limit := 5
	if len(nations) < limit {
		limit = len(nations)
	}

	targetNations := nations[:limit]
	year := "2023" // Most recent complete year

	// Strategy 1: Use UN Comtrade for REAL bilateral trade data
	for _, nation1 := range targetNations {
		// Get country code
		code1, ok := datasources.GetCountryCode(strings.ToLower(nation1))
		if !ok {
			logger.WarnDepth(1, logger.StatusWarn, "No ISO code for %s, skipping Comtrade lookup", nation1)
			continue
		}

		// Get economic profile from World Bank
		logger.InfoDepth(1, logger.StatusData, "Fetching economic data for %s from World Bank...", nation1)
		profile, err := s.WorldBankClient.GetEconomicProfile(code1, year)
		if err == nil && profile.GDP > 0 {
			// Store economic data in node attributes
			if node, ok := g.GetNode(cleanID(nation1)); ok {
				node.Attributes["gdp"] = profile.GDP
				node.Attributes["exports"] = profile.Exports
				node.Attributes["imports"] = profile.Imports
				node.Attributes["fdi"] = profile.FDI
				logger.SuccessDepth(2, "GDP: $%.2fB, Exports: $%.2fB", profile.GDP/1e9, profile.Exports/1e9)
			}
		}

		// Get top exports from Comtrade
		logger.InfoDepth(1, logger.StatusGlob, "Fetching trade data for %s from UN Comtrade...", nation1)
		topExports, err := s.ComtradeClient.GetTopExports(code1, year, 5)
		if err != nil {
			logger.WarnDepth(2, logger.StatusWarn, "Comtrade error: %v", err)
			continue
		}

		// Create edges based on real trade data
		for _, trade := range topExports {
			if trade.PrimaryValue < 1e9 { // Skip trades < $1B
				continue
			}

			// Add commodity node if it doesn't exist
			commodityID := cleanID(trade.CommodityDesc)
			if _, exists := g.GetNode(commodityID); !exists {
				g.AddNode(&graph.Node{
					ID:   commodityID,
					Type: graph.NodeTypeRawMaterial,
					Name: trade.CommodityDesc,
					Attributes: map[string]interface{}{
						"hs_code": trade.CommodityCode,
					},
				})
			}

			// Create PRODUCES edge with real trade value as weight
			// Normalize weight: $1B = 0.1, $10B = 0.5, $100B = 1.0 (log scale)
			weight := 0.1 + (0.4 * (trade.PrimaryValue / 1e11))
			if weight > 1.0 {
				weight = 1.0
			}

			g.AddEdge(&graph.Edge{
				SourceID: cleanID(nation1),
				TargetID: commodityID,
				Type:     graph.EdgeTypeProduces,
				Weight:   weight,
			})

			logger.SuccessDepth(2, "%s exports %s ($%.2fB, weight=%.2f)",
				nation1, trade.CommodityDesc, trade.PrimaryValue/1e9, weight)
		}

		// Check bilateral trade with other nations in the list
		for _, nation2 := range targetNations {
			if nation1 == nation2 {
				continue
			}

			code2, ok := datasources.GetCountryCode(strings.ToLower(nation2))
			if !ok {
				continue
			}

			// Get bilateral trade
			bilateralTrade, err := s.ComtradeClient.GetBilateralTrade(code1, code2, year)
			if err != nil {
				continue
			}

			// Sum up total bilateral trade value
			totalValue := 0.0
			for _, trade := range bilateralTrade {
				totalValue += trade.PrimaryValue
			}

			if totalValue > 5e9 { // Only create edges for significant trade (>$5B)
				srcID := cleanID(nation1)
				tgtID := cleanID(nation2)

				if _, ok := g.GetNode(srcID); !ok {
					continue
				}
				if _, ok := g.GetNode(tgtID); !ok {
					continue
				}

				// Normalize weight
				weight := 0.3 + (0.5 * (totalValue / 1e11))
				if weight > 1.0 {
					weight = 1.0
				}

				g.AddEdge(&graph.Edge{
					SourceID: srcID,
					TargetID: tgtID,
					Type:     graph.EdgeTypeTrade,
					Weight:   weight,
				})

				logger.SuccessDepth(1, "%s -> %s: $%.2fB trade (weight=%.2f)", nation1, nation2, totalValue/1e9, weight)
			}
		}
	}

	logger.SuccessDepth(1, "Trade discovery complete with real UN Comtrade + World Bank data")
}

func (s *Seeder) validateRelationship(source, target, product string) (bool, error) {
	logger.InfoDepth(2, logger.StatusChk, "Validating: %s exports %s to %s", source, product, target)
	query := fmt.Sprintf("Does %s export %s to %s", source, product, target)

	results, err := s.WebSearcher.Search(query)
	if err != nil {
		// Silently trust if search fails - no need to warn
		return true, nil
	}

	if len(results) == 0 {
		return false, nil
	}

	// Check for keywords in snippets
	keywords := []string{"export", "trade", "sell", "supply", "deal", "import", "relation", "partner", "billion", "million"}
	hits := 0

	sourceLower := strings.ToLower(source)
	targetLower := strings.ToLower(target)

	for _, res := range results {
		text := strings.ToLower(res.Title + " " + res.Snippet)

		// Must mention both entities (roughly)
		if !strings.Contains(text, sourceLower) && !strings.Contains(text, targetLower) {
			continue
		}

		for _, kw := range keywords {
			if strings.Contains(text, kw) {
				hits++
				break
			}
		}
	}

	if hits > 0 {
		return true, nil
	}
	return false, nil
}

// ProcessNation adds a nation, finds its industries
func (s *Seeder) ProcessNation(g *graph.Graph, name string, depth int) error {
	id := cleanID(name)

	if s.isVisited(id) {
		return nil
	}
	s.markVisited(id)

	// 1. Add Nation Node
	if valid, _ := s.validateEntity(name, "Nation"); valid {
		g.AddNode(&graph.Node{ID: id, Type: graph.NodeTypeNation, Name: name})
		logger.InfoDepth(depth, logger.StatusNat, "Added Nation: %s", name)
	} else {
		return nil // Skip if invalid
	}

	// 2. Find Industries (Expanded sectors)
	prompt := fmt.Sprintf("List the top %d major industries driving the economy of %s. Ensure to cover diverse sectors like Agriculture, Manufacturing, Tech, Finance, and Energy. Return ONLY a JSON array of strings.", config.Global.Scraping.BranchingLimit, name)
	industries, err := s.fetchList(prompt)
	if err != nil {
		return err
	}

	for _, ind := range industries {
		if err := s.processIndustry(g, ind, name, depth); err != nil {
			fmt.Printf("    Error processing industry %s: %v\n", ind, err)
		}
	}

	return nil
}

// processIndustry adds industry, links to nation, finds companies and raw materials
func (s *Seeder) processIndustry(g *graph.Graph, industryName, nationName string, depth int) error {
	indID := cleanID(nationName + "_" + industryName)
	nationID := cleanID(nationName)

	// Add Industry Node
	g.AddNode(&graph.Node{ID: indID, Type: graph.NodeTypeIndustry, Name: industryName})
	g.AddEdge(&graph.Edge{SourceID: nationID, TargetID: indID, Type: graph.EdgeTypeHasIndustry, Weight: 1.0})
	logger.InfoDepth(2, logger.StatusInd, "Added Industry: %s (in %s)", industryName, nationName)

	// 1. Find Major Companies (RAG: Search + LLM Extraction)
	logger.InfoDepth(3, logger.StatusChk, "Finding companies in '%s' (%s)...", industryName, nationName)
	searchQuery := fmt.Sprintf("Largest %s companies in %s market cap", industryName, nationName)
	searchResults, err := s.WebSearcher.Search(searchQuery)

	var companies []string
	searchSucceeded := false

	if err == nil && len(searchResults) > 0 {
		// Construct context from search results
		var contextBuilder strings.Builder
		for _, res := range searchResults {
			contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", res.Title, res.Snippet))
		}

		// RAG Prompt
		ragPrompt := fmt.Sprintf(`
Extract the names of the top %d %s companies in %s from the following search results.
Search Results:
%s
Return ONLY a JSON array of strings, e.g. ["Company A", "Company B"].
`, config.Global.Scraping.BranchingLimit, industryName, nationName, contextBuilder.String())

		companies, _ = s.fetchList(ragPrompt)
		if len(companies) > 0 {
			searchSucceeded = true
			logger.InfoDepth(3, logger.StatusOK, "Found %d companies via web search", len(companies))
		}
	}

	// Fallback if search/extraction failed or returned empty
	if len(companies) == 0 {
		// Only log warning if search truly failed, not just for LLM fallback
		if err != nil && !searchSucceeded {
			logger.InfoDepth(3, logger.StatusChk, "Using LLM knowledge base for companies...")
		}
		cPrompt := fmt.Sprintf("List %d largest companies by market cap in the %s industry in %s. Return ONLY a JSON array of strings.", config.Global.Scraping.BranchingLimit, industryName, nationName)
		companies, _ = s.fetchList(cPrompt)
	}

	for _, comp := range companies {
		compID := cleanID(comp)
		g.AddNode(&graph.Node{ID: compID, Type: graph.NodeTypeCorporation, Name: comp})
		g.AddEdge(&graph.Edge{SourceID: indID, TargetID: compID, Type: graph.EdgeTypeHasCompany, Weight: 1.0})
		logger.InfoDepth(3, logger.StatusCor, "Added Company: %s", comp)

		// Discover supplier/client relationships for this company
		go s.discoverCompanyRelations(g, comp, compID, industryName, depth)
	}

	// 2. Find Raw Materials
	mPrompt := fmt.Sprintf("List %d key raw materials or commodities required for the %s industry. Return ONLY a JSON array of strings.", config.Global.Scraping.BranchingLimit, industryName)
	materials, _ := s.fetchList(mPrompt)
	for _, mat := range materials {
		if err := s.processMaterial(g, mat, indID, depth); err != nil {
			fmt.Printf("      Error processing material %s: %v\n", mat, err)
		}
	}

	return nil
}

// processMaterial adds material, links to industry, finds top producers (recursion)
func (s *Seeder) processMaterial(g *graph.Graph, matName, industryNodeID string, depth int) error {
	matID := cleanID(matName)

	// Add Material Node (idempotent check done by AddNode usually, but we might want to ensure it exists)
	if _, exists := g.GetNode(matID); !exists {
		g.AddNode(&graph.Node{ID: matID, Type: graph.NodeTypeRawMaterial, Name: matName})
		logger.InfoDepth(3, logger.StatusMat, "Added Material: %s", matName)
	}

	// Link Industry -> Requires -> Material
	g.AddEdge(&graph.Edge{SourceID: industryNodeID, TargetID: matID, Type: graph.EdgeTypeRequires, Weight: 1.0})

	// RECURSION CHECK
	if depth >= config.Global.Scraping.SearchDepth {
		return nil
	}

	// Find Producer Nations
	pPrompt := fmt.Sprintf("List top %d countries that produce %s. Return ONLY a JSON array of strings.", config.Global.Scraping.BranchingLimit, matName)
	producers, _ := s.fetchList(pPrompt)

	for _, producerName := range producers {
		prodID := cleanID(producerName)

		// Recursively process this nation
		// We rely on s.visited to stop infinite loops if we've already seen this nation
		if !s.isVisited(prodID) {
			logger.InfoDepth(4, logger.StatusRec, "Discovered Producer: %s (Recursing...)", producerName)
			if err := s.ProcessNation(g, producerName, depth+1); err != nil {
				fmt.Printf("Error recursing nation %s: %v\n", producerName, err)
			}
		}

		// Link Producer -> Produces -> Material
		// (Even if nation was already visited, we establish the link)
		if _, ok := g.GetNode(prodID); ok {
			g.AddEdge(&graph.Edge{SourceID: prodID, TargetID: matID, Type: graph.EdgeTypeProduces, Weight: 1.0})
			logger.InfoDepth(4, logger.StatusLink, "Link: %s -> Produces -> %s", producerName, matName)
		}
	}

	return nil
}

// Helpers

func (s *Seeder) isVisited(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.visited[id]
}

func (s *Seeder) markVisited(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visited[id] = true
}

func (s *Seeder) fetchList(prompt string) ([]string, error) {
	resp, err := s.Client.Complete(prompt)
	if err != nil {
		return nil, err
	}

	cleaned := cleanJSON(resp)
	var list []string
	if err := json.Unmarshal([]byte(cleaned), &list); err != nil {
		// Try to parse simplified list if JSON fails or if LLM returned bullets
		return nil, fmt.Errorf("json parse error: %v | raw: %s", err, resp)
	}
	return list, nil
}

type edgeDTO struct {
	Source  string  `json:"source"`
	Target  string  `json:"target"`
	Product string  `json:"product"`
	Weight  float64 `json:"weight"`
}

func (s *Seeder) fetchEdges(prompt string) ([]edgeDTO, error) {
	resp, err := s.Client.Complete(prompt)
	if err != nil {
		return nil, err
	}

	cleaned := cleanJSON(resp)
	var list []edgeDTO
	if err := json.Unmarshal([]byte(cleaned), &list); err != nil {
		return nil, fmt.Errorf("json parse error: %v | raw: %s", err, resp)
	}
	return list, nil
}

func (s *Seeder) validateEntity(name, category string) (bool, error) {
	// Real Web Validation
	logger.InfoDepth(2, logger.StatusChk, "Validating '%s'...", name)

	query := fmt.Sprintf("%s %s wikipedia", name, category)
	results, err := s.WebSearcher.Search(query)
	if err != nil {
		// Silently assume valid if search fails
		return true, nil
	}

	if len(results) == 0 {
		logger.WarnDepth(3, logger.StatusWarn, "No search results found for %s", name)
		return false, nil
	}

	// Basic check: does the top result title contain the name?
	// Or do we see keywords?
	hitCount := 0
	nameLower := strings.ToLower(name)

	for _, res := range results {
		titleLower := strings.ToLower(res.Title)
		snippetLower := strings.ToLower(res.Snippet)

		if strings.Contains(titleLower, nameLower) || strings.Contains(snippetLower, nameLower) {
			hitCount++
		}
	}

	if hitCount > 0 {
		return true, nil
	}

	logger.WarnDepth(3, logger.StatusErr, "Validation failed. Web results didn't match '%s'", name)
	return false, nil
}

// Reuse existing clean helpers
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

// extractCompaniesFromSearchResults extracts company names from search results
func (s *Seeder) extractCompaniesFromSearchResults(results []scraper.SearchResult, excludeCompany, relationType string) []string {
	companies := make([]string, 0)
	companySet := make(map[string]bool)

	// Common patterns that indicate company names
	// Look for: "Company Name Inc", "Company Corp", "Company Ltd", etc.
	companyIndicators := []string{
		"Inc", "Corp", "Corporation", "Ltd", "Limited", "LLC", "Co.",
		"Group", "Holdings", "International", "Technologies", "Systems",
	}

	for _, result := range results {
		text := result.Title + " " + result.Snippet
		words := strings.Fields(text)

		// Simple heuristic: look for capitalized sequences that might be company names
		for i := 0; i < len(words); i++ {
			// Skip if word is too short or is the excluded company
			if len(words[i]) < 3 {
				continue
			}

			// Check if this looks like start of a company name (capitalized)
			if words[i][0] >= 'A' && words[i][0] <= 'Z' {
				possibleName := words[i]

				// Look ahead for multi-word company names (up to 3 words)
				for j := i + 1; j < len(words) && j < i+3; j++ {
					nextWord := words[j]

					// Stop if we hit common non-company words
					if strings.ToLower(nextWord) == "the" || strings.ToLower(nextWord) == "a" ||
						strings.ToLower(nextWord) == "and" || strings.ToLower(nextWord) == "of" {
						break
					}

					// If next word is capitalized or a company indicator, add it
					if (nextWord[0] >= 'A' && nextWord[0] <= 'Z') || containsAny(nextWord, companyIndicators) {
						possibleName += " " + nextWord

						// If we found a company indicator, this is likely a complete company name
						if containsAny(nextWord, companyIndicators) {
							break
						}
					} else {
						break
					}
				}

				// Clean and validate the company name
				possibleName = strings.TrimSpace(possibleName)
				possibleName = strings.Trim(possibleName, ".,;:()[]{}\"'")

				// Only add if it's not too short, not the excluded company, and not already added
				if len(possibleName) >= 3 &&
					!strings.Contains(strings.ToLower(possibleName), strings.ToLower(excludeCompany)) &&
					!companySet[possibleName] {

					companySet[possibleName] = true
					companies = append(companies, possibleName)
				}
			}
		}
	}

	return companies
}

// containsAny checks if string contains any of the substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// contains checks if a string slice contains a string (case-insensitive)
func contains(slice []string, item string) bool {
	itemLower := strings.ToLower(item)
	for _, s := range slice {
		if strings.ToLower(s) == itemLower {
			return true
		}
	}
	return false
}

// discoverCompanyRelations discovers and adds supplier/client relationships for a company
func (s *Seeder) discoverCompanyRelations(g *graph.Graph, companyName, companyID, industryName string, depth int) {
	// Don't go too deep to avoid infinite recursion
	if depth > config.Global.Scraping.SearchDepth {
		return
	}

	logger.InfoDepth(4, logger.StatusChk, "Discovering relations for %s...", companyName)

	var relations struct {
		Suppliers []string `json:"suppliers"`
		Clients   []string `json:"clients"`
	}

	// Strategy 1: Web search for supplier relationships
	suppliersQuery := fmt.Sprintf("%s suppliers major partners procurement", companyName)
	suppliersResults, err := s.WebSearcher.Search(suppliersQuery)

	if err == nil && len(suppliersResults) > 0 {
		// Extract company names from search results
		suppliers := s.extractCompaniesFromSearchResults(suppliersResults, companyName, "supplier")
		relations.Suppliers = append(relations.Suppliers, suppliers...)

		if len(suppliers) > 0 {
			logger.InfoDepth(4, logger.StatusOK, "Found %d suppliers via web search", len(suppliers))
		}
	}

	// Strategy 2: Web search for client/customer relationships
	clientsQuery := fmt.Sprintf("%s customers clients major contracts partnerships", companyName)
	clientsResults, err := s.WebSearcher.Search(clientsQuery)

	if err == nil && len(clientsResults) > 0 {
		// Extract company names from search results
		clients := s.extractCompaniesFromSearchResults(clientsResults, companyName, "client")
		relations.Clients = append(relations.Clients, clients...)

		if len(clients) > 0 {
			logger.InfoDepth(4, logger.StatusOK, "Found %d clients via web search", len(clients))
		}
	}

	// Strategy 3: Use LLM with search context as RAG to supplement findings
	logger.InfoDepth(4, logger.StatusChk, "Analyzing with LLM for additional relations...")

	// Build context from all search results
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Web search findings:\n")

	if len(suppliersResults) > 0 {
		contextBuilder.WriteString("\nSupplier-related information:\n")
		for _, res := range suppliersResults {
			contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", res.Title, res.Snippet))
		}
	}

	if len(clientsResults) > 0 {
		contextBuilder.WriteString("\nClient-related information:\n")
		for _, res := range clientsResults {
			contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", res.Title, res.Snippet))
		}
	}

	// RAG prompt with web search context - no limits
	prompt := fmt.Sprintf(`
Based on the following web search results about "%s" in the %s industry, extract ALL company relationships you can find.

%s

Extract ALL suppliers and clients mentioned. Focus on extracting actual company names mentioned in the search results.

Return ONLY a JSON object in this format:
{
  "suppliers": ["Company Name 1", "Company Name 2", ...],
  "clients": ["Company Name 1", "Company Name 2", ...]
}

Include all companies explicitly mentioned in the search results. Return empty arrays if no clear relationships are found.
`, companyName, industryName, contextBuilder.String())

	resp, err := s.Client.Complete(prompt)
	if err == nil {
		cleaned := cleanJSON(resp)

		var llmRelations struct {
			Suppliers []string `json:"suppliers"`
			Clients   []string `json:"clients"`
		}

		if err := json.Unmarshal([]byte(cleaned), &llmRelations); err == nil {
			// Add LLM-found relations that we don't already have
			for _, supplier := range llmRelations.Suppliers {
				if supplier != "" && !contains(relations.Suppliers, supplier) {
					relations.Suppliers = append(relations.Suppliers, supplier)
				}
			}
			for _, client := range llmRelations.Clients {
				if client != "" && !contains(relations.Clients, client) {
					relations.Clients = append(relations.Clients, client)
				}
			}
		}
	}

	// Add suppliers
	for _, supplier := range relations.Suppliers {
		if supplier == "" {
			continue
		}

		supplierID := cleanID(supplier)

		// Add supplier node if it doesn't exist
		if _, exists := g.GetNode(supplierID); !exists {
			g.AddNode(&graph.Node{
				ID:   supplierID,
				Type: graph.NodeTypeCorporation,
				Name: supplier,
			})
			logger.InfoDepth(4, logger.StatusNew, "Added supplier: %s", supplier)
		}

		// Add Supplies edge (supplier -> company)
		g.AddEdge(&graph.Edge{
			SourceID:       supplierID,
			TargetID:       companyID,
			Type:           graph.EdgeTypeSupplies,
			Weight:         0.7,
			Status:         "Active",
			Directionality: graph.DirectionalityUnidirectional,
		})

		// Add ProcuresFrom edge (company -> supplier)
		g.AddEdge(&graph.Edge{
			SourceID:       companyID,
			TargetID:       supplierID,
			Type:           graph.EdgeTypeProcuresFrom,
			Weight:         0.7,
			Status:         "Active",
			Directionality: graph.DirectionalityReverse,
		})

		logger.SuccessDepth(4, "%s ← supplies ← %s", companyName, supplier)
	}

	// Add clients
	for _, client := range relations.Clients {
		if client == "" {
			continue
		}

		clientID := cleanID(client)

		// Add client node if it doesn't exist
		if _, exists := g.GetNode(clientID); !exists {
			g.AddNode(&graph.Node{
				ID:   clientID,
				Type: graph.NodeTypeCorporation,
				Name: client,
			})
			logger.InfoDepth(4, logger.StatusNew, "Added client: %s", client)
		}

		// Add Supplies edge (company -> client)
		g.AddEdge(&graph.Edge{
			SourceID:       companyID,
			TargetID:       clientID,
			Type:           graph.EdgeTypeSupplies,
			Weight:         0.7,
			Status:         "Active",
			Directionality: graph.DirectionalityUnidirectional,
		})

		// Add ProcuresFrom edge (client -> company)
		g.AddEdge(&graph.Edge{
			SourceID:       clientID,
			TargetID:       companyID,
			Type:           graph.EdgeTypeProcuresFrom,
			Weight:         0.7,
			Status:         "Active",
			Directionality: graph.DirectionalityReverse,
		})

		logger.SuccessDepth(4, "%s → supplies → %s", companyName, client)
	}

	relationCount := len(relations.Suppliers) + len(relations.Clients)
	if relationCount > 0 {
		logger.SuccessDepth(4, "Discovered %d relations for %s", relationCount, companyName)
	}
}
