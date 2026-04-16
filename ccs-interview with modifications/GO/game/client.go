package game

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func StartClient(address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("error connecting to server: %v", err)
	}

	fmt.Println("Connected to server. Waiting for game to start...")
	fmt.Println("Type 'exit' or 'quit' at any time to leave the game.")

	done := make(chan struct{})

	// Goroutine: continuously read and print server messages
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				fmt.Println("\nDisconnected from server.")
				close(done)
				return
			}
			msg := string(buf[:n])
			fmt.Print("\n" + msg)

			if strings.Contains(msg, "Shutting down") || strings.Contains(msg, "Goodbye") {
				close(done)
				return
			}
		}
	}()

	// Goroutine: read stdin
	reader := bufio.NewReader(os.Stdin)
	inputCh := make(chan string)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line != "" {
				inputCh <- line
			}
		}
	}()

	for {
		select {
		case <-done:
			conn.Close()
			return nil
		case line := <-inputCh:
			if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") {
				_, _ = conn.Write([]byte("QUIT"))
				fmt.Println("Exiting the game.")
				conn.Close()
				return nil
			}
			_, err := conn.Write([]byte(line))
			if err != nil {
				conn.Close()
				return fmt.Errorf("error sending to server: %v", err)
			}
		}
	}
}
