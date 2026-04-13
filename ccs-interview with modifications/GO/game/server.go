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

	conn, err := listener.Accept()
	if err != nil {
		log.Fatalf("Error accepting connection: %v", err)
	}
	defer conn.Close()

	fmt.Println("Player has connected.")

	// Generate the secret code ONCE per session
	secret := g.CodeGen.GenerateSecretCode()
	log.Printf("Secret code generated: %d", secret)

	for {
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			log.Printf("Error reading from client: %v", err)
			return
		}

		guess := strings.TrimSpace(string(buffer[:n]))
		fmt.Printf("Received guess: %s\n", guess)

		numGuess, err := ValidateGuess(guess)
		prefix := GenerateTimestampPrefix()

		if err != nil {
			log.Printf("Error validating guess: %v", err)
			writeToClient(conn, prefix+err.Error()+"\n")
		} else if numGuess == secret {
			writeToClient(conn, prefix+"Congratulations! You guessed the correct number!\n")
			// Start a new round with a new secret
			secret = g.CodeGen.GenerateSecretCode()
			log.Printf("New secret code generated: %d", secret)
		} else {
			feedback := GenerateFeedback(secret, numGuess)
			writeToClient(conn, prefix+feedback+"\n")
		}
	}
}

func writeToClient(conn net.Conn, s string) {
	_, err := conn.Write([]byte(s))
	if err != nil {
		log.Printf("Error writing to client: %v", err)
		return
	}
}
