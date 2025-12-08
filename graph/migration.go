package graph

import (
	"fmt"
)

// MigrateEdgeDirectionality updates all edges in the graph to have directionality
// This is useful for existing graphs that were created before directionality was added
func (g *Graph) MigrateEdgeDirectionality() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	updated := 0
	for _, edge := range g.Edges {
		if edge.Directionality == "" {
			edge.Directionality = GetEdgeDirectionality(edge.Type)
			updated++
		}
	}

	return updated
}

// ValidateEdgeDirectionality checks if all edges have directionality set
func (g *Graph) ValidateEdgeDirectionality() (bool, []string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var missingDirectionality []string
	for i, edge := range g.Edges {
		if edge.Directionality == "" {
			missingDirectionality = append(missingDirectionality,
				fmt.Sprintf("Edge %d: %s -> %s [%s]", i, edge.SourceID, edge.TargetID, edge.Type))
		}
	}

	return len(missingDirectionality) == 0, missingDirectionality
}

// PrintEdgeDirectionalityReport prints a summary of edge directionalities in the graph
func (g *Graph) PrintEdgeDirectionalityReport() {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Count by directionality type
	counts := make(map[EdgeDirectionality]int)
	for _, edge := range g.Edges {
		if edge.Directionality == "" {
			counts["Unset"]++
		} else {
			counts[edge.Directionality]++
		}
	}

	fmt.Println("\nEdge Directionality Report:")
	fmt.Println("================================================================================")
	fmt.Printf("Total Edges: %d\n\n", len(g.Edges))

	for directionality, count := range counts {
		percentage := float64(count) / float64(len(g.Edges)) * 100
		fmt.Printf("  %-20s: %4d (%.1f%%)\n", directionality, count, percentage)
	}
	fmt.Println("================================================================================")
	fmt.Println()
}

// GetEdgesByDirectionality returns all edges with a specific directionality
func (g *Graph) GetEdgesByDirectionality(directionality EdgeDirectionality) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Edge
	for _, edge := range g.Edges {
		if edge.Directionality == directionality {
			result = append(result, edge)
		}
	}

	return result
}
