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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type connection struct {
	ws       *websocket.Conn
	send     chan []byte
	username string
}

type room struct {
	clients map[*connection]bool
	join    chan *connection
	leave   chan *connection
}

type User struct {
	Username string `json:"username" bson:"username"`
}

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
	username := r.URL.Query().Get("username")

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	conn := &connection{
		ws:       ws,
		send:     make(chan []byte, 256),
		username: username,
	}

	globalRoom.join <- conn

	go conn.writePump()

	go conn.readPump()

	sendUserUpdate()
}

func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")

	_, err := userCollection.InsertOne(context.Background(), bson.M{"username": username})
	if err != nil {
		log.Printf("Error storing user information: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	handleWebSocket(w, r)
}

func handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")

	_, err := userCollection.DeleteOne(context.Background(), bson.M{"username": username})
	if err != nil {
		log.Printf("Error removing user information: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	handleWebSocket(w, r)
}

func sendUserUpdate() {
	roomMutex.Lock()
	defer roomMutex.Unlock()

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
	userCollection = client.Database("chat").Collection("users")
	recordingCollection = client.Database("chat").Collection("recordings")
	return nil
}

func main() {
	err := initMongoDB()
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	globalRoom = newRoom()

	go globalRoom.run()

	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/join", handleJoinRoom)
	http.HandleFunc("/leave", handleLeaveRoom)

	server := &http.Server{Addr: ":8080"}

	go func() {
		log.Fatal(server.ListenAndServe())
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	log.Println("Shutting down server...")

	roomMutex.Lock()
	close(globalRoom.join)
	for client := range globalRoom.clients {
		close(client.send)
		delete(globalRoom.clients, client)
	}
	roomMutex.Unlock()

	time.Sleep(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	err = client.Disconnect(context.Background())
	if err != nil {
		log.Printf("Error disconnecting from MongoDB: %v", err)
	}
}
