package graph

import (
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
