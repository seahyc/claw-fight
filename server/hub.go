package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	pongWait     = 35 * time.Second
	writeWait    = 10 * time.Second
	maxMsgSize   = 8192
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	playerID string
	matchID  string
	send     chan []byte
	mu       sync.Mutex
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	spectators map[string]map[*Client]bool // matchID -> clients
	register   chan *Client
	unregister chan *Client
	byPlayer   map[string]*Client // playerID -> client
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		spectators: make(map[string]map[*Client]bool),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		byPlayer:   make(map[string]*Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			if client.playerID != "" {
				h.byPlayer[client.playerID] = client
			}
			h.mu.Unlock()
			log.Printf("Client connected: %s", client.playerID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.playerID != "" {
					delete(h.byPlayer, client.playerID)
				}
				close(client.send)
			}
			// Remove from spectators
			for mid, specs := range h.spectators {
				if _, ok := specs[client]; ok {
					delete(specs, client)
					if len(specs) == 0 {
						delete(h.spectators, mid)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("Client disconnected: %s", client.playerID)
		}
	}
}

func (h *Hub) GetClientByPlayer(playerID string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.byPlayer[playerID]
}

func (h *Hub) RegisterPlayer(client *Client, playerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	client.playerID = playerID
	h.byPlayer[playerID] = client
}

func (h *Hub) AddSpectator(matchID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.spectators[matchID] == nil {
		h.spectators[matchID] = make(map[*Client]bool)
	}
	h.spectators[matchID][client] = true
}

func (h *Hub) BroadcastToSpectators(matchID string, msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	specs := h.spectators[matchID]
	h.mu.RUnlock()

	for client := range specs {
		select {
		case client.send <- data:
		default:
			// Client buffer full, skip
		}
	}
}

func (c *Client) SendJSON(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
		log.Printf("Send buffer full for player %s", c.playerID)
	}
}

func (c *Client) ReadPump(handler func(*Client, []byte)) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMsgSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		handler(c, message)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
