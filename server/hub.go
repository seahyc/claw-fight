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

	// Event queue
	events      []map[string]any
	eventCh     chan struct{}
	eventMu     sync.Mutex
	listenCancel chan struct{} // cancel active listen
}

// DisconnectHandler is called when a player's WebSocket disconnects.
// It allows the match manager to start a grace period instead of immediate forfeit.
type DisconnectHandler func(playerID string)

type Hub struct {
	mu                sync.RWMutex
	clients           map[*Client]bool
	spectators        map[string]map[*Client]bool // matchID -> clients
	register          chan *Client
	unregister        chan *Client
	byPlayer          map[string]*Client // playerID -> client
	disconnectHandler DisconnectHandler
	restSinks         map[string]*RESTSink
}

func NewHub() *Hub {
	h := &Hub{
		clients:    make(map[*Client]bool),
		spectators: make(map[string]map[*Client]bool),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		byPlayer:   make(map[string]*Client),
		restSinks:  make(map[string]*RESTSink),
	}
	go h.cleanupRESTSinks()
	return h
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
			playerID := client.playerID
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if playerID != "" {
					delete(h.byPlayer, playerID)
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
			handler := h.disconnectHandler
			h.mu.Unlock()
			log.Printf("Client disconnected: %s", playerID)
			// Notify match manager so it can start a grace period
			if playerID != "" && handler != nil {
				handler(playerID)
			}
		}
	}
}

func (h *Hub) GetClientByPlayer(playerID string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.byPlayer[playerID]
}

// RESTSink queues events for players using HTTP long-poll instead of WebSocket.
type RESTSink struct {
	mu       sync.Mutex
	events   []map[string]any
	eventCh  chan struct{}
	lastUsed time.Time
}

func newRESTSink() *RESTSink {
	return &RESTSink{
		eventCh:  make(chan struct{}, 1),
		lastUsed: time.Now(),
	}
}

func (rs *RESTSink) enqueue(event map[string]any) {
	rs.mu.Lock()
	rs.events = append(rs.events, event)
	rs.lastUsed = time.Now()
	rs.mu.Unlock()
	select {
	case rs.eventCh <- struct{}{}:
	default:
	}
}

func (rs *RESTSink) drain(matchID string, types []string) []map[string]any {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.lastUsed = time.Now()
	if len(rs.events) == 0 {
		return nil
	}
	var result, remaining []map[string]any
	for _, ev := range rs.events {
		match := true
		if matchID != "" {
			if mid, ok := ev["match_id"].(string); ok && mid != matchID {
				match = false
			}
		}
		if match && len(types) > 0 {
			evType, _ := ev["type"].(string)
			found := false
			for _, t := range types {
				if t == evType {
					found = true
					break
				}
			}
			if !found {
				match = false
			}
		}
		if match {
			result = append(result, ev)
		} else {
			remaining = append(remaining, ev)
		}
	}
	rs.events = remaining
	return result
}

// GetOrCreateRESTSink returns the REST sink for a player, creating one if needed.
func (h *Hub) GetOrCreateRESTSink(playerID string) *RESTSink {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.restSinks[playerID] == nil {
		h.restSinks[playerID] = newRESTSink()
	}
	return h.restSinks[playerID]
}

// DeliverEvent sends an event to a player via WebSocket (preferred) or REST sink.
// If the player has no WebSocket connection, the event is buffered in a REST sink
// so it can be retrieved via the poll endpoint later.
func (h *Hub) DeliverEvent(playerID string, event map[string]any) {
	h.mu.RLock()
	c := h.byPlayer[playerID]
	sink := h.restSinks[playerID]
	h.mu.RUnlock()

	if c != nil {
		c.QueueEvent(event)
		return
	}
	// No WebSocket — buffer in REST sink (create if needed)
	if sink == nil {
		sink = h.GetOrCreateRESTSink(playerID)
	}
	sink.enqueue(event)
}

func (h *Hub) cleanupRESTSinks() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.Lock()
		for id, sink := range h.restSinks {
			if time.Since(sink.lastUsed) > 10*time.Minute {
				delete(h.restSinks, id)
			}
		}
		h.mu.Unlock()
	}
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

// QueueEvent appends an event to the client's event queue and signals the channel.
func (c *Client) QueueEvent(event map[string]any) {
	c.eventMu.Lock()
	c.events = append(c.events, event)
	c.eventMu.Unlock()

	// Non-blocking signal
	select {
	case c.eventCh <- struct{}{}:
	default:
	}
}

// DrainEvents returns all queued events, optionally filtering by matchID and event types.
func (c *Client) DrainEvents(matchID string, types []string) []map[string]any {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()

	if len(c.events) == 0 {
		return nil
	}

	var result []map[string]any
	var remaining []map[string]any

	for _, ev := range c.events {
		match := true
		if matchID != "" {
			if mid, ok := ev["match_id"].(string); ok && mid != matchID {
				match = false
			}
		}
		if match && len(types) > 0 {
			evType, _ := ev["type"].(string)
			found := false
			for _, t := range types {
				if t == evType {
					found = true
					break
				}
			}
			if !found {
				match = false
			}
		}

		if match {
			result = append(result, ev)
		} else {
			remaining = append(remaining, ev)
		}
	}

	c.events = remaining
	return result
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
