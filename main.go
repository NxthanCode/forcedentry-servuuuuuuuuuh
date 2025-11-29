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
	IsReal    bool 
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
	fmt.Printf("🚀 Starting Unity Multiplayer Server on %s\n", serverAddress)

	lobby := &Lobby{
		Players: make(map[string]*Player),
	}

	listener, err := net.Listen("tcp", serverAddress)
	if err != nil {
		fmt.Println("❌ Error starting server:", err)
		return
	}
	defer listener.Close()

	fmt.Printf("✅ Server listening on %s\n", serverAddress)

	go cleanupConnections(lobby)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("❌ Connection error:", err)
			continue
		}

		go handleConnection(conn, lobby)
	}
}

func handleConnection(conn net.Conn, lobby *Lobby) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	reader := bufio.NewReader(conn)
	firstMessage, err := reader.ReadString('\n')

	if err != nil {

		fmt.Printf("🏥 Health check (timeout): %s\n", remoteAddr)
		return
	}

	conn.SetReadDeadline(time.Time{})

	firstMessage = strings.TrimSpace(firstMessage)
	fmt.Printf("🔍 First message from %s: %s\n", remoteAddr, firstMessage)

	if strings.HasPrefix(firstMessage, "GET") || 
	   strings.HasPrefix(firstMessage, "HEAD") || 
	   strings.HasPrefix(firstMessage, "OPTIONS") ||
	   strings.Contains(firstMessage, "HTTP/1.1") ||
	   strings.Contains(firstMessage, "Host: forcedentry.onrender.com") {

		fmt.Printf("🏥 HTTP Health check: %s\n", remoteAddr)

		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
		return
	}

	if !isUnityClientMessage(firstMessage) {
		fmt.Printf("❓ Unknown protocol from %s: %s\n", remoteAddr, firstMessage)
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
		IsReal:   true, 
	}

	lobby.processFirstMessage(playerID, firstMessage, player)

	lobby.mutex.Lock()
	lobby.Players[playerID] = player
	lobby.mutex.Unlock()

	fmt.Printf("✅ REAL Unity Player %s connected. Total players: %d\n", playerID, len(lobby.Players))

	welcomeMsg := fmt.Sprintf("server_info:Welcome to Forced Entry! Your ID: %s", playerID)
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

		fmt.Printf("🎮 From %s: %s\n", playerID, message)
		lobby.processMessage(playerID, message)
	}

	lobby.mutex.Lock()
	delete(lobby.Players, playerID)
	lobby.mutex.Unlock()

	fmt.Printf("👋 Player %s disconnected. Remaining players: %d\n", playerID, len(lobby.Players))

	lobby.broadcast(fmt.Sprintf("player_left:%s", playerID), "")
}

func isUnityClientMessage(message string) bool {
	validPrefixes := []string{
		"join:",
		"position:",
		"rotation:", 
		"shoot:",
		"chat:",
		"ping:",
	}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
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
		fmt.Printf("🎯 Player %s joined with name: %s\n", playerID, data)
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

	if !exists || !player.IsReal {
		return
	}

	switch command {
	case "join":
		player.Username = data
		fmt.Printf("🎯 Player %s set username: %s\n", playerID, data)
		l.broadcast(fmt.Sprintf("lobby_info:Player %s joined the game", data), playerID)

	case "position":
		player.Position = data

		l.broadcast(fmt.Sprintf("position:%s,%s", playerID, data), playerID)

	case "rotation":
		player.Rotation = data

		l.broadcast(fmt.Sprintf("rotation:%s,%s", playerID, data), playerID)

	case "shoot":

		l.broadcast(fmt.Sprintf("shoot:%s,%s", playerID, data), playerID)
		fmt.Printf("🔫 Player %s shot: %s\n", playerID, data)

	case "chat":

		l.broadcast(fmt.Sprintf("chat:%s:%s", player.Username, data), playerID)
		fmt.Printf("💬 Chat from %s: %s\n", player.Username, data)
		fmt.Printf("💬 Chat from %s: %s\n", player.Username, data)
		

	case "ping":

		player.Conn.Write([]byte("pong:\n"))
	}
}

func (l *Lobby) broadcast(message string, excludePlayerID string) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	for id, player := range l.Players {
		if id != excludePlayerID && player.IsReal {
			_, err := player.Conn.Write([]byte(message + "\n"))
			if err != nil {
				fmt.Printf("❌ Error sending to player %s: %v\n", id, err)
			}
		}
	}
}

func (l *Lobby) sendLobbyInfo(player *Player) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	playerCount := len(l.Players)
	info := fmt.Sprintf("lobby_info:Connected to Forced Entry! Players online: %d", playerCount)
	player.Conn.Write([]byte(info + "\n"))

	for id, otherPlayer := range l.Players {
		if id != player.ID && otherPlayer.IsReal {

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
			if now.Sub(player.LastSeen) > time.Minute && player.IsReal {
				fmt.Printf("🧹 Removing inactive player: %s\n", id)
				player.Conn.Close()
				delete(lobby.Players, id)
				removedCount++
			}
		}

		if removedCount > 0 {
			fmt.Printf("🧹 Cleaned up %d inactive connections. Remaining: %d\n", removedCount, len(lobby.Players))
		}
		lobby.mutex.Unlock()
	}
}
