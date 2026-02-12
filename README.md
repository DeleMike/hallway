# Hallway: Real-time WebSocket Chat Server

**Hallway** is a real-time multi-user chat server built with Go. It utilises a centralised Hub architecture to manage state across thousands of concurrent connections without the need for complex mutex locking.

---

## 🚀 Tech Stack

| Property | Value |
| --- | --- |
| **Language** | Go 1.24+ |
| **Web Framework** | [Gin](https://github.com/gin-gonic/gin) (HTTP Routing) |
| **WebSocket** | [Gorilla WebSocket](https://github.com/gorilla/websocket) v1.5.1 |
| **Architecture** | Hub / Client goroutine model (CSP) |
| **History** | Rolling in-memory buffer (Last 100 messages) |
| **Frontend** | Single Page Application (LINK-WILL-BE-ATTACHED) |

---

## 1. Overview

Hallway enables real-time JSON message exchange. A single **Hub** goroutine acts as the "Single Source of Truth," owning the client list and message history.

### Key Features:

* **Websocket Architecture Design:** Real time communication!
* **Immediate Context:** New clients receive a replay of the last 100 messages upon joining.
* **Authoritative Usernames:** Usernames are resolved server-side via `?username=` query params. If blank, the server generates an `Anon_HHmmss` timestamp-based name. The server "stamps" this name on every message to prevent spoofing.

---

## 2. Message Protocol

All communication uses a universal JSON envelope:

```go
type Message struct {
    Type    string `json:"type"`    // chatMessage | system | userCount
    Payload any    `json:"payload"` // Shape varies by Type
}

```

### Server-to-Client Frames:

* **ChatMessage:** `{ "type": "chatMessage", "payload": { "username": "alice", "message": "Hello!" } }`
* **System:** `{ "type": "system", "payload": { "text": "alice joined" } }`
* **UserCount:** `{ "type": "userCount", "payload": { "count": 3 } }`

---

## 3. Architecture & Data Types

### 3.1 The Hub

The central switchboard. history uses **lazy allocation** (it stays `nil` until the first message arrives to save memory).

| Field | Description |
| --- | --- |
| `clients` | Map of active connections. |
| `broadcast` | Inbound channel for fan-out messages. |
| `register` | Channel for new connection arrivals. |
| `history` | Slice of up to 100 rolling messages. |

### 3.2 The Client

Represents a live session. Every client spawns two dedicated goroutines:

1. **readPump:** Reads from WebSocket, enforces authoritative usernames, and pushes to Hub.
2. **writePump:** Drains the buffered `send` channel (cap 256) and writes to the browser.

---

## 4. Concurrency Model (CSP)

Hallway follows the mantra: *"Do not communicate by sharing memory; instead, share memory by communicating."*

### Goroutine Flow:

1. **`main()`** → Starts `go hub.run()`.
2. **`serveChat()`** → Upgrades HTTP to WS, creates Client, and starts:
* `go client.writePump()`
* `go client.readPump()`



### Data Flow Path:

`readPump` ➔ `hub.broadcast` ➔ `hub.run` ➔ `client.send` ➔ `writePump`

---

## 5. Implementation Details

### History Trimming

To keep memory usage constant, the history buffer is trimmed once it exceeds `MaxHistory`:

```go
h.history = append(h.history, message)
if len(h.history) > MaxHistory {
    h.history = h.history[len(h.history)-MaxHistory:]
}

```

### Deadlock Prevention

Inside `hub.run()`, system broadcasts and user count updates are launched via `go` statements. This ensures the Hub doesn't block itself trying to send a message to its own broadcast channel while the loop is still busy.

---

## 6. Known Limitations To Me

* **Persistence:** History lives in RAM; data is lost on restart.
* **Authentication:** No password protection; usernames are assigned via URL parameters.
* **Scaling:** Single-room only. Multi-room support would require a map of Hubs.
* **Security:** `CheckOrigin` is set to `true` for development; it should be restricted for production.

---

## 🛠️ Getting Started

1. Clone the repository.
2. Run the server:
```bash
go run main.go

```
3. Attach the server to your frontend or test in Postman

---
