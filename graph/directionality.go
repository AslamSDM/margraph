package graph

import "fmt"

// GetEdgeDirectionality returns the directionality for a given edge type
// This determines how supply shocks propagate through the graph
func GetEdgeDirectionality(edgeType EdgeType) EdgeDirectionality {
	switch edgeType {
	// Supply chain edges - shocks flow downstream (supplier -> client)
	case EdgeTypeSupplies:
		return DirectionalityUnidirectional
	case EdgeTypeManufactures:
		return DirectionalityUnidirectional
	case EdgeTypeProduces:
		return DirectionalityUnidirectional
	case EdgeTypeHasIndustry:
		return DirectionalityUnidirectional
	case EdgeTypeHasCompany:
		return DirectionalityUnidirectional

	// Reverse flow edges - shocks flow upstream (client -> supplier)
	case EdgeTypeProcuresFrom:
		return DirectionalityReverse
	case EdgeTypeRequires:
		return DirectionalityReverse // When a company requires something, shock to company affects supplier
	case EdgeTypeConsumes:
		return DirectionalityReverse
	case EdgeTypeDependsOn:
		return DirectionalityReverse

	// Bidirectional edges - shocks flow both ways
	case EdgeTypeTrade:
		return DirectionalityBidirectional
	case EdgeTypeCapital:
		return DirectionalityBidirectional
	case EdgeTypeCompetesWith:
		return DirectionalityBidirectional
	case EdgeTypeSubstituteFor:
		return DirectionalityBidirectional
	case EdgeTypeRegulatory:
		return DirectionalityBidirectional

	default:
		// Default to bidirectional for unknown types
		return DirectionalityBidirectional
	}
}

// ShouldPropagateShock determines if a shock should propagate through an edge
// based on the edge's directionality and the direction of propagation
func ShouldPropagateShock(edge *Edge, fromSource bool) bool {
	if edge.Directionality == "" {
		// If not set, determine from type
		edge.Directionality = GetEdgeDirectionality(edge.Type)
	}

	switch edge.Directionality {
	case DirectionalityUnidirectional:
		// Only propagate from source to target
		return fromSource

	case DirectionalityReverse:
		// Only propagate from target to source
		return !fromSource

	case DirectionalityBidirectional:
		// Propagate in both directions
		return true

	default:
		return true
	}
}

// GetShockPropagationFactor returns how much of the shock energy propagates through this edge type
// Some edge types attenuate shocks more than others
func GetShockPropagationFactor(edgeType EdgeType) float64 {
	switch edgeType {
	// Strong propagation - direct supply chain relationships
	case EdgeTypeSupplies:
		return 0.9 // 90% of shock propagates downstream
	case EdgeTypeManufactures:
		return 0.9
	case EdgeTypeProduces:
		return 0.8
	case EdgeTypeProcuresFrom:
		return 0.7 // Upstream propagation slightly weaker
	case EdgeTypeConsumes:
		return 0.7
	case EdgeTypeRequires:
		return 0.7
	case EdgeTypeDependsOn:
		return 0.8

	// Medium propagation - trade and capital
	case EdgeTypeTrade:
		return 0.6
	case EdgeTypeCapital:
		return 0.5

	// Weak propagation - indirect relationships
	case EdgeTypeCompetesWith:
		return 0.3 // Competitors less directly affected
	case EdgeTypeSubstituteFor:
		return 0.4
	case EdgeTypeRegulatory:
		return 0.4
	case EdgeTypeHasIndustry:
		return 0.6
	case EdgeTypeHasCompany:
		return 0.5

	default:
		return 0.5 // Default medium propagation
	}
}

// EdgeDirectionalityDescription returns a human-readable description
func EdgeDirectionalityDescription(edgeType EdgeType) string {
	dir := GetEdgeDirectionality(edgeType)
	factor := GetShockPropagationFactor(edgeType)

	switch dir {
	case DirectionalityUnidirectional:
		return fmt.Sprintf("Unidirectional (supplier→client, %.0f%% propagation)", factor*100)
	case DirectionalityReverse:
		return fmt.Sprintf("Reverse (client→supplier, %.0f%% propagation)", factor*100)
	case DirectionalityBidirectional:
		return fmt.Sprintf("Bidirectional (both ways, %.0f%% propagation)", factor*100)
	default:
		return "Unknown directionality"
	}
}
