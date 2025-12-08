package main

import (
	"ccs_interview/game"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <mode>")
	}

	mode := os.Args[1]

	randGen := &game.RandomCodeGenerator{}
	g := game.NewGame(randGen)

	switch mode {
	case "server":
		g.StartServer() // Start the server, handling one player for now
	case "client":
		err := game.StartClient("localhost:8080") // Client connects to server
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("Invalid mode. Use 'server' or 'client'.")
	}
}
