package game

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// MultiplayerGame handles multiple players in a turn-based session.
type MultiplayerGame struct {
	CodeGen    CodeGenerator
	MaxPlayers int
	Timeout    time.Duration // per-turn timeout
	Analytics  *Analytics

	listener net.Listener
	players  []*Player
}

// Player holds connection + id info.
type Player struct {
	ID   int
	Conn net.Conn
	// Optionally add name, score, etc.
}

// Analytics contains lightweight server-side stats.
type Analytics struct {
	mu         sync.Mutex
	TotalGames int
	GamesWon   int
	GuessCount map[int]int
}

func NewAnalytics() *Analytics {
	return &Analytics{
		GuessCount: make(map[int]int),
	}
}

func (a *Analytics) RecordGuess(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.GuessCount[n]++
}

func (a *Analytics) RecordGame(win bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.TotalGames++
	if win {
		a.GamesWon++
	}
}

// NewMultiplayerGame creates a new multiplayer game instance
func NewMultiplayerGame(codeGen CodeGenerator, maxPlayers int, timeout time.Duration) *MultiplayerGame {
	return &MultiplayerGame{
		CodeGen:    codeGen,
		MaxPlayers: maxPlayers,
		Timeout:    timeout,
		Analytics:  NewAnalytics(),
		players:    []*Player{},
	}
}

// Start listens for player connections and starts the multiplayer loop.
// address example "0.0.0.0:8080"
func (mg *MultiplayerGame) Start(address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("error starting multiplayer server: %w", err)
	}
	mg.listener = ln
	defer ln.Close()

	log.Printf("Multiplayer server listening on %s — waiting for %d players...", address, mg.MaxPlayers)

	// Accept connections until we have MaxPlayers
	for len(mg.players) < mg.MaxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		player := &Player{
			ID:   len(mg.players) + 1,
			Conn: conn,
		}
		mg.players = append(mg.players, player)
		log.Printf("Player %d connected from %s", player.ID, conn.RemoteAddr().String())
		// Notify player of their id
		mg.writeToPlayer(player, fmt.Sprintf("%sWelcome Player %d! Waiting for other players...\n", GenerateTimestampPrefix(), player.ID))
	}

	// Notify all players that game is starting
	mg.broadcast(fmt.Sprintf("%sGame started with %d players! Player 1 begins.\n", GenerateTimestampPrefix(), mg.MaxPlayers))

	// Main loop allows multiple games with restart
	for {
		secret := mg.CodeGen.GenerateSecretCode()
		log.Printf("Generated secret for session: %d", secret)
		// track guesses until win
		winner := mg.runGameSession(secret)
		// record analytics
		mg.Analytics.RecordGame(winner != nil)

		if winner != nil {
			mg.broadcast(fmt.Sprintf("%sPlayer %d broke the code %d! Game Over.\n", GenerateTimestampPrefix(), winner.ID, secret))
		} else {
			mg.broadcast(fmt.Sprintf("%sGame ended without a winner.\n", GenerateTimestampPrefix()))
		}

		// Ask for restart from all players
		mg.broadcast(fmt.Sprintf("%sType RESTART to play again or QUIT to disconnect. Waiting for all players...\n", GenerateTimestampPrefix()))
		restarts := make([]bool, mg.MaxPlayers)
		for i, p := range mg.players {
			// set deadline for restart waiting, generous: 120s
			p.Conn.SetReadDeadline(time.Now().Add(120 * time.Second))
			buf := make([]byte, 256)
			n, err := p.Conn.Read(buf)
			if err != nil {
				// if a player disconnected or timed out, treat as QUIT
				log.Printf("player %d failed to send restart: %v", p.ID, err)
				restarts[i] = false
				continue
			}
			cmd := strings.TrimSpace(string(buf[:n]))
			if strings.EqualFold(cmd, "RESTART") {
				restarts[i] = true
			} else if strings.EqualFold(cmd, "QUIT") {
				restarts[i] = false
				// close connection
				mg.writeToPlayer(p, GenerateTimestampPrefix()+"Goodbye!\n")
				p.Conn.Close()
			} else {
				// treat anything else as NO
				restarts[i] = false
			}
		}

		// If all players want restart, loop again; else end session and close connections.
		allRestart := true
		for _, r := range restarts {
			if !r {
				allRestart = false
				break
			}
		}
		if !allRestart {
			mg.broadcast(GenerateTimestampPrefix() + "One or more players declined to restart. Shutting down session.\n")
			// close all player connections
			for _, p := range mg.players {
				_ = p.Conn.Close()
			}
			return nil
		}
		// else continue and generate new secret — loop continues
	}
}

// runGameSession runs a single game round until someone guesses correctly or all disconnect.
// returns the winning Player pointer or nil if no winner.
func (mg *MultiplayerGame) runGameSession(secret int) *Player {
	current := 0 // player index (0-based)
	activePlayers := mg.players
	for {
		// Check if any player disconnected
		tempPlayers := []*Player{}
		for _, p := range activePlayers {
			if p == nil {
				continue
			}
			tempPlayers = append(tempPlayers, p)
		}
		if len(tempPlayers) == 0 {
			return nil
		}
		activePlayers = tempPlayers

		p := activePlayers[current%len(activePlayers)]
		mg.writeToPlayer(p, fmt.Sprintf("%sYour turn Player %d. Enter a 4-digit guess:\n", GenerateTimestampPrefix(), p.ID))
		mg.broadcastExcept(p, fmt.Sprintf("%sPlayer %d is guessing...\n", GenerateTimestampPrefix(), p.ID))

		// Set deadline for this player's turn
		if mg.Timeout > 0 {
			_ = p.Conn.SetReadDeadline(time.Now().Add(mg.Timeout))
		} else {
			// no timeout
			_ = p.Conn.SetReadDeadline(time.Time{})
		}

		buf := make([]byte, 1024)
		n, err := p.Conn.Read(buf)
		if err != nil {
			// check for timeout or disconnect: treat as forfeited turn
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				mg.broadcast(fmt.Sprintf("%sPlayer %d missed their turn (timeout).\n", GenerateTimestampPrefix(), p.ID))
				current = (current + 1) % len(activePlayers)
				continue
			}
			// other read error — remove player
			log.Printf("Error reading from player %d: %v. Removing player.", p.ID, err)
			_ = p.Conn.Close()
			// remove from activePlayers
			newPlayers := []*Player{}
			for _, pl := range activePlayers {
				if pl != p {
					newPlayers = append(newPlayers, pl)
				}
			}
			activePlayers = newPlayers
			if len(activePlayers) == 0 {
				return nil
			}
			current = current % len(activePlayers)
			continue
		}

		// got input
		input := strings.TrimSpace(string(buf[:n]))
		// allow player to type RESTART or QUIT mid-game
		if strings.EqualFold(input, "RESTART") {
			// treat as player wants to restart immediately — break to outer restart flow
			mg.broadcast(fmt.Sprintf("%sPlayer %d requested restart. Ending this round.\n", GenerateTimestampPrefix(), p.ID))
			return nil
		}
		if strings.EqualFold(input, "QUIT") {
			mg.writeToPlayer(p, GenerateTimestampPrefix()+"Goodbye!\n")
			_ = p.Conn.Close()
			// remove player
			newPlayers := []*Player{}
			for _, pl := range activePlayers {
				if pl != p {
					newPlayers = append(newPlayers, pl)
				}
			}
			activePlayers = newPlayers
			if len(activePlayers) == 0 {
				return nil
			}
			current = current % len(activePlayers)
			continue
		}

		// Validate guess
		guessNum, err := ValidateGuess(input)
		if err != nil {
			mg.writeToPlayer(p, GenerateTimestampPrefix()+err.Error()+"\n")
			// do not advance turn on invalid input? we'll advance — keeps flow moving
			current = (current + 1) % len(activePlayers)
			continue
		}

		// record analytics
		mg.Analytics.RecordGuess(guessNum)

		// Check guess against secret
		if guessNum == secret {
			// winner
			mg.writeToPlayer(p, GenerateTimestampPrefix()+"Congratulations! You guessed the correct number!\n")
			// notify others
			mg.broadcastExcept(p, fmt.Sprintf("%sPlayer %d guessed the correct number %d!\n", GenerateTimestampPrefix(), p.ID, secret))
			return p
		} else {
			// broadcast result and next turn
			mg.broadcast(fmt.Sprintf("%sPlayer %d guessed %d → Try again!\n", GenerateTimestampPrefix(), p.ID, guessNum))
			current = (current + 1) % len(activePlayers)
			continue
		}
	}
}

// helper to write to a single player
func (mg *MultiplayerGame) writeToPlayer(p *Player, s string) {
	_, err := p.Conn.Write([]byte(s))
	if err != nil {
		log.Printf("Error writing to player %d: %v", p.ID, err)
	}
}

// broadcast to all players
func (mg *MultiplayerGame) broadcast(s string) {
	for _, p := range mg.players {
		if p == nil {
			continue
		}
		_, _ = p.Conn.Write([]byte(s))
	}
}

// broadcast except to one player
func (mg *MultiplayerGame) broadcastExcept(except *Player, s string) {
	for _, p := range mg.players {
		if p == nil {
			continue
		}
		if p == except {
			continue
		}
		_, _ = p.Conn.Write([]byte(s))
	}
}
