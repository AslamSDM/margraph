package graph

import (
	"encoding/json"
	"fmt"
	"margraf/logger"
	"os"
	"sync"
	"time"
)

// NodeType represents the category of a node.
type NodeType string

const (
	NodeTypeNation      NodeType = "Nation"
	NodeTypeCorporation NodeType = "Corporation"
	NodeTypeProduct     NodeType = "Product" // Generic product
	NodeTypeIndustry    NodeType = "Industry"
	NodeTypeRawMaterial NodeType = "RawMaterial"
	NodeTypeCrop        NodeType = "Crop"
)

// EdgeType represents the nature of the relationship.
type EdgeType string

const (
	EdgeTypeTrade         EdgeType = "Trade"
	EdgeTypeCapital       EdgeType = "Capital"
	EdgeTypeRegulatory    EdgeType = "Regulatory"
	EdgeTypeHasIndustry   EdgeType = "HasIndustry"   // Nation -> Industry
	EdgeTypeHasCompany    EdgeType = "HasCompany"    // Industry -> Company
	EdgeTypeRequires      EdgeType = "Requires"      // Industry/Company -> RawMaterial
	EdgeTypeProduces      EdgeType = "Produces"      // Nation -> RawMaterial
	EdgeTypeSubstituteFor EdgeType = "SubstituteFor" // Commodity -> Commodity (for finding winners)
	EdgeTypeCompetesWith  EdgeType = "CompetesWith"  // Company -> Company
	EdgeTypeDependsOn     EdgeType = "DependsOn"     // Company -> Supplier

	// Supply Chain Directional Edges (for shock propagation)
	EdgeTypeSupplies      EdgeType = "Supplies"      // Supplier -> Client (shocks flow downstream)
	EdgeTypeProcuresFrom  EdgeType = "ProcuresFrom"  // Client -> Supplier (for reference, shocks flow reverse)
	EdgeTypeManufactures  EdgeType = "Manufactures"  // Company -> Product
	EdgeTypeConsumes      EdgeType = "Consumes"      // Company -> RawMaterial
)

// EdgeDirectionality defines how shocks propagate through edge types
type EdgeDirectionality string

const (
	// Unidirectional: shocks only flow from source to target
	DirectionalityUnidirectional EdgeDirectionality = "Unidirectional"

	// Bidirectional: shocks can flow both ways
	DirectionalityBidirectional EdgeDirectionality = "Bidirectional"

	// Reverse: shocks flow from target back to source
	DirectionalityReverse EdgeDirectionality = "Reverse"
)

// Node represents an entity in the economic ecosystem.
type Node struct {
	ID         string                 `json:"id"`
	Type       NodeType               `json:"type"`
	Name       string                 `json:"name"`
	Health     float64                `json:"health"` // 1.0 = Normal, <1.0 = Stressed, >1.0 = Booming
	Ticker     string                 `json:"ticker,omitempty"`
	Price      float64                `json:"price,omitempty"`
	Currency   string                 `json:"currency,omitempty"`
	LastUpdated time.Time             `json:"last_updated,omitempty"`
	Attributes map[string]interface{} `json:"attributes"`
}

// Edge represents a connection between two nodes.
type Edge struct {
	SourceID      string              `json:"source_id"`
	TargetID      string              `json:"target_id"`
	Type          EdgeType            `json:"type"`
	Weight        float64             `json:"weight"`          // Represents strength, volume, or influence (0.0 to 1.0 or scalar)
	Timestamp     time.Time           `json:"timestamp"`       // Temporal Knowledge Graph: Track when edge was created/updated
	Status        string              `json:"status"`          // Active, Blocked, Suspended, etc.
	Directionality EdgeDirectionality `json:"directionality"` // How shocks propagate through this edge
}

// EdgeHistory tracks the temporal evolution of a relationship
type EdgeHistory struct {
	SourceID string      `json:"source_id"`
	TargetID string      `json:"target_id"`
	Type     EdgeType    `json:"type"`
	History  []EdgeSnapshot `json:"history"`
}

// EdgeSnapshot represents a point-in-time state of an edge
type EdgeSnapshot struct {
	Weight    float64   `json:"weight"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
	EventID   string    `json:"event_id,omitempty"` // Reference to news event that caused change
}

// Graph represents the FDKG (Financial Dynamic Knowledge Graph).
type Graph struct {
	Nodes        map[string]*Node          `json:"nodes"`
	Edges        []*Edge                   `json:"edges"`
	EdgeHistories map[string]*EdgeHistory   `json:"edge_histories"` // Key: "srcID|tgtID|type"
	Adjacency    map[string][]*Edge        `json:"-"` // Cache for O(1) lookup, ignored in JSON
	mu           sync.RWMutex

	// Auto-save configuration
	autoSavePath    string
	changesSinceLastSave int
	autoSaveThreshold    int // Save after N changes
}

// NewGraph initializes a new empty graph.
func NewGraph() *Graph {
	return &Graph{
		Nodes:             make(map[string]*Node),
		Edges:             make([]*Edge, 0),
		EdgeHistories:     make(map[string]*EdgeHistory),
		Adjacency:         make(map[string][]*Edge),
		autoSavePath:      "margraf_graph.json",
		autoSaveThreshold: 10, // Save every 10 changes
	}
}

// EnableAutoSave configures automatic graph persistence
func (g *Graph) EnableAutoSave(path string, threshold int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.autoSavePath = path
	g.autoSaveThreshold = threshold
	logger.Info(logger.StatusSave, "Auto-save enabled: %s (every %d changes)", path, threshold)
}

// triggerAutoSave saves the graph if threshold is reached (must be called with lock held)
func (g *Graph) triggerAutoSave() {
	g.changesSinceLastSave++

	if g.changesSinceLastSave >= g.autoSaveThreshold {
		// Release lock temporarily for save operation
		g.mu.Unlock()

		if err := g.Save(g.autoSavePath); err != nil {
			logger.Warn(logger.StatusWarn, "Auto-save failed: %v", err)
		} else {
			logger.Info(logger.StatusSave, "Auto-saved graph to %s (%d nodes, %d edges)", g.autoSavePath, len(g.Nodes), len(g.Edges))
		}

		g.mu.Lock()
		g.changesSinceLastSave = 0
	}
}

// AddNode adds a node to the graph safely.
func (g *Graph) AddNode(n *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n.Health == 0 {
		n.Health = 1.0 // Default health
	}
	g.Nodes[n.ID] = n

	// Trigger auto-save if enabled
	g.triggerAutoSave()
}

// Clear removes all nodes and edges from the graph safely.
func (g *Graph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.Nodes = make(map[string]*Node)
	g.Edges = make([]*Edge, 0)
	g.EdgeHistories = make(map[string]*EdgeHistory)
	g.Adjacency = make(map[string][]*Edge)
	g.changesSinceLastSave = 0
	
	logger.Info(logger.StatusInit, "Graph cleared")
}

// UpdateNodeHealth safely updates a node's health score.
func (g *Graph) UpdateNodeHealth(id string, delta float64) (float64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	node, ok := g.Nodes[id]
	if !ok {
		return 0, false
	}

	// Apply delta (e.g. -0.1 or +0.05)
	node.Health += delta

	// Clamp health reasonable bounds (e.g., 0.1 to 2.0)
	if node.Health < 0.1 { node.Health = 0.1 }
	if node.Health > 2.0 { node.Health = 2.0 }

	return node.Health, true
}

// UpdateNodePrice safely updates a node's price and currency.
func (g *Graph) UpdateNodePrice(id string, price float64, currency string, ticker string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	node, ok := g.Nodes[id]
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}

	node.Price = price
	node.Currency = currency
	if ticker != "" {
		node.Ticker = ticker
	}
	node.LastUpdated = time.Now()

	return nil
}

// GetNodeTicker safely retrieves a node's ticker.
func (g *Graph) GetNodeTicker(id string) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, ok := g.Nodes[id]
	if !ok {
		return "", false
	}

	return node.Ticker, true
}

// SetNodeTicker safely sets a node's ticker.
func (g *Graph) SetNodeTicker(id string, ticker string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	node, ok := g.Nodes[id]
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}

	node.Ticker = ticker
	return nil
}

// AddEdge adds an edge to the graph safely and records its history.
func (g *Graph) AddEdge(e *Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Set timestamp if not already set
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	// Set default status
	if e.Status == "" {
		e.Status = "Active"
	}

	// Set directionality based on edge type
	if e.Directionality == "" {
		e.Directionality = GetEdgeDirectionality(e.Type)
	}

	g.Edges = append(g.Edges, e)

	// Update Adjacency Map
	if g.Adjacency == nil {
		g.Adjacency = make(map[string][]*Edge)
	}
	g.Adjacency[e.SourceID] = append(g.Adjacency[e.SourceID], e)

	// Record in temporal history
	g.recordEdgeHistory(e, "")

	// Trigger auto-save if enabled
	g.triggerAutoSave()
}

// recordEdgeHistory stores a snapshot of the edge state (must be called with lock held)
func (g *Graph) recordEdgeHistory(e *Edge, eventID string) {
	key := fmt.Sprintf("%s|%s|%s", e.SourceID, e.TargetID, e.Type)

	if g.EdgeHistories == nil {
		g.EdgeHistories = make(map[string]*EdgeHistory)
	}

	history, exists := g.EdgeHistories[key]
	if !exists {
		history = &EdgeHistory{
			SourceID: e.SourceID,
			TargetID: e.TargetID,
			Type:     e.Type,
			History:  make([]EdgeSnapshot, 0),
		}
		g.EdgeHistories[key] = history
	}

	snapshot := EdgeSnapshot{
		Weight:    e.Weight,
		Timestamp: e.Timestamp,
		Status:    e.Status,
		EventID:   eventID,
	}

	history.History = append(history.History, snapshot)
}

// UpdateEdgeWeight updates an edge's weight using the decay-based formula from Section 5.1.
// Formula: W_ij^(t) = W_ij^(t-1) * e^(-λ) + Σ(S_k * R_k)
// where:
//   - λ (lambda) is the temporal decay factor (forgetting mechanism)
//   - S_k is the sentiment score of news event k (range: -1.0 to +1.0)
//   - R_k is the relevance/credibility score of the source (range: 0.0 to 1.0)
func (g *Graph) UpdateEdgeWeight(sourceID, targetID string, edgeType EdgeType, sentimentScore, relevanceScore float64, eventID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Find the edge
	var targetEdge *Edge
	for _, e := range g.Adjacency[sourceID] {
		if e.TargetID == targetID && e.Type == edgeType {
			targetEdge = e
			break
		}
	}

	if targetEdge == nil {
		return fmt.Errorf("edge not found: %s -> %s (%s)", sourceID, targetID, edgeType)
	}

	// Calculate time since last update (for decay)
	timeSinceUpdate := time.Since(targetEdge.Timestamp).Hours() / 24.0 // Convert to days
	lambda := 0.05 // Decay rate (5% per day) - configurable in production

	// Apply decay: W_old * e^(-λ * t)
	// Using Taylor series approximation for e^x: e^x ≈ 1 + x + x²/2! + x³/3! + ...
	// For e^(-λt), we compute exp(-lambda * timeSinceUpdate)
	exponent := -lambda * timeSinceUpdate
	decayFactor := expApprox(exponent)

	previousWeight := targetEdge.Weight
	decayedWeight := previousWeight * decayFactor

	// Apply sentiment impact: Σ(S_k * R_k)
	sentimentImpact := sentimentScore * relevanceScore

	// New weight
	newWeight := decayedWeight + sentimentImpact

	// Clamp weight to reasonable bounds [0.0, 1.0] for normalized edges
	// or [-1.0, 1.0] if we allow negative relationships
	if newWeight < 0.0 {
		newWeight = 0.0
	}
	if newWeight > 1.0 {
		newWeight = 1.0
	}

	// Update edge
	targetEdge.Weight = newWeight
	targetEdge.Timestamp = time.Now()

	// Update status based on weight threshold
	if newWeight < 0.1 {
		targetEdge.Status = "Blocked"
	} else if newWeight < 0.3 {
		targetEdge.Status = "Weak"
	} else if newWeight < 0.7 {
		targetEdge.Status = "Active"
	} else {
		targetEdge.Status = "Strong"
	}

	// Record in history
	g.recordEdgeHistory(targetEdge, eventID)

	return nil
}

// GetOutgoingEdges returns edges starting from the given node ID.
func (g *Graph) GetOutgoingEdges(id string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if list, ok := g.Adjacency[id]; ok {
		// Return a copy to be safe from concurrent modifications
		result := make([]*Edge, len(list))
		copy(result, list)
		return result
	}
	return nil
}

// GetIncomingEdges returns edges pointing to the given node ID.
func (g *Graph) GetIncomingEdges(id string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	result := make([]*Edge, 0)
	for _, edge := range g.Edges {
		if edge.TargetID == id {
			result = append(result, edge)
		}
	}
	return result
}

// GetNode retrieves a node by ID.
func (g *Graph) GetNode(id string) (*Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.Nodes[id]
	return n, ok
}

// String returns a summary of the graph.
func (g *Graph) String() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return fmt.Sprintf("Graph(Nodes: %d, Edges: %d)", len(g.Nodes), len(g.Edges))
}

// NodesRange safely iterates over a copy of nodes to avoid long locks.
func (g *Graph) NodesRange(f func(*Node)) {
	g.mu.RLock()
	// Snapshot references to avoid holding lock during callback
	snapshot := make([]*Node, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		snapshot = append(snapshot, n)
	}
	g.mu.RUnlock()

	for _, n := range snapshot {
		f(n)
	}
}

// EdgesRange safely iterates over a copy of edges to avoid long locks.
func (g *Graph) EdgesRange(f func(*Edge)) {
	g.mu.RLock()
	// Snapshot references to avoid holding lock during callback
	snapshot := make([]*Edge, len(g.Edges))
	copy(snapshot, g.Edges)
	g.mu.RUnlock()

	for _, e := range snapshot {
		f(e)
	}
}

// Save writes the graph to a JSON file.
func (g *Graph) Save(filename string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// expApprox computes e^x using Taylor series approximation
// e^x ≈ 1 + x + x²/2! + x³/3! + x⁴/4! + ...
func expApprox(x float64) float64 {
	const terms = 20 // Number of terms in series for accuracy
	result := 1.0
	term := 1.0

	for i := 1; i < terms; i++ {
		term *= x / float64(i)
		result += term
		// Early exit if term becomes negligible
		if term < 1e-10 && term > -1e-10 {
			break
		}
	}

	return result
}

// Load reads a graph from a JSON file.
func Load(filename string) (*Graph, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}

	// Initialize maps
	if g.Nodes == nil {
		g.Nodes = make(map[string]*Node)
	}
	if g.EdgeHistories == nil {
		g.EdgeHistories = make(map[string]*EdgeHistory)
	}
	g.Adjacency = make(map[string][]*Edge) // Rebuild cache

	// Populate Adjacency and migrate directionality
	if g.Edges == nil {
		g.Edges = make([]*Edge, 0)
	} else {
		for _, e := range g.Edges {
			// Migrate: Set directionality for edges that don't have it
			if e.Directionality == "" {
				e.Directionality = GetEdgeDirectionality(e.Type)
			}
			g.Adjacency[e.SourceID] = append(g.Adjacency[e.SourceID], e)
		}
	}

	// Discover and add missing supply chain relationships
	addedEdges := g.DiscoverSupplyChainRelations()
	if addedEdges > 0 {
		fmt.Printf("[DISCOVERY] Added %d supply chain edges from existing relationships\n", addedEdges)
	}

	return &g, nil
}

// Replace replaces the current graph's data with another graph's data safely.
func (g *Graph) Replace(other *Graph) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Nodes = other.Nodes
	g.Edges = other.Edges
	g.EdgeHistories = other.EdgeHistories

	// Rebuild Adjacency
	g.Adjacency = make(map[string][]*Edge)
	for _, e := range other.Edges {
		g.Adjacency[e.SourceID] = append(g.Adjacency[e.SourceID], e)
	}
}

// ApplyTemporalDecay applies time-based decay to all edges in the graph
// This simulates the natural weakening of relationships over time without new events
func (g *Graph) ApplyTemporalDecay(lambda float64) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	updatedCount := 0
	now := time.Now()

	for _, edge := range g.Edges {
		// Calculate time since last update (in days)
		timeSinceUpdate := now.Sub(edge.Timestamp).Hours() / 24.0

		// Skip if updated very recently (less than 1 hour)
		if timeSinceUpdate < 1.0/24.0 {
			continue
		}

		// Apply decay: W_new = W_old * e^(-λ * t)
		exponent := -lambda * timeSinceUpdate
		decayFactor := expApprox(exponent)

		previousWeight := edge.Weight
		newWeight := previousWeight * decayFactor

		// Clamp to minimum threshold
		if newWeight < 0.01 {
			newWeight = 0.01
		}

		// Only update if there's a meaningful change
		if previousWeight-newWeight > 0.001 {
			edge.Weight = newWeight
			edge.Timestamp = now

			// Update status based on weight
			if newWeight < 0.1 {
				edge.Status = "Blocked"
			} else if newWeight < 0.3 {
				edge.Status = "Weak"
			} else if newWeight < 0.7 {
				edge.Status = "Active"
			} else {
				edge.Status = "Strong"
			}

			// Record in history
			g.recordEdgeHistory(edge, "temporal_decay")
			updatedCount++
		}
	}

	return updatedCount
}

// StartTemporalDecayWorker starts a background goroutine that periodically applies decay
func (g *Graph) StartTemporalDecayWorker(interval time.Duration, lambda float64) {
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			count := g.ApplyTemporalDecay(lambda)
			if count > 0 {
				// Use a simple print to avoid circular imports with logger
				// In production, you might want to use a callback or channel
				fmt.Printf("[DECAY] Updated %d edges with temporal decay\n", count)
			}
		}
	}()
}

// CompanyRelations holds all relationships for a company
type CompanyRelations struct {
	CompanyID    string   `json:"company_id"`
	CompanyName  string   `json:"company_name"`
	Suppliers    []*Node  `json:"suppliers"`
	Clients      []*Node  `json:"clients"`
	RawMaterials []*Node  `json:"raw_materials"`
	Products     []*Node  `json:"products"`
}

// GetSuppliers returns all companies that supply to the given company
func (g *Graph) GetSuppliers(companyID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	suppliers := make([]*Node, 0)
	seenIDs := make(map[string]bool)

	// Find companies that have Supplies edges pointing TO this company
	for _, edge := range g.Edges {
		if edge.TargetID == companyID && edge.Type == EdgeTypeSupplies {
			if supplier, ok := g.Nodes[edge.SourceID]; ok {
				if supplier.Type == NodeTypeCorporation && !seenIDs[supplier.ID] {
					suppliers = append(suppliers, supplier)
					seenIDs[supplier.ID] = true
				}
			}
		}
		// Also check for ProcuresFrom edges (this company procures FROM supplier)
		if edge.SourceID == companyID && edge.Type == EdgeTypeProcuresFrom {
			if supplier, ok := g.Nodes[edge.TargetID]; ok {
				if supplier.Type == NodeTypeCorporation && !seenIDs[supplier.ID] {
					suppliers = append(suppliers, supplier)
					seenIDs[supplier.ID] = true
				}
			}
		}
	}

	return suppliers
}

// GetClients returns all companies that the given company supplies to
func (g *Graph) GetClients(companyID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	clients := make([]*Node, 0)
	seenIDs := make(map[string]bool)

	// Find companies that this company has Supplies edges pointing TO
	for _, edge := range g.Edges {
		if edge.SourceID == companyID && edge.Type == EdgeTypeSupplies {
			if client, ok := g.Nodes[edge.TargetID]; ok {
				if client.Type == NodeTypeCorporation && !seenIDs[client.ID] {
					clients = append(clients, client)
					seenIDs[client.ID] = true
				}
			}
		}
		// Also check for ProcuresFrom edges (client procures FROM this company)
		if edge.TargetID == companyID && edge.Type == EdgeTypeProcuresFrom {
			if client, ok := g.Nodes[edge.SourceID]; ok {
				if client.Type == NodeTypeCorporation && !seenIDs[client.ID] {
					clients = append(clients, client)
					seenIDs[client.ID] = true
				}
			}
		}
	}

	return clients
}

// GetRawMaterials returns all raw materials that the given company uses
func (g *Graph) GetRawMaterials(companyID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	materials := make([]*Node, 0)
	seenIDs := make(map[string]bool)

	// Find raw materials that this company Requires or Consumes
	for _, edge := range g.Edges {
		if edge.SourceID == companyID && (edge.Type == EdgeTypeRequires || edge.Type == EdgeTypeConsumes) {
			if material, ok := g.Nodes[edge.TargetID]; ok {
				if (material.Type == NodeTypeRawMaterial || material.Type == NodeTypeCrop) && !seenIDs[material.ID] {
					materials = append(materials, material)
					seenIDs[material.ID] = true
				}
			}
		}
	}

	return materials
}

// GetProducts returns all products that the given company manufactures
func (g *Graph) GetProducts(companyID string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	products := make([]*Node, 0)
	seenIDs := make(map[string]bool)

	// Find products that this company Manufactures
	for _, edge := range g.Edges {
		if edge.SourceID == companyID && edge.Type == EdgeTypeManufactures {
			if product, ok := g.Nodes[edge.TargetID]; ok {
				if product.Type == NodeTypeProduct && !seenIDs[product.ID] {
					products = append(products, product)
					seenIDs[product.ID] = true
				}
			}
		}
	}

	return products
}

// GetCompanyRelations returns all relationships for a given company
func (g *Graph) GetCompanyRelations(companyID string) (*CompanyRelations, error) {
	g.mu.RLock()
	company, ok := g.Nodes[companyID]
	g.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("company %s not found", companyID)
	}

	if company.Type != NodeTypeCorporation {
		return nil, fmt.Errorf("node %s is not a corporation", companyID)
	}

	return &CompanyRelations{
		CompanyID:    companyID,
		CompanyName:  company.Name,
		Suppliers:    g.GetSuppliers(companyID),
		Clients:      g.GetClients(companyID),
		RawMaterials: g.GetRawMaterials(companyID),
		Products:     g.GetProducts(companyID),
	}, nil
}

// DiscoverSupplyChainRelations analyzes the graph and adds missing supplier/client edges
// based on existing relationships and patterns
func (g *Graph) DiscoverSupplyChainRelations() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	addedEdges := 0
	existingEdges := make(map[string]bool)

	// Build a map of existing edges for quick lookup
	for _, edge := range g.Edges {
		key := fmt.Sprintf("%s|%s|%s", edge.SourceID, edge.TargetID, edge.Type)
		existingEdges[key] = true
	}

	// Helper function to check if edge exists
	hasEdge := func(sourceID, targetID string, edgeType EdgeType) bool {
		key := fmt.Sprintf("%s|%s|%s", sourceID, targetID, edgeType)
		return existingEdges[key]
	}

	// Discover supplier/client relationships from DependsOn edges
	for _, edge := range g.Edges {
		if edge.Type == EdgeTypeDependsOn {
			sourceNode, sourceExists := g.Nodes[edge.SourceID]
			targetNode, targetExists := g.Nodes[edge.TargetID]

			if !sourceExists || !targetExists {
				continue
			}

			// If both are corporations and DependsOn exists, add Supplies edge
			if sourceNode.Type == NodeTypeCorporation && targetNode.Type == NodeTypeCorporation {
				// target supplies to source (source depends on target)
				if !hasEdge(edge.TargetID, edge.SourceID, EdgeTypeSupplies) {
					newEdge := &Edge{
						SourceID:       edge.TargetID,
						TargetID:       edge.SourceID,
						Type:           EdgeTypeSupplies,
						Weight:         edge.Weight,
						Status:         edge.Status,
						Directionality: DirectionalityUnidirectional,
					}
					g.Edges = append(g.Edges, newEdge)
					g.Adjacency[newEdge.SourceID] = append(g.Adjacency[newEdge.SourceID], newEdge)

					key := fmt.Sprintf("%s|%s|%s", newEdge.SourceID, newEdge.TargetID, newEdge.Type)
					existingEdges[key] = true
					addedEdges++
				}

				// Add corresponding ProcuresFrom edge
				if !hasEdge(edge.SourceID, edge.TargetID, EdgeTypeProcuresFrom) {
					newEdge := &Edge{
						SourceID:       edge.SourceID,
						TargetID:       edge.TargetID,
						Type:           EdgeTypeProcuresFrom,
						Weight:         edge.Weight,
						Status:         edge.Status,
						Directionality: DirectionalityReverse,
					}
					g.Edges = append(g.Edges, newEdge)
					g.Adjacency[newEdge.SourceID] = append(g.Adjacency[newEdge.SourceID], newEdge)

					key := fmt.Sprintf("%s|%s|%s", newEdge.SourceID, newEdge.TargetID, newEdge.Type)
					existingEdges[key] = true
					addedEdges++
				}
			}
		}
	}

	// Discover supply chain relationships from Trade edges between corporations
	for _, edge := range g.Edges {
		if edge.Type == EdgeTypeTrade {
			sourceNode, sourceExists := g.Nodes[edge.SourceID]
			targetNode, targetExists := g.Nodes[edge.TargetID]

			if !sourceExists || !targetExists {
				continue
			}

			// If both are corporations with Trade relationship, infer potential supply chain
			if sourceNode.Type == NodeTypeCorporation && targetNode.Type == NodeTypeCorporation {
				// Check if one company requires materials that the other might supply
				// This is a heuristic - in real scenarios, this would need more intelligence

				// For now, we'll use industry/product relationships to infer supply chains
				// This is a placeholder for more sophisticated discovery logic
			}
		}
	}

	// Discover manufacturing relationships
	// If a company requires raw materials and there are products, infer manufacturing
	companiesWithMaterials := make(map[string][]string) // company -> materials
	companiesWithProducts := make(map[string][]string)  // company -> products

	for _, edge := range g.Edges {
		if edge.Type == EdgeTypeRequires || edge.Type == EdgeTypeConsumes {
			sourceNode, exists := g.Nodes[edge.SourceID]
			targetNode, targetExists := g.Nodes[edge.TargetID]

			if exists && targetExists && sourceNode.Type == NodeTypeCorporation {
				if targetNode.Type == NodeTypeRawMaterial || targetNode.Type == NodeTypeCrop {
					companiesWithMaterials[edge.SourceID] = append(companiesWithMaterials[edge.SourceID], edge.TargetID)
				}
			}
		}

		if edge.Type == EdgeTypeManufactures {
			sourceNode, exists := g.Nodes[edge.SourceID]
			if exists && sourceNode.Type == NodeTypeCorporation {
				companiesWithProducts[edge.SourceID] = append(companiesWithProducts[edge.SourceID], edge.TargetID)
			}
		}
	}

	return addedEdges
}

// GetAllCompanies returns a list of all corporations in the graph
func (g *Graph) GetAllCompanies() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	companies := make([]*Node, 0)
	for _, node := range g.Nodes {
		if node.Type == NodeTypeCorporation {
			companies = append(companies, node)
		}
	}
	return companies
}
