package server

import (
	"encoding/json"
	"margraf/graph"
	"margraf/logger"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for prototype
	},
}

type BroadcastMessage struct {
	Type    string      `json:"type"`    // "graph_update", "news_alert", "social_pulse"
	Payload interface{} `json:"payload"` // The actual data
}

type Hub struct {
	clients   map[*websocket.Conn]bool
	broadcast chan BroadcastMessage
	mu        sync.Mutex
	graph     *graph.Graph
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan BroadcastMessage),
	}
}

// SetGraph sets the graph reference for the hub
func (h *Hub) SetGraph(g *graph.Graph) {
	h.graph = g
}

func (h *Hub) Run() {
	for msg := range h.broadcast {
		h.mu.Lock()
		for client := range h.clients {
			err := client.WriteJSON(msg)
			if err != nil {
				logger.Warn(logger.StatusWarn, "WS Error: %v", err)
				client.Close()
				delete(h.clients, client)
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Broadcast(msgType string, payload interface{}) {
	h.broadcast <- BroadcastMessage{
		Type:    msgType,
		Payload: payload,
	}
}

// IncomingMessage represents a message from the client
type IncomingMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Warn(logger.StatusWarn, "Upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	// Send initial "connected" message
	conn.WriteJSON(BroadcastMessage{Type: "system", Payload: "Connected to Margraf Stream"})

	// Start listening for incoming messages from this client
	go h.handleClientMessages(conn)
}

// handleClientMessages listens for incoming messages from a client
func (h *Hub) handleClientMessages(conn *websocket.Conn) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	for {
		var msg IncomingMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Warn(logger.StatusWarn, "WS read error: %v", err)
			}
			break
		}

		// Handle different message types
		switch msg.Type {
		case "get_company_relations":
			h.handleGetCompanyRelations(conn, msg.Payload)
		case "get_companies_list":
			h.handleGetCompaniesList(conn)
		case "get_full_graph":
			h.handleGetFullGraph(conn)
		default:
			logger.Warn(logger.StatusWarn, "Unknown message type: %s", msg.Type)
		}
	}
}

// handleGetCompanyRelations handles requests for company relationship data
func (h *Hub) handleGetCompanyRelations(conn *websocket.Conn, payload map[string]interface{}) {
	if h.graph == nil {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: "Graph not initialized",
		})
		return
	}

	companyID, ok := payload["company_id"].(string)
	if !ok {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: "Invalid company_id",
		})
		return
	}

	relations, err := h.graph.GetCompanyRelations(companyID)
	if err != nil {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: err.Error(),
		})
		return
	}

	// Convert to JSON to send back
	relationsJSON, err := json.Marshal(relations)
	if err != nil {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: "Failed to encode relations",
		})
		return
	}

	conn.WriteJSON(BroadcastMessage{
		Type:    "company_relations",
		Payload: string(relationsJSON),
	})
}

// handleGetCompaniesList handles requests for the list of all companies
func (h *Hub) handleGetCompaniesList(conn *websocket.Conn) {
	if h.graph == nil {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: "Graph not initialized",
		})
		return
	}

	companies := make([]map[string]interface{}, 0)

	h.graph.NodesRange(func(n *graph.Node) {
		if n.Type == graph.NodeTypeCorporation {
			companies = append(companies, map[string]interface{}{
				"id":   n.ID,
				"name": n.Name,
			})
		}
	})

	companiesJSON, err := json.Marshal(companies)
	if err != nil {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: "Failed to encode companies",
		})
		return
	}

	conn.WriteJSON(BroadcastMessage{
		Type:    "companies_list",
		Payload: string(companiesJSON),
	})
}

// handleGetFullGraph handles requests for the complete graph data
func (h *Hub) handleGetFullGraph(conn *websocket.Conn) {
	if h.graph == nil {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: "Graph not initialized",
		})
		return
	}

	// Export the graph to JSON format
	graphJSON, err := h.graph.ToJSON()
	if err != nil {
		conn.WriteJSON(BroadcastMessage{
			Type:    "error",
			Payload: "Failed to export graph",
		})
		return
	}

	conn.WriteJSON(BroadcastMessage{
		Type:    "graph_update",
		Payload: graphJSON,
	})
}

func StartServer(h *Hub, port string) {
	http.HandleFunc("/ws", h.HandleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./public")))

	logger.Info(logger.StatusGlob, "WebSocket Server started on ws://localhost%s/ws", port)
	logger.Info(logger.StatusGlob, "Web Dashboard available at http://localhost%s", port)

	go func() {
		if err := http.ListenAndServe(port, nil); err != nil {
			logger.Error(logger.StatusErr, "ListenAndServe: %v", err)
		}
	}()
}
