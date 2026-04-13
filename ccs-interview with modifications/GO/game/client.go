package game

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func StartClient(address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("error connecting to server: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected to server. Waiting for game to start...")

	// Start a goroutine to continuously read server messages
	done := make(chan struct{})
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
			fmt.Print(msg)

			if strings.Contains(msg, "Congratulations! You guessed the correct number!") {
				fmt.Println("You won!")
			}
		}
	}()

	// Read user input and send to server
	reader := bufio.NewReader(os.Stdin)
	for {
		select {
		case <-done:
			return nil
		default:
		}

		guess, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("error reading input: %v", err)
		}
		guess = strings.TrimSpace(guess)

		if strings.EqualFold(guess, "exit") {
			fmt.Println("Exiting the game.")
			return nil
		}

		_, err = conn.Write([]byte(guess))
		if err != nil {
			return fmt.Errorf("error sending message to server: %v", err)
		}

		// Small delay to let the server response arrive before next prompt
		time.Sleep(100 * time.Millisecond)
	}
}
