package server

import (
	"fmt"
	"log"
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
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan BroadcastMessage),
	}
}

func (h *Hub) Run() {
	for msg := range h.broadcast {
		h.mu.Lock()
		for client := range h.clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("WS Error: %v", err)
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

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	// Send initial "connected" message
	conn.WriteJSON(BroadcastMessage{Type: "system", Payload: "Connected to Margraf Stream"})
}

func StartServer(h *Hub, port string) {
	http.HandleFunc("/ws", h.HandleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./public")))

	fmt.Printf("🌐 WebSocket Server started on ws://localhost%s/ws\n", port)
	fmt.Printf("🌍 Web Dashboard available at http://localhost%s\n", port)
	
	go func() {
		if err := http.ListenAndServe(port, nil); err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()
}
