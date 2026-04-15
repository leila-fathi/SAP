package game

import (
	"fmt"
	"log"
	"net"
	"strings"
)

type Game struct {
	CodeGen CodeGenerator
}

func NewGame(gen CodeGenerator) *Game {
	return &Game{CodeGen: gen}
}

func (g *Game) StartServer() {
	listener, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
	defer listener.Close()

	fmt.Println("Server started, waiting for a player...")

	// Accept only one player connection for now.
	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("Error accepting connection: %v", err)
	}
	defer conn.Close()

	fmt.Println("Player has connected.")

	for {
		// Generate a new secret for each round
		secret := g.CodeGen.GenerateSecretCode()
		writeToClient(conn, GenerateTimestampPrefix()+"New round! Guess the 4-digit secret code (or QUIT to leave).\n")

		won := false
		for !won {
			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil {
				log.Printf("Error reading from client: %v", err)
				return
			}

			guess := strings.TrimSpace(string(buffer[:n]))
			fmt.Printf("Received guess: %s\n", guess)

			if strings.EqualFold(guess, "QUIT") {
				writeToClient(conn, GenerateTimestampPrefix()+"Goodbye!\n")
				return
			}

			numGuess, err := ValidateGuess(guess)
			if err != nil {
				writeToClient(conn, GenerateTimestampPrefix()+err.Error()+"\n")
				continue
			}

			prefix := GenerateTimestampPrefix()
			if secret == numGuess {
				writeToClient(conn, prefix+"Congratulations! You guessed the correct number!\n")
				writeToClient(conn, prefix+"Type RESTART to play again or QUIT to disconnect.\n")
				won = true
			} else {
				feedback := GenerateFeedback(secret, numGuess)
				writeToClient(conn, prefix+"Try again! "+feedback+"\n")
			}
		}

		// Wait for restart or quit
		buffer := make([]byte, 256)
		n, err := conn.Read(buffer)
		if err != nil {
			log.Printf("Client disconnected: %v", err)
			return
		}
		cmd := strings.TrimSpace(string(buffer[:n]))
		if strings.EqualFold(cmd, "RESTART") {
			continue // new round with new secret
		}
		writeToClient(conn, GenerateTimestampPrefix()+"Shutting down. Goodbye!\n")
		return
	}
}

func writeToClient(conn net.Conn, s string) {
	_, err := conn.Write([]byte(s))
	if err != nil {
		log.Printf("Error writing to client: %v", err)
		return
	}
}
