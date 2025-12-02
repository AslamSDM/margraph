package simulation

import (
	"fmt"
	"margraf/graph"
)

// Simulator handles shock propagation.
type Simulator struct {
	Graph *graph.Graph
}

func NewSimulator(g *graph.Graph) *Simulator {
	return &Simulator{Graph: g}
}

// ShockEvent represents a disruption.
type ShockEvent struct {
	TargetNodeID string
	Description  string
	ImpactFactor float64 // 0.0 to 1.0 (1.0 = no change, 0.0 = total block)
}

// RunShock simulates a shock event using Spreading Activation (Section 5.2).
func (s *Simulator) RunShock(event ShockEvent) {
	fmt.Printf("\n⚡ SIMULATING SHOCK: %s on %s (Factor: %.2f)\n", event.Description, event.TargetNodeID, event.ImpactFactor)

	target, ok := s.Graph.GetNode(event.TargetNodeID)
	if !ok {
		fmt.Printf("Target node %s not found.\n", event.TargetNodeID)
		return
	}

	// Health-based Resilience
	// Health > 1.0 reduces impact. Health < 1.0 amplifies it.
	resilience := target.Health
	if resilience <= 0.1 {
		resilience = 0.1 // Prevent division by zero/infinity
	}

	// Impact factor is a multiplier on flow. 1.0 = no change, 0.0 = total block.
	// Formula: NewFactor = 1.0 - ((1.0 - Factor) / Resilience)
	effectiveImpact := 1.0 - ((1.0 - event.ImpactFactor) / resilience)
	if effectiveImpact < 0 {
		effectiveImpact = 0
	}
	if effectiveImpact > 1 {
		effectiveImpact = 1
	}

	fmt.Printf("  🏥 Node Health: %.2f -> Effective Impact Factor: %.2f\n", target.Health, effectiveImpact)

	// Apply damage to the node itself
	s.Graph.UpdateNodeHealth(event.TargetNodeID, -0.2)

	// Spreading Activation: Propagate impact through the graph
	fmt.Printf("  Direct Impact on %s:\n", target.Name)

	// Track propagation across multiple hops
	activationMap := make(map[string]float64) // nodeID -> activation energy
	activationMap[event.TargetNodeID] = 1.0 - effectiveImpact // Initial shock energy

	// First-order propagation
	outgoing := s.Graph.GetOutgoingEdges(event.TargetNodeID)
	impactedNodeIDs := make([]string, 0)
	winners := make([]string, 0) // Track nodes that benefit (substitutes, competitors)

	for _, e := range outgoing {
		neighbor, _ := s.Graph.GetNode(e.TargetID)
		originalWeight := e.Weight

		// Calculate new weight based on shock
		newWeight := originalWeight * effectiveImpact

		// Actually update the edge weight in the graph (THIS WAS MISSING!)
		sentimentScore := -(1.0 - effectiveImpact) // Negative shock
		relevanceScore := 1.0 // Direct connection = high relevance
		eventID := fmt.Sprintf("shock_%s_%d", event.TargetNodeID, len(activationMap))

		if err := s.Graph.UpdateEdgeWeight(e.SourceID, e.TargetID, e.Type, sentimentScore, relevanceScore, eventID); err == nil {
			fmt.Printf("    ✓ %s -> %s: Weight %.2f -> %.2f (-%0.f%%)\n", target.Name, neighbor.Name, originalWeight, newWeight, (1.0-effectiveImpact)*100)

			// Propagate activation energy
			activationMap[e.TargetID] = (1.0 - effectiveImpact) * e.Weight * 0.7 // 70% pass-through

			// Apply health impact to downstream node
			healthDelta := -0.1 * (1.0 - effectiveImpact)
			s.Graph.UpdateNodeHealth(e.TargetID, healthDelta)

			impactedNodeIDs = append(impactedNodeIDs, e.TargetID)
		}
	}

	// Identify WINNERS: Find substitute and competitor nodes
	s.identifyWinners(event.TargetNodeID, &winners)

	if len(winners) > 0 {
		fmt.Println("\n  💰 WINNERS (Positive Impact):")
		for _, winnerID := range winners {
			winner, _ := s.Graph.GetNode(winnerID)
			fmt.Printf("    ✓ %s (Substitute/Competitor) - Expected demand increase\n", winner.Name)

			// Apply positive health boost
			s.Graph.UpdateNodeHealth(winnerID, +0.15)
		}
	}

	// Second-order ripple effects with actual propagation
	if len(impactedNodeIDs) > 0 {
		fmt.Println("\n  🌊 Ripple Effects (2nd Order):")
		for _, impactedID := range impactedNodeIDs {
			impactedNode, _ := s.Graph.GetNode(impactedID)
			activation := activationMap[impactedID]

			if activation < 0.05 {
				continue // Skip negligible propagation
			}

			secondaryOutgoing := s.Graph.GetOutgoingEdges(impactedID)
			for _, e := range secondaryOutgoing {
				downstream, _ := s.Graph.GetNode(e.TargetID)

				// Propagate reduced activation (50% attenuation per hop)
				sentimentScore := -activation * 0.5
				relevanceScore := 0.7 // Indirect connection
				eventID := fmt.Sprintf("shock_%s_2nd_%s", event.TargetNodeID, impactedID)

				s.Graph.UpdateEdgeWeight(e.SourceID, e.TargetID, e.Type, sentimentScore, relevanceScore, eventID)

				fmt.Printf("    - %s -> %s: Reduced flow (Activation: %.2f)\n", impactedNode.Name, downstream.Name, activation)

				// Propagate to third order if significant
				if activation > 0.15 {
					activationMap[e.TargetID] = activation * 0.3 // 30% for third order
				}
			}
		}
	}

	fmt.Printf("\n  📊 Summary: %d directly impacted, %d winners identified\n", len(impactedNodeIDs), len(winners))
}

// identifyWinners finds nodes that benefit from the shock (substitutes, competitors).
func (s *Simulator) identifyWinners(shockedNodeID string, winners *[]string) {
	// Strategy 1: Find SUBSTITUTE_FOR edges pointing to the shocked node's products
	shockedNode, _ := s.Graph.GetNode(shockedNodeID)

	// If it's a nation or produces commodities, find substitutes
	if shockedNode.Type == graph.NodeTypeNation || shockedNode.Type == graph.NodeTypeRawMaterial {
		// Find what the shocked node produces
		outgoing := s.Graph.GetOutgoingEdges(shockedNodeID)
		for _, e := range outgoing {
			if e.Type == graph.EdgeTypeProduces {
				// Find substitutes for this commodity
				s.findSubstitutes(e.TargetID, winners)
			}
		}
	}

	// Strategy 2: Find direct competitors
	s.Graph.NodesRange(func(n *graph.Node) {
		if n.ID == shockedNodeID {
			return
		}

		// Same type and category = potential competitor benefiting from shock
		if n.Type == shockedNode.Type {
			// Check if there's a COMPETES_WITH edge (future enhancement)
			// For now, assume nodes of same type in same industry benefit
			*winners = append(*winners, n.ID)
		}
	})
}

// findSubstitutes identifies alternative suppliers/products
func (s *Simulator) findSubstitutes(commodityID string, winners *[]string) {
	// Find all nodes that produce this commodity (alternative suppliers)
	s.Graph.NodesRange(func(n *graph.Node) {
		edges := s.Graph.GetOutgoingEdges(n.ID)
		for _, e := range edges {
			if e.Type == graph.EdgeTypeProduces && e.TargetID == commodityID {
				*winners = append(*winners, n.ID)
			}
		}
	})
}
