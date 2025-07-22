package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

type Room struct {
	clients map[*Client]bool
	lock    sync.Mutex
	name    string
}

var rooms = make(map[string]*Room)
var roomsLock sync.Mutex

func getRoom(id string) *Room {
	roomsLock.Lock()
	defer roomsLock.Unlock()
	room, exists := rooms[id]
	if !exists {
		room = &Room{
			clients: make(map[*Client]bool),
			name:    id,
		}
		rooms[id] = room
	}
	return room
}

func (room *Room) broadcast(message []byte) {
	room.lock.Lock()
	defer room.lock.Unlock()
	for client := range room.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(room.clients, client)
		}
	}
}

func (c *Client) readPump(room *Room) {
	defer func() {
		room.lock.Lock()
		delete(room.clients, c)
		room.lock.Unlock()
		c.conn.Close()
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		room.broadcast(msg)
		fmt.Printf("[%s]: %s\n", room.name, msg)
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		err := c.conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			break
		}
	}
}

func websocketHandler(c echo.Context) error {
	roomId := c.Param("roomId")
	room := getRoom(roomId)

	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	room.lock.Lock()
	room.clients[client] = true
	room.lock.Unlock()

	go client.writePump()
	client.readPump(room)

	return nil
}

func main() {
	e := echo.New()
	e.GET("/:roomId", websocketHandler)
	fmt.Println("Listening on :8080")
	e.Logger.Fatal(e.Start(":8080"))
}
