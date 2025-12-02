package graph

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToDOT returns the graph in Graphviz DOT format.
func (g *Graph) ToDOT() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var w strings.Builder
	w.WriteString("digraph FDKG {\n")
	w.WriteString("  rankdir=LR;\n")
	w.WriteString("  node [shape=box, style=filled, fontname=\"Arial\"];\n")

	// Nodes
	for _, n := range g.Nodes {
		color := "lightgrey"
		switch n.Type {
		case NodeTypeNation:
			color = "lightblue"
		case NodeTypeCorporation:
			color = "salmon"
		case NodeTypeIndustry:
			color = "lightyellow"
		case NodeTypeRawMaterial:
			color = "lightgreen"
		}
		
		// Label with Price if available
		label := fmt.Sprintf("%s\n(%s)\nHealth: %.2f", n.Name, n.Type, n.Health)
		if n.Price > 0 {
			label += fmt.Sprintf("\n$%.2f", n.Price)
		}

		w.WriteString(fmt.Sprintf("  \"%s\" [label=\" %s \", fillcolor=\" %s \"];\n", n.ID, label, color))
	}

	// Edges
	for _, e := range g.Edges {
		w.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [label=\" %s \", weight=%.2f];\n", e.SourceID, e.TargetID, e.Type, e.Weight))
	}

	w.WriteString("}\n")
	return w.String()
}

// GraphData represents the graph in a format suitable for D3.js force-directed layouts
type GraphData struct {
	Nodes []NodeData `json:"nodes"`
	Links []LinkData `json:"links"`
}

// NodeData represents a node for visualization
type NodeData struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Health float64 `json:"health"`
	Price  float64 `json:"price,omitempty"`
	Ticker string  `json:"ticker,omitempty"`
}

// LinkData represents an edge for visualization
type LinkData struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Type   string  `json:"type"`
	Weight float64 `json:"weight"`
	Status string  `json:"status"`
}

// ToJSON returns the graph in a JSON format suitable for D3.js force-directed graphs
func (g *Graph) ToJSON() (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	data := GraphData{
		Nodes: make([]NodeData, 0, len(g.Nodes)),
		Links: make([]LinkData, 0, len(g.Edges)),
	}

	// Convert nodes
	for _, n := range g.Nodes {
		data.Nodes = append(data.Nodes, NodeData{
			ID:     n.ID,
			Name:   n.Name,
			Type:   string(n.Type),
			Health: n.Health,
			Price:  n.Price,
			Ticker: n.Ticker,
		})
	}

	// Convert edges
	for _, e := range g.Edges {
		data.Links = append(data.Links, LinkData{
			Source: e.SourceID,
			Target: e.TargetID,
			Type:   string(e.Type),
			Weight: e.Weight,
			Status: e.Status,
		})
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}
