package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Define WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// Define struct for managing connections
type connection struct {
	ws       *websocket.Conn
	send     chan []byte
	username string
}

// Define struct for managing rooms
type room struct {
	clients map[*connection]bool
	join    chan *connection
	leave   chan *connection
}

// Define struct for user information
type User struct {
	Username string `json:"username" bson:"username"`
}

// Define struct for recording data
type Recording struct {
	Username string    `json:"username" bson:"username"`
	Audio    []byte    `json:"audio" bson:"audio"`
	Time     time.Time `json:"time" bson:"time"`
}

var (
	globalRoom          *room
	roomMutex           sync.Mutex
	client              *mongo.Client
	userCollection      *mongo.Collection
	recordingCollection *mongo.Collection
)

func newRoom() *room {
	return &room{
		clients: make(map[*connection]bool),
		join:    make(chan *connection),
		leave:   make(chan *connection),
	}
}

func (r *room) run() {
	for {
		select {
		case client := <-r.join:
			r.clients[client] = true
		case client := <-r.leave:
			delete(r.clients, client)
			close(client.send)
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	username := r.URL.Query().Get("username")

	// Create new connection
	conn := &connection{
		ws:       ws,
		send:     make(chan []byte, 256),
		username: username,
	}

	// Add connection to the global room
	globalRoom.join <- conn

	// Start goroutine to handle outgoing messages
	go conn.writePump()

	// Start goroutine to handle incoming messages
	go conn.readPump()

	// Update connected users
	sendUserUpdate()
}


func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")

	// Store user information in MongoDB
	_, err := userCollection.InsertOne(context.Background(), bson.M{"username": username})
	if err != nil {
		log.Printf("Error storing user information: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Call handleWebSocket to handle WebSocket connection
	handleWebSocket(w, r)
}

func handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")

	// Remove user information from MongoDB
	_, err := userCollection.DeleteOne(context.Background(), bson.M{"username": username})
	if err != nil {
		log.Printf("Error removing user information: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Call handleWebSocket to handle WebSocket connection
	handleWebSocket(w, r)
}

func sendUserUpdate() {
	roomMutex.Lock()
	defer roomMutex.Unlock()

	// Retrieve user information from MongoDB
	cursor, err := userCollection.Find(context.Background(), bson.M{})
	if err != nil {
		log.Printf("Error retrieving user information: %v", err)
		return
	}
	defer cursor.Close(context.Background())

	var users []User
	err = cursor.All(context.Background(), &users)
	if err != nil {
		log.Printf("Error decoding user information: %v", err)
		return
	}

	// Extract usernames from user information
	var usernames []string
	for _, user := range users {
		usernames = append(usernames, user.Username)
	}

	userUpdate := struct {
		Type  string   `json:"type"`
		Users []string `json:"users"`
	}{
		Type:  "userUpdate",
		Users: usernames,
	}

	userUpdateJSON, err := json.Marshal(userUpdate)
	if err != nil {
		log.Printf("Error marshalling userUpdate: %v", err)
		return
	}

	// Broadcast user update to all clients
	for client := range globalRoom.clients {
		select {
		case client.send <- userUpdateJSON:
		default:
			delete(globalRoom.clients, client)
			close(client.send)
		}
	}
}

func (c *connection) readPump() {
	for {
		messageType, message, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		if messageType == websocket.BinaryMessage {
			// Broadcast audio message to other clients
			roomMutex.Lock()
			for client := range globalRoom.clients {
				if client != c {
					select {
					case client.send <- message:
					default:
						delete(globalRoom.clients, client)
						close(client.send)
					}
				}
			}
			roomMutex.Unlock()
		} else {
			// Handle other message types (if any)
		}
	}
}

func (c *connection) writePump() {
	for message := range c.send {
		err := c.ws.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
}

func saveRecording(username string, data []byte) error {
	// Insert audio data into MongoDB collection
	_, err := recordingCollection.InsertOne(context.Background(), bson.M{"username": username, "audio": data, "time": time.Now()})
	if err != nil {
		return err
	}
	return nil
}

func initMongoDB() error {
	clientOptions := options.Client().ApplyURI("mongodb+srv://root:root@cluster0.cjrjjex.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0")
	var err error
	client, err = mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		return err
	}
	err = client.Ping(context.Background(), nil)
	if err != nil {
		return err
	}
	userCollection = client.Database("chat").Collection("users") // Collection for user information
	recordingCollection = client.Database("chat").Collection("recordings")
	return nil
}

func main() {
	// Initialize MongoDB
	err := initMongoDB()
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Initialize the global room
	globalRoom = newRoom()

	// Start the room
	go globalRoom.run()

	// Define HTTP handlers
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/join", handleJoinRoom)
	http.HandleFunc("/leave", handleLeaveRoom)

	// Start HTTP server
	server := &http.Server{Addr: ":8080"}

	go func() {
		log.Fatal(server.ListenAndServe())
	}()

	// Handle graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	log.Println("Shutting down server...")

	// Close the room
	roomMutex.Lock()
	close(globalRoom.join)
	for client := range globalRoom.clients {
		close(client.send)
		delete(globalRoom.clients, client)
	}
	roomMutex.Unlock()

	// Wait for a while to allow clients to disconnect gracefully
	time.Sleep(2 * time.Second)

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	// Disconnect from MongoDB
	err = client.Disconnect(context.Background())
	if err != nil {
		log.Printf("Error disconnecting from MongoDB: %v", err)
	}
}
