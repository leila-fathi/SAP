package main

import (
	"ccs_interview/game"
	"flag"
	"log"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <mode>")
	}

	//mode := os.Args[1]

	//randGen := &game.RandomCodeGenerator{}
	//g := game.NewGame(randGen)

	/*	switch mode {
		case "server":
			g.StartServer() // Start the server, handling one player for now
		case "client":
			err := game.StartClient("localhost:8080") // Client connects to server
			if err != nil {
				log.Fatal(err)
			}
		default:
			log.Fatal("Invalid mode. Use 'server' or 'client'.")
		}*/

	/////////////////

	mode := flag.String("mode", "", "mode: server / client / mserver")
	players := flag.Int("players", 2, "number of players for multiplayer server")
	timeout := flag.Int("timeout", 15, "turn timeout in seconds (0 = no timeout)")
	addr := flag.String("addr", "localhost:8080", "server address")
	flag.Parse()

	switch *mode {
	case "server":
		// single-player server (legacy) — using existing Game
		randGen := &game.RandomCodeGenerator{}
		g := game.NewGame(randGen)
		g.StartServer()
	case "client":
		err := game.StartClient(*addr)
		if err != nil {
			log.Fatal(err)
		}
	case "mserver":
		// multiplayer server
		randGen := &game.RandomCodeGenerator{}
		t := time.Duration(*timeout) * time.Second
		mg := game.NewMultiplayerGame(randGen, *players, t)
		if err := mg.Start(*addr); err != nil {
			log.Fatalf("multiplayer server stopped: %v", err)
		}
	default:
		log.Fatalf("invalid or empty mode. Use -mode=server|-mode=client|-mode=mserver")
	}

}
