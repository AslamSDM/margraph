package simulation

import (
	"fmt"
	"margraf/graph"
	"margraf/logger"
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
	logger.Info(logger.StatusShock, "SIMULATING SHOCK: %s on %s (Factor: %.2f)", event.Description, event.TargetNodeID, event.ImpactFactor)

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

	logger.InfoDepth(1, logger.StatusHlth, "Node Health: %.2f -> Effective Impact Factor: %.2f", target.Health, effectiveImpact)

	// Apply damage to the node itself
	s.Graph.UpdateNodeHealth(event.TargetNodeID, -0.2)

	// Spreading Activation: Propagate impact through the graph
	logger.InfoDepth(1, "", "Direct Impact on %s:", target.Name)

	// Track propagation across multiple hops
	activationMap := make(map[string]float64) // nodeID -> activation energy
	activationMap[event.TargetNodeID] = 1.0 - effectiveImpact // Initial shock energy

	// First-order propagation - respect edge directionality
	outgoing := s.Graph.GetOutgoingEdges(event.TargetNodeID)
	impactedNodeIDs := make([]string, 0)
	winners := make([]string, 0) // Track nodes that benefit (substitutes, competitors)

	for _, e := range outgoing {
		// Check if shock should propagate through this edge (respects directionality)
		if !graph.ShouldPropagateShock(e, true) {
			logger.InfoDepth(2, "", "Skipping %s -> %s (%s): Wrong direction for shock propagation",
				target.Name, e.TargetID, e.Type)
			continue
		}

		neighbor, _ := s.Graph.GetNode(e.TargetID)
		originalWeight := e.Weight

		// Get propagation factor based on edge type
		propagationFactor := graph.GetShockPropagationFactor(e.Type)

		// Calculate new weight based on shock
		newWeight := originalWeight * effectiveImpact

		// Actually update the edge weight in the graph
		sentimentScore := -(1.0 - effectiveImpact) // Negative shock
		relevanceScore := 1.0 // Direct connection = high relevance
		eventID := fmt.Sprintf("shock_%s_%d", event.TargetNodeID, len(activationMap))

		if err := s.Graph.UpdateEdgeWeight(e.SourceID, e.TargetID, e.Type, sentimentScore, relevanceScore, eventID); err == nil {
			logger.SuccessDepth(2, "%s -> %s [%s]: Weight %.2f -> %.2f (-%0.f%%, propagation: %.0f%%)",
				target.Name, neighbor.Name, e.Type, originalWeight, newWeight,
				(1.0-effectiveImpact)*100, propagationFactor*100)

			// Propagate activation energy with edge-specific factor
			activationMap[e.TargetID] = (1.0 - effectiveImpact) * e.Weight * propagationFactor

			// Apply health impact to downstream node (scaled by propagation factor)
			healthDelta := -0.1 * (1.0 - effectiveImpact) * propagationFactor
			s.Graph.UpdateNodeHealth(e.TargetID, healthDelta)

			impactedNodeIDs = append(impactedNodeIDs, e.TargetID)
		}
	}

	// Also check for reverse-direction edges (e.g., ProcuresFrom)
	// These would be incoming edges where we are the target, but shock flows backwards
	s.propagateReverseShocks(event.TargetNodeID, target, effectiveImpact, activationMap, &impactedNodeIDs)

	// Identify WINNERS: Find substitute and competitor nodes
	s.identifyWinners(event.TargetNodeID, &winners)

	if len(winners) > 0 {
		logger.Info(logger.StatusFin, "WINNERS (Positive Impact):")
		for _, winnerID := range winners {
			winner, _ := s.Graph.GetNode(winnerID)
			logger.SuccessDepth(2, "%s (Substitute/Competitor) - Expected demand increase", winner.Name)

			// Apply positive health boost
			s.Graph.UpdateNodeHealth(winnerID, +0.15)
		}
	}

	// Second-order ripple effects with actual propagation
	if len(impactedNodeIDs) > 0 {
		logger.InfoDepth(1, logger.StatusRipple, "Ripple Effects (2nd Order):")
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

				logger.InfoDepth(2, "", "%s -> %s: Reduced flow (Activation: %.2f)", impactedNode.Name, downstream.Name, activation)

				// Propagate to third order if significant
				if activation > 0.15 {
					activationMap[e.TargetID] = activation * 0.3 // 30% for third order
				}
			}
		}
	}

	logger.InfoDepth(1, logger.StatusData, "Summary: %d directly impacted, %d winners identified", len(impactedNodeIDs), len(winners))
}

// identifyWinners finds nodes that benefit from the shock (substitutes, competitors).
func (s *Simulator) identifyWinners(shockedNodeID string, winners *[]string) {
	// Strategy 1: Find SUBSTITUTE_FOR edges pointing to the shocked node's products
	shockedNode, ok := s.Graph.GetNode(shockedNodeID)
	if !ok {
		return
	}

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

	// Strategy 2: Find direct competitors via COMPETES_WITH edges only
	// Don't add all nodes of the same type - that's too aggressive
	s.Graph.EdgesRange(func(e *graph.Edge) {
		if e.Type == graph.EdgeTypeCompetesWith {
			if e.SourceID == shockedNodeID {
				*winners = append(*winners, e.TargetID)
			} else if e.TargetID == shockedNodeID {
				*winners = append(*winners, e.SourceID)
			}
		}
		// Also check SUBSTITUTE_FOR edges
		if e.Type == graph.EdgeTypeSubstituteFor {
			if e.TargetID == shockedNodeID {
				*winners = append(*winners, e.SourceID)
			}
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

// propagateReverseShocks handles edges where shocks flow backwards (client -> supplier)
func (s *Simulator) propagateReverseShocks(targetNodeID string, target *graph.Node, effectiveImpact float64, activationMap map[string]float64, impactedNodeIDs *[]string) {
	// We need to check all edges in the graph where we are the TARGET
	// and the edge has reverse directionality
	// Use thread-safe edge iteration
	s.Graph.EdgesRange(func(edge *graph.Edge) {
		if edge.TargetID != targetNodeID {
			return
		}

		// Check if this is a reverse-direction edge
		if !graph.ShouldPropagateShock(edge, false) {
			return
		}

		// Shock propagates backwards (from target to source)
		upstream, ok := s.Graph.GetNode(edge.SourceID)
		if !ok {
			return
		}

		propagationFactor := graph.GetShockPropagationFactor(edge.Type)
		originalWeight := edge.Weight
		newWeight := originalWeight * effectiveImpact

		sentimentScore := -(1.0 - effectiveImpact)
		relevanceScore := 1.0
		eventID := fmt.Sprintf("shock_%s_reverse", targetNodeID)

		if err := s.Graph.UpdateEdgeWeight(edge.SourceID, edge.TargetID, edge.Type, sentimentScore, relevanceScore, eventID); err == nil {
			logger.SuccessDepth(2, "%s <- %s [%s REVERSE]: Weight %.2f -> %.2f (upstream impact: %.0f%%)",
				upstream.Name, target.Name, edge.Type, originalWeight, newWeight, propagationFactor*100)

			// Propagate activation energy upstream
			activationMap[edge.SourceID] = (1.0 - effectiveImpact) * edge.Weight * propagationFactor

			// Apply health impact to upstream node
			healthDelta := -0.05 * (1.0 - effectiveImpact) * propagationFactor // Weaker upstream impact
			s.Graph.UpdateNodeHealth(edge.SourceID, healthDelta)

			*impactedNodeIDs = append(*impactedNodeIDs, edge.SourceID)
		}
	})
}
