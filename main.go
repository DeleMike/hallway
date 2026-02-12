package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Every WebSocket frame carries a "type" field that tells the receiver
// how to interpret the "payload" field.
const (
	System      = "system"      // Server-generated announcements (e.g. "alice joined")
	UserCount   = "userCount"   // Current connected-user count
	ChatMessage = "chatMessage" // Ordinary user chat message
	DateFormat  = "150405"      // Go time layout – HHmmss, used for anon names
	MaxHistory  = 100           // Maximum chat messages kept in memory
)

// Message is the universal JSON envelope for every frame sent and received.
//
//	Client - Server:  { "type": "chatMessage", "payload": { "username": "…", "message": "…" } }
//	Server - Client:  same shape, plus system / userCount frames
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// Client represents a single connected WebSocket session.
type Client struct {
	conn *websocket.Conn // live WebSocket connection
	send chan Message    // outbound queue (buffered 256); closed by hub on disconnect
	name string          // resolved username (prompt value or Anon_HHmmss)
}

// Hub is the single-goroutine event loop that owns all shared state.
type Hub struct {
	clients    map[*Client]bool // set of active connections
	broadcast  chan Message     // messages to send-out to all clients
	register   chan *Client     // new connection arrivals
	unregister chan *Client     // disconnecting clients
	history    []Message        // set of last previous chat messages limited by MaxHistory
}

var upgrader = websocket.Upgrader{
	// Allow connections from any origin for now. If prod, please change.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// newHub allocates all channels and the client set.
// history is nil until the first message arrives (lazy alloc).
func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// run is the heart of the server. It must be started in its own goroutine.
func (h *Hub) run() {
	for {
		select {

		// ── New client connected ───
		case client := <-h.register:
			h.clients[client] = true

			// Replay chat history so the new joiner sees recent context.
			for _, msg := range h.history {
				client.send <- msg
			}

			go h.broadcastUserCount()
			go h.broadcastSystem(client.name + " joined")

		// ── Client disconnected ───
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send) // signals writePump to exit
				go h.broadcastUserCount()
				go h.broadcastSystem(client.name + " left")
			}

		// ── send-out an inbound message to everyone ──
		case message := <-h.broadcast:
			// Only persist chat messages; ignore system/count frames.
			if message.Type == ChatMessage {
				h.history = append(h.history, message)
				if len(h.history) > MaxHistory {
					// we are showing only latest 100.
					h.history = h.history[len(h.history)-MaxHistory:]
				}
			}

			for client := range h.clients {
				client.send <- message
			}
		}
	}
}

// broadcastUserCount sends a userCount frame to all connected clients.
// Must be called from a goroutine (not directly inside hub.run's select).
func (h *Hub) broadcastUserCount() {
	h.broadcast <- Message{
		Type:    UserCount,
		Payload: map[string]int{"count": len(h.clients)},
	}
}

// broadcastSystem sends a server-generated system announcement to all clients.
// Must be called from a goroutine (not directly inside hub.run's select).
func (h *Hub) broadcastSystem(text string) {
	h.broadcast <- Message{
		Type:    System,
		Payload: map[string]string{"text": text},
	}
}

// readPump reads JSON frames from the WebSocket and forwards chat messages to
// the hub. It exits (and triggers cleanup) on any read error, which covers:
//   - Normal browser tab close / WebSocket close frame
//   - Network drop
//   - Postman disconnecting
func (c *Client) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()

	for {
		var msg Message
		if err := c.conn.ReadJSON(&msg); err != nil {
			break
		}

		if msg.Type == ChatMessage {
			// Normalise the payload: replace whatever username the client sent
			// with the server-resolved name (handles anon users and prevents
			// clients from impersonating others by spoofing the username field).
			if payload, ok := msg.Payload.(map[string]any); ok {
				payload["username"] = c.name
				msg.Payload = payload
			}

			h.broadcast <- msg
		}
	}
}

// writePump drains the client's send channel and serialises each Message to
// the WebSocket as JSON. It exits when the channel is closed by hub.run,
// which happens during unregister.
func (c *Client) writePump() {
	defer c.conn.Close()

	for msg := range c.send {
		if err := c.conn.WriteJSON(msg); err != nil {
			log.Println("Write error:", err)
			break
		}
	}
}

// serveChat upgrades an HTTP GET /chat request to a WebSocket, resolves the
// username, and wires up the client to the hub.
func serveChat(hub *Hub, c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	username := strings.TrimSpace(c.Query("username"))
	if username == "" {
		var sb strings.Builder
		sb.WriteString("Anon_")
		sb.WriteString(time.Now().Format(DateFormat))
		username = sb.String()
	}

	client := &Client{
		conn: conn,
		send: make(chan Message, 256), // buffered to absorb bursts without stalling hub.run ; incase those guys wanna spam
		name: username,
	}

	hub.register <- client

	go client.writePump()
	go client.readPump(hub)
}

func main() {
	hub := newHub()
	go hub.run()

	r := gin.Default()

	r.GET("/chat", func(c *gin.Context) {
		serveChat(hub, c)
	})

	log.Println("Hallway Server running on :8080")
	r.Run(":8080")
}
