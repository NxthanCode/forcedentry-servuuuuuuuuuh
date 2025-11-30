package main 

import (
	"net"
	"fmt"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}

	fmt.Printf("starting server: %s\n", port)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			continue
		}

		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()

	remoteaddr := conn.RemoteAddr().String()
	fmt.Printf("real client connected: %s\n", remoteaddr)

	buffer := make([]byte, 1024)
	for {
		_, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("client disconnected: %s\n", remoteaddr)
		}
	}
}
