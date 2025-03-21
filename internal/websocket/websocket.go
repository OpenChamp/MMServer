package websocket

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
)

// Logger for the WebSocket server
var log = logrus.New()

// Client represents a connected websocket client
type Client struct {
	id       string
	conn     *websocket.Conn
	send     chan []byte
	manager  *ClientManager
	dbPool   *pgxpool.Pool
	username string

	// Authentication fields
	authenticated bool
	authToken     string
}

type ClientManager struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
}

// Create a new global client manager
var manager = ClientManager{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan []byte),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

// WebSocket upgrader to handle HTTP to WebSocket upgrade
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

// initializeLogger sets up the logging system
func initializeLogger() error {
	// Create logs directory if it doesn't exist
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %v", err)
	}

	// Create log file with timestamp in filename
	timestamp := time.Now().Format("2006-01-02")
	logFilePath := filepath.Join(logsDir, fmt.Sprintf("websocket-%s.log", timestamp))
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	// Configure logrus
	log.SetOutput(logFile)
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Set log level
	log.SetLevel(logrus.InfoLevel)

	log.Info("WebSocket logging system initialized")
	fmt.Printf("WebSocket logs will be written to %s\n", logFilePath)
	return nil
}

// StartWebSocketServer initializes the WebSocket server
func StartWebSocketServer(port int, dbPool *pgxpool.Pool) {
	// Default Port
	if port == 0 {
		port = 8081
	}
	// Logging System
	if err := initializeLogger(); err != nil {
		fmt.Printf("Failed to initialize WebSocket logger: %v\n", err)
		os.Exit(1)
	}
	// Start the client manager in a separate goroutine for performance
	go manager.run()

	// Upgrader
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocketConnection(w, r, dbPool)
	})

	// Start the WebSocket server
	log.Info(fmt.Sprintf("Starting WebSocket server on :%d...", port))
	fmt.Printf("Starting WebSocket server on :%d...\n", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.WithFields(logrus.Fields{
			"error": err,
		}).Fatal("Error starting WebSocket server")
	}
}

// Run the client manager to handle client registration, unregistration, and broadcasts
func (manager *ClientManager) run() {
	for {
		select {
		case client := <-manager.register:
			// Register new client
			manager.mutex.Lock()
			manager.clients[client] = true
			manager.mutex.Unlock()

			log.WithFields(logrus.Fields{
				"client_id": client.id,
			}).Info("Client connected")

			// Send a welcome message to the new client
			client.send <- []byte(`Welcome to the Server!`)

		case client := <-manager.unregister:
			// Unregister client
			if _, ok := manager.clients[client]; ok {
				manager.mutex.Lock()
				delete(manager.clients, client)
				manager.mutex.Unlock()
				close(client.send)

				log.WithFields(logrus.Fields{
					"client_id": client.id,
				}).Info("Client disconnected")
			}

		case message := <-manager.broadcast:
			// Broadcast message to all clients
			manager.mutex.RLock()
			for client := range manager.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(manager.clients, client)

					log.WithFields(logrus.Fields{
						"client_id": client.id,
						"reason":    "send buffer full",
					}).Warn("Client forcibly disconnected")
				}
			}
			manager.mutex.RUnlock()
		}
	}
}

// handleWebSocketConnection upgrades the HTTP request to a WebSocket connection
func handleWebSocketConnection(w http.ResponseWriter, r *http.Request, dbpool *pgxpool.Pool) {
	// Upgrade the incoming HTTP request to a WebSocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithFields(logrus.Fields{
			"error":       err,
			"remote_addr": r.RemoteAddr,
		}).Error("Upgrade error")
		return
	}

	// Create a new client
	client := &Client{
		id:      r.RemoteAddr,
		conn:    conn,
		send:    make(chan []byte, 256),
		manager: &manager,
		dbPool:  dbpool,
	}

	// Register the client with the manager
	client.manager.register <- client

	// Start goroutines for reading and writing
	go client.readPump()
	go client.writePump()
}

// readPump handles incoming messages from the client
func (c *Client) readPump() {
	defer func() {
		c.manager.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.WithFields(logrus.Fields{
					"client_id": c.id,
					"error":     err,
				}).Error("Error reading message")
			}
			break
		}

		// Process the message
		log.WithFields(logrus.Fields{
			"client_id": c.id,
			"message":   string(message),
		}).Info("Received message")

		// Handle the packet (you can implement the handlePacket function)
		handlePacket(c, string(message), log)
	}
}

// writePump sends messages to the client
func (c *Client) writePump() {
	defer c.conn.Close()

	for {
		message, ok := <-c.send
		if !ok {
			// Channel was closed
			c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.WithFields(logrus.Fields{
				"client_id": c.id,
				"error":     err,
			}).Error("Error writing message")
			return
		}
	}
}

// BroadcastMessage sends a message to all connected clients
func BroadcastMessage(message []byte) {
	manager.broadcast <- message
}

// GetConnectedClientsCount returns the number of currently connected clients
func GetConnectedClientsCount() int {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	return len(manager.clients)
}

// SetLogLevel allows changing the log level at runtime
func SetLogLevel(level string) {
	switch level {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}
	log.WithFields(logrus.Fields{
		"level": level,
	}).Info("Log level changed")
}
