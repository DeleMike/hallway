package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	System      = "system"
	UserCount   = "userCount"
	ChatMessage = "chatMessage"
)

// a typical message coming from the client
type Message struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// to help us change from http(s) to ws(s)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var clients = make(map[*websocket.Conn]string)
var mutex = &sync.Mutex{}

func chatHandler(c *gin.Context) {
	// upgrade connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("We could not upgrade connection: ", err)
		return
	}

	// collect user name
	username := c.Query("username")
	if username == "" {
		username = "Anon"
	}

	mutex.Lock()
	clients[conn] = username
	mutex.Unlock()

	// update clients statuses
	broadcastUserCount()
	broadcastSystem(username + " joined")

	// will run when a client disconnects
	defer func() {
		mutex.Lock()
		delete(clients, conn)
		mutex.Unlock()

		broadcastUserCount()
		broadcastSystem(username + " left")

		conn.Close()
	}()

	// a pipe that waits for any incoming message
	for {
		var msg Message

		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		if msg.Type == ChatMessage {
			broadcast(msg)
		}
	}

}

// broadcast sends message to pipe for all connected clients to receive
func broadcast(message Message) {
	mutex.Lock()
	defer mutex.Unlock()

	for client := range clients {
		client.WriteJSON(message)
	}
}

// broadcastUserCount counts how many connected clients in the room
func broadcastUserCount() {
	msg := Message{
		Type: UserCount,
		Payload: map[string]int{
			"count": len(clients),
		},
	}

	broadcast(msg)
}

// broadcastSystem is used to notify who joined the room
func broadcastSystem(text string) {
	msg := Message{
		Type: System,
		Payload: map[string]string{
			"text": text,
		},
	}

	broadcast(msg)
}

func main() {
	r := gin.Default()
	r.GET("/chat", chatHandler)
	r.Run(":8080")

}
