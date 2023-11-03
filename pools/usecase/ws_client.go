package usecase

import (
	"context"
	"errors"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	reconnectMaxDelay = 120 * time.Second
	pingPeriod        = 30 * time.Second // How often to send pings
	writeWait         = 10 * time.Second // Time allowed to write a message to the peer
)

// UpdatePriceRequest represents the JSON payload for the update_price method.
type UpdatePriceRequest struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Account string `json:"account"`
		Price   int64  `json:"price"`
		Conf    uint64 `json:"conf"`
		Status  string `json:"status"`
	} `json:"params"`
	ID int `json:"id"`
}

// WebSocketClient is responsible for maintaining a WebSocket connection.
type WebSocketClient struct {
	url            string
	conn           *websocket.Conn
	reconnectDelay time.Duration
	doneChan       chan struct{}
	mutex          sync.Mutex
}

// NewWebSocketClient creates a new WebSocket client.
func NewWebSocketClient(url string) *WebSocketClient {
	return &WebSocketClient{
		url:            url,
		reconnectDelay: 1 * time.Second,
		doneChan:       make(chan struct{}),
	}
}

// connect establishes the WebSocket connection.
func (c *WebSocketClient) connect() error {
	u, err := url.Parse(c.url)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

// keepAlive sends pings at regular intervals and waits for pongs, closing the connection if it times out.
func (c *WebSocketClient) keepAlive(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	c.conn.SetPongHandler(func(string) error { return nil }) // Set handler for pong messages.

	for {
		select {
		case <-ticker.C:
			c.mutex.Lock()
			if c.conn != nil {
				c.conn.SetWriteDeadline(time.Now().Add(writeWait)) // Set deadline for write.
				if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Println("write:", err)
					c.conn.Close()
					c.conn = nil
				}
			}
			c.mutex.Unlock()
		case <-ctx.Done():
			c.mutex.Lock()
			if c.conn != nil {
				c.conn.Close()
			}
			c.mutex.Unlock()
			return
		case <-c.doneChan:
			return
		}
	}
}

// ensureConnection attempts to establish a websocket connection and reconnects if necessary.
func (c *WebSocketClient) ensureConnection(ctx context.Context) {
	for {
		select {
		case <-c.doneChan:
			return // Exit if the done channel is closed.
		default:
			c.mutex.Lock()
			if c.conn == nil {
				err := c.connect()
				if err != nil {
					log.Printf("Error connecting to WebSocket: %v; retrying in %v", err, c.reconnectDelay)
					c.mutex.Unlock()
					time.Sleep(c.reconnectDelay)
					// Exponential backoff
					c.reconnectDelay = min(reconnectMaxDelay, 2*c.reconnectDelay)
					continue
				}
				// Connection established, reset reconnect delay and start keep-alive pings.
				c.reconnectDelay = 1 * time.Second
				go c.keepAlive(ctx)
			}
			c.mutex.Unlock()
			time.Sleep(1 * time.Second)
		}
	}
}

// SendUpdatePrice sends an update_price request to the WebSocket server.
func (c *WebSocketClient) SendUpdatePrice(account string, price int64, conf uint64, status string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn == nil {
		return errors.New("WebSocket connection not established")
	}

	req := UpdatePriceRequest{
		Jsonrpc: "2.0",
		Method:  "update_price",
		ID:      1,
	}
	req.Params.Account = account
	req.Params.Price = price
	req.Params.Conf = conf
	req.Params.Status = status

	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	err := c.conn.WriteJSON(req)
	if err != nil {
		log.Printf("Error sending update price: %v", err)
		c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// Run starts the WebSocket client and maintains the connection.
func (c *WebSocketClient) Run(ctx context.Context) {
	go c.ensureConnection(ctx)
}

// Close cleanly closes the WebSocket connection.
func (c *WebSocketClient) Close() {
	close(c.doneChan) // Signal all goroutines to stop.
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
		c.conn = nil
	}
}

// Helper function to get the minimum of two durations.
func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
