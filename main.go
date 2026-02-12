package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	System      = "system"
	UserCount   = "userCount"
	ChatMessage = "chatMessage"
	DateFormat  = "150405" // Hour, Minute, Second
)

type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type Client struct {
	conn *websocket.Conn
	send chan Message
	name string
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) run() {
	for {
		select {

		case client := <-h.register:
			h.clients[client] = true
			go h.broadcastUserCount()
			go h.broadcastSystem(client.name + " joined")

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				go h.broadcastUserCount()
				go h.broadcastSystem(client.name + " left")
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				client.send <- message
			}
		}
	}
}

func (h *Hub) broadcastUserCount() {
	msg := Message{
		Type: UserCount,
		Payload: map[string]int{
			"count": len(h.clients),
		},
	}
	h.broadcast <- msg
}

func (h *Hub) broadcastSystem(text string) {
	msg := Message{
		Type: System,
		Payload: map[string]string{
			"text": text,
		},
	}
	h.broadcast <- msg
}

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
			h.broadcast <- msg
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for msg := range c.send {
		if err := c.conn.WriteJSON(msg); err != nil {
			log.Println("Write error:", err)
			break
		}
	}
}

func serveChat(hub *Hub, c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	username := c.Query("username")
	if username == "" {
		var sb strings.Builder
		sb.WriteString("Anon_")
		sb.WriteString(time.Now().Format(DateFormat))
		username = sb.String()
	}

	client := &Client{
		conn: conn,
		send: make(chan Message, 256),
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

	// Serve the frontend
	r.StaticFile("/", "./static/index.html")

	r.GET("/chat", func(c *gin.Context) {
		serveChat(hub, c)
	})

	log.Println("Hallway running on :8080")
	r.Run(":8080")
}
