package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type Player struct {
	ID        string
	Conn      net.Conn
	Position  string
	Rotation  string
	LastSeen  time.Time
	Username  string
}

type Lobby struct {
	Players map[string]*Player
	mutex   sync.RWMutex
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	serverAddress := fmt.Sprintf("%s:%s", host, port)
	fmt.Printf("Starting Unity Multiplayer Server on %s\n", serverAddress)

	lobby := &Lobby{
		Players: make(map[string]*Player),
	}

	listener, err := net.Listen("tcp", serverAddress)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listener.Close()

	fmt.Printf("Server listening on %s\n", serverAddress)

	go cleanupConnections(lobby)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Connection error:", err)
			continue
		}

		fmt.Printf("New connection from: %s\n", conn.RemoteAddr())
		go handleConnection(conn, lobby)
	}
}

func handleConnection(conn net.Conn, lobby *Lobby) {
	defer func() {
		conn.Close()
		fmt.Println("Connection closed")
	}()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReader(conn)
	firstMessage, err := reader.ReadString('\n')

	if err != nil {

		fmt.Println("Health check connection (timeout/error)")
		return
	}

	conn.SetReadDeadline(time.Time{})

	firstMessage = strings.TrimSpace(firstMessage)
	fmt.Printf("First message: %s\n", firstMessage)

	if strings.HasPrefix(firstMessage, "GET") || 
	   strings.HasPrefix(firstMessage, "HEAD") || 
	   strings.HasPrefix(firstMessage, "OPTIONS") ||
	   strings.Contains(firstMessage, "HTTP/1.1") {

		fmt.Println("Health check detected - sending OK and closing")
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
		return
	}

	playerID := fmt.Sprintf("player_%d", time.Now().UnixNano())

	player := &Player{
		ID:       playerID,
		Conn:     conn,
		Position: "0,0,0",
		Rotation: "0",
		LastSeen: time.Now(),
		Username: fmt.Sprintf("Player%d", time.Now().Unix()%1000),
	}

	lobby.processFirstMessage(playerID, firstMessage, player)

	lobby.mutex.Lock()
	lobby.Players[playerID] = player
	lobby.mutex.Unlock()

	fmt.Printf("Real Unity Player %s connected. Total players: %d\n", playerID, len(lobby.Players))

	welcomeMsg := fmt.Sprintf("server_info:Welcome! Your ID: %s", playerID)
	conn.Write([]byte(welcomeMsg + "\n"))

	lobby.broadcast(fmt.Sprintf("player_joined:%s,%s", playerID, player.Username), playerID)

	lobby.sendLobbyInfo(player)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		player.LastSeen = time.Now()

		if message == "" {
			continue
		}

		fmt.Printf("Received from %s: %s\n", playerID, message)
		lobby.processMessage(playerID, message)
	}

	lobby.mutex.Lock()
	delete(lobby.Players, playerID)
	lobby.mutex.Unlock()

	fmt.Printf("Player %s disconnected. Remaining players: %d\n", playerID, len(lobby.Players))

	lobby.broadcast(fmt.Sprintf("player_left:%s", playerID), "")
}

func (l *Lobby) processFirstMessage(playerID string, firstMessage string, player *Player) {
	parts := strings.SplitN(firstMessage, ":", 2)
	if len(parts) < 2 {
		return
	}

	command := parts[0]
	data := parts[1]

	switch command {
	case "join":
		player.Username = data
		fmt.Printf("Player %s set username: %s\n", playerID, data)
	case "ping":

	}
}

func (l *Lobby) processMessage(playerID string, message string) {
	parts := strings.SplitN(message, ":", 2)
	if len(parts) < 2 {
		return
	}

	command := parts[0]
	data := parts[1]

	l.mutex.RLock()
	player, exists := l.Players[playerID]
	l.mutex.RUnlock()

	if !exists {
		return
	}

	switch command {
	case "join":
		player.Username = data
		fmt.Printf("Player %s set username: %s\n", playerID, data)
		l.broadcast(fmt.Sprintf("lobby_info:Player %s joined the game", data), playerID)

	case "position":
		player.Position = data

		l.broadcast(fmt.Sprintf("position:%s,%s", playerID, data), playerID)

	case "rotation":
		player.Rotation = data

		l.broadcast(fmt.Sprintf("rotation:%s,%s", playerID, data), playerID)

	case "shoot":

		l.broadcast(fmt.Sprintf("shoot:%s,%s", playerID, data), playerID)
		fmt.Printf("Player %s shot: %s\n", playerID, data)

	case "chat":

		l.broadcast(fmt.Sprintf("chat:%s:%s", player.Username, data), playerID)
		fmt.Printf("Chat from %s: %s\n", player.Username, data)

	case "ping":

		player.Conn.Write([]byte("pong:\n"))
	}
}

func (l *Lobby) broadcast(message string, excludePlayerID string) {
    l.mutex.RLock()
    defer l.mutex.RUnlock()

    for id, player := range l.Players {
        if id != excludePlayerID {
            // PRÜFE ob Verbindung noch lebt
            if player.Conn == nil {
                continue
            }
            
            // Setze Schreib-Timeout
            player.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
            
            _, err := player.Conn.Write([]byte(message + "\n"))
            if err != nil {
                fmt.Printf("❌ Error sending to player %s: %v\n", id, err)
                // Verbindung schließen
                go func(pid string, p *Player) {
                    time.Sleep(100 * time.Millisecond)
                    p.Conn.Close()
                }(id, player)
            }
            
            // Timeout zurücksetzen
            player.Conn.SetWriteDeadline(time.Time{})
        }
    }
}

func (l *Lobby) sendLobbyInfo(player *Player) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	playerCount := len(l.Players)
	info := fmt.Sprintf("lobby_info:Connected! Players online: %d", playerCount)
	player.Conn.Write([]byte(info + "\n"))

	for id, otherPlayer := range l.Players {
		if id != player.ID {

			joinMsg := fmt.Sprintf("player_joined:%s,%s", id, otherPlayer.Username)
			player.Conn.Write([]byte(joinMsg + "\n"))

			if otherPlayer.Position != "" {
				posMsg := fmt.Sprintf("position:%s,%s", id, otherPlayer.Position)
				player.Conn.Write([]byte(posMsg + "\n"))
			}
		}
	}
}

func cleanupConnections(lobby *Lobby) {
	for {
		time.Sleep(30 * time.Second)

		lobby.mutex.Lock()
		now := time.Now()
		removedCount := 0

		for id, player := range lobby.Players {
			if now.Sub(player.LastSeen) > time.Minute {
				fmt.Printf("Removing inactive player: %s\n", id)
				player.Conn.Close()
				delete(lobby.Players, id)
				removedCount++
			}
		}

		if removedCount > 0 {
			fmt.Printf("Cleaned up %d inactive connections. Remaining: %d\n", removedCount, len(lobby.Players))
		}
		lobby.mutex.Unlock()
	}
}
