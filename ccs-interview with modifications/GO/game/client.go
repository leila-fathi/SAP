package game

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func StartClient(address string) error {
	// Connect to the server
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("error connecting to server: %v", err)
	}
	defer conn.Close()

	fmt.Println("Welcome to the Code Breaker Game! Enter a code between 1000 and 9999.")

	// Create a reader to capture input from stdin
	reader := bufio.NewReader(os.Stdin)

	// Start the game loop
	for {
		// Prompt the user to enter their guess
		fmt.Print("Enter your guess (secret code) or 'exit' to quit: ")
		guess, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("error reading input: %v", err)
		}
		guess = guess[:len(guess)-1] // Remove the trailing newline character

		// Allow the user to quit the game
		if guess == "exit" {
			fmt.Println("Exiting the game.")
			break
		}

		// Send the guess to the server
		_, err = conn.Write([]byte(guess))
		if err != nil {
			return fmt.Errorf("error sending message to server: %v", err)
		}

		// Wait for a response from the server
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			return fmt.Errorf("error reading from server: %v", err)
		}

		// Print the server's response
		serverResponse := string(buffer[:n])
		fmt.Println("Server response:", serverResponse)

		// If the guess was correct, end the game
		if serverResponse == "Congratulations! You guessed the correct number!" {
			fmt.Println("You won the game! Exiting...")
			break
		}

		time.Sleep(1 * time.Second) // Simulate a delay before the next round
	}

	return nil
}
