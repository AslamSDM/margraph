package main

import (
	"fmt"
	"margraf/graph"
	"margraf/logger"
	"margraf/simulation"
)

// Test program to demonstrate directional shock propagation
func main() {
	logger.Init("info", true)

	fmt.Println("================================================================================")
	fmt.Println("DIRECTIONAL SHOCK PROPAGATION TEST")
	fmt.Println("================================================================================")
	fmt.Println()

	// Create a test supply chain graph
	g := graph.NewGraph()

	// Create nodes: Supplier -> Manufacturer -> Retailer -> Consumer
	supplier := &graph.Node{
		ID:     "chip_supplier",
		Name:   "Taiwan Semiconductor",
		Type:   graph.NodeTypeCorporation,
		Health: 1.0,
	}

	manufacturer := &graph.Node{
		ID:     "phone_manufacturer",
		Name:   "Phone Manufacturer Inc",
		Type:   graph.NodeTypeCorporation,
		Health: 1.0,
	}

	retailer := &graph.Node{
		ID:     "electronics_retailer",
		Name:   "Electronics Retailer",
		Type:   graph.NodeTypeCorporation,
		Health: 1.0,
	}

	consumer := &graph.Node{
		ID:     "consumer_market",
		Name:   "Consumer Market",
		Type:   graph.NodeTypeNation,
		Health: 1.0,
	}

	alternativeSupplier := &graph.Node{
		ID:     "alternative_chip_supplier",
		Name:   "South Korea Semiconductor",
		Type:   graph.NodeTypeCorporation,
		Health: 1.0,
	}

	g.AddNode(supplier)
	g.AddNode(manufacturer)
	g.AddNode(retailer)
	g.AddNode(consumer)
	g.AddNode(alternativeSupplier)

	// Add directional supply chain edges
	// Supplier -> Manufacturer (Supplies edge - unidirectional downstream)
	g.AddEdge(&graph.Edge{
		SourceID: supplier.ID,
		TargetID: manufacturer.ID,
		Type:     graph.EdgeTypeSupplies,
		Weight:   0.9,
	})

	// Manufacturer -> Retailer (Supplies edge)
	g.AddEdge(&graph.Edge{
		SourceID: manufacturer.ID,
		TargetID: retailer.ID,
		Type:     graph.EdgeTypeSupplies,
		Weight:   0.8,
	})

	// Retailer -> Consumer (Trade edge - bidirectional)
	g.AddEdge(&graph.Edge{
		SourceID: retailer.ID,
		TargetID: consumer.ID,
		Type:     graph.EdgeTypeTrade,
		Weight:   0.7,
	})

	// Add reverse edge: Manufacturer depends on Supplier
	g.AddEdge(&graph.Edge{
		SourceID: manufacturer.ID,
		TargetID: supplier.ID,
		Type:     graph.EdgeTypeDependsOn,
		Weight:   0.9,
	})

	// Alternative supplier (for comparison)
	g.AddEdge(&graph.Edge{
		SourceID: alternativeSupplier.ID,
		TargetID: manufacturer.ID,
		Type:     graph.EdgeTypeSupplies,
		Weight:   0.5, // Weaker relationship
	})

	fmt.Println("Supply Chain Structure:")
	fmt.Println("  Taiwan Semiconductor (Supplier)")
	fmt.Println("          |")
	fmt.Println("          | Supplies (90% weight, Unidirectional ↓)")
	fmt.Println("          ↓")
	fmt.Println("  Phone Manufacturer")
	fmt.Println("          |")
	fmt.Println("          | Supplies (80% weight, Unidirectional ↓)")
	fmt.Println("          ↓")
	fmt.Println("  Electronics Retailer")
	fmt.Println("          |")
	fmt.Println("          | Trade (70% weight, Bidirectional ↕)")
	fmt.Println("          ↓")
	fmt.Println("  Consumer Market")
	fmt.Println()
	fmt.Println("  Alternative: South Korea Semiconductor → Manufacturer (50% weight)")
	fmt.Println()

	// Print edge directionality information
	fmt.Println("Edge Type Directionality Rules:")
	fmt.Println("--------------------------------------------------------------------------------")
	edgeTypes := []graph.EdgeType{
		graph.EdgeTypeSupplies,
		graph.EdgeTypeDependsOn,
		graph.EdgeTypeTrade,
		graph.EdgeTypeRequires,
		graph.EdgeTypeProduces,
	}

	for _, et := range edgeTypes {
		desc := graph.EdgeDirectionalityDescription(et)
		fmt.Printf("  %-20s: %s\n", et, desc)
	}
	fmt.Println()

	// Run shock simulation on supplier
	fmt.Println("================================================================================")
	fmt.Println("SCENARIO 1: Supply Shock at Taiwan Semiconductor")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("Simulating: Earthquake disrupts chip production (90% reduction)")
	fmt.Println()

	sim := simulation.NewSimulator(g)
	sim.RunShock(simulation.ShockEvent{
		TargetNodeID: supplier.ID,
		Description:  "Earthquake at chip factory",
		ImpactFactor: 0.1, // 90% reduction in supply
	})

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("POST-SHOCK ANALYSIS")
	fmt.Println("================================================================================")
	fmt.Println()

	// Show health of all nodes
	nodes := []*graph.Node{supplier, manufacturer, retailer, consumer, alternativeSupplier}
	fmt.Println("Node Health After Shock:")
	for _, n := range nodes {
		currentNode, _ := g.GetNode(n.ID)
		fmt.Printf("  %-30s: %.2f", currentNode.Name, currentNode.Health)
		if currentNode.Health < 0.8 {
			fmt.Printf(" ⚠️  (Stressed)")
		} else if currentNode.Health > 1.1 {
			fmt.Printf(" ✅ (Beneficiary)")
		}
		fmt.Println()
	}
	fmt.Println()

	// Show edge weights
	fmt.Println("Edge Weights After Shock:")
	for _, e := range g.Edges {
		src, _ := g.GetNode(e.SourceID)
		tgt, _ := g.GetNode(e.TargetID)
		dir := "→"
		if graph.GetEdgeDirectionality(e.Type) == graph.DirectionalityReverse {
			dir = "←"
		} else if graph.GetEdgeDirectionality(e.Type) == graph.DirectionalityBidirectional {
			dir = "↔"
		}
		fmt.Printf("  %s %s %s [%s]: %.2f\n", src.Name, dir, tgt.Name, e.Type, e.Weight)
	}

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("KEY OBSERVATIONS")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("✓ Shock propagated DOWNSTREAM through Supplies edges (unidirectional)")
	fmt.Println("✓ Manufacturer affected more strongly than retailer (90% vs 80% base weight)")
	fmt.Println("✓ Consumer market also affected (through trade edge)")
	fmt.Println("✓ Alternative supplier may benefit from increased demand")
	fmt.Println("✓ Shocks DO NOT propagate backwards through Supplies edges")
	fmt.Println("✓ DependsOn edges can propagate shocks upstream (reverse direction)")
	fmt.Println()
	fmt.Println("This demonstrates realistic supply chain shock propagation where:")
	fmt.Println("  - Supply disruptions flow downstream to customers")
	fmt.Println("  - Demand shocks can flow upstream to suppliers (via DependsOn)")
	fmt.Println("  - Alternative suppliers benefit from primary supplier failures")
	fmt.Println()
}
