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

	mu      sync.Mutex // protects players slice
	players []*Player
}

// Player holds connection + id info.
type Player struct {
	ID   int
	Conn net.Conn
}

// Analytics contains lightweight server-side stats.
type Analytics struct {
	mu         sync.Mutex
	TotalGames int
	GamesWon   int
	GamesLost  int
	GuessTotal int
	GuessCount map[int]int // frequency of each guessed number
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
	a.GuessTotal++
}

func (a *Analytics) RecordGame(win bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.TotalGames++
	if win {
		a.GamesWon++
	} else {
		a.GamesLost++
	}
}

func (a *Analytics) Summary() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return fmt.Sprintf("Games: %d | Won: %d | Lost: %d | Total Guesses: %d",
		a.TotalGames, a.GamesWon, a.GamesLost, a.GuessTotal)
}

// NewMultiplayerGame creates a new multiplayer game instance.
func NewMultiplayerGame(codeGen CodeGenerator, maxPlayers int, timeout time.Duration) *MultiplayerGame {
	return &MultiplayerGame{
		CodeGen:    codeGen,
		MaxPlayers: maxPlayers,
		Timeout:    timeout,
		Analytics:  NewAnalytics(),
		players:    make([]*Player, 0, maxPlayers),
	}
}

// Start listens for player connections and runs the game loop.
func (mg *MultiplayerGame) Start(address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("error starting multiplayer server: %w", err)
	}
	mg.listener = ln
	defer ln.Close()

	log.Printf("Multiplayer server listening on %s — waiting for %d players...", address, mg.MaxPlayers)

	// Accept connections until we have MaxPlayers
	for mg.playerCount() < mg.MaxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		player := &Player{
			ID:   mg.playerCount() + 1,
			Conn: conn,
		}
		mg.addPlayer(player)
		log.Printf("Player %d connected from %s", player.ID, conn.RemoteAddr().String())
		connectedPlayers := mg.playerCount()
		remainingPlayers := mg.MaxPlayers - connectedPlayers
		mg.broadcastExcept(mg.snapshotPlayers(), player, fmt.Sprintf("%sPlayer %d joined the game. %d/%d connected, waiting for %d more.\n", GenerateTimestampPrefix(), player.ID, connectedPlayers, mg.MaxPlayers, remainingPlayers))
		mg.writeToPlayer(player, fmt.Sprintf("%sWelcome Player %d! Waiting for other players...\n", GenerateTimestampPrefix(), player.ID))
	}

	mg.broadcast(fmt.Sprintf("%sGame started with %d players! Player 1 begins.\n", GenerateTimestampPrefix(), mg.MaxPlayers))

	// Main loop allows multiple rounds while enough players remain connected.
	for {
		if mg.playerCount() == 0 {
			log.Printf("No players remain. Shutting down multiplayer server.")
			return nil
		}

		secret := mg.CodeGen.GenerateSecretCode()
		log.Printf("Generated secret for session: %d", secret)

		winner := mg.runGameSession(secret)

		if mg.playerCount() < mg.minimumPlayers() {
			mg.broadcast(fmt.Sprintf("%sNot enough players remain to continue. Ending session.\n", GenerateTimestampPrefix()))
			mg.closeAllPlayers()
			log.Printf("Not enough players remain. Shutting down multiplayer server.")
			return nil
		}

		mg.Analytics.RecordGame(winner != nil)

		if winner != nil {
			mg.broadcast(fmt.Sprintf("%sPlayer %d broke the code %d! Round over.\n", GenerateTimestampPrefix(), winner.ID, secret))
		} else {
			mg.broadcast(fmt.Sprintf("%sGame ended without a winner. The code was %d.\n", GenerateTimestampPrefix(), secret))
		}

		log.Printf("Analytics: %s", mg.Analytics.Summary())
		mg.broadcast(fmt.Sprintf("%sStarting a new round...\n", GenerateTimestampPrefix()))
	}
}

// runGameSession runs a single game round with turn rotation.
// Returns the winning Player or nil.
func (mg *MultiplayerGame) runGameSession(secret int) *Player {
	mg.mu.Lock()
	activePlayers := make([]*Player, len(mg.players))
	copy(activePlayers, mg.players)
	mg.mu.Unlock()

	current := 0

	for {
		activePlayers = mg.filterActivePlayers(activePlayers)
		if len(activePlayers) < mg.minimumPlayers() {
			return nil
		}

		p := activePlayers[current%len(activePlayers)]
		mg.writeToPlayer(p, fmt.Sprintf("%sYour turn Player %d. Enter a 4-digit guess:\n", GenerateTimestampPrefix(), p.ID))
		mg.broadcastExcept(activePlayers, p, fmt.Sprintf("%sPlayer %d is guessing...\n", GenerateTimestampPrefix(), p.ID))

		// Set turn deadline
		if mg.Timeout > 0 {
			_ = p.Conn.SetReadDeadline(time.Now().Add(mg.Timeout))
		} else {
			_ = p.Conn.SetReadDeadline(time.Time{})
		}

		buf := make([]byte, 1024)
		n, err := p.Conn.Read(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d missed their turn (timeout).\n", GenerateTimestampPrefix(), p.ID))
				current = (current + 1) % len(activePlayers)
				continue
			}
			// Disconnect — remove player
			log.Printf("Error reading from player %d: %v. Removing player.", p.ID, err)
			_ = p.Conn.Close()
			activePlayers = removePlayer(activePlayers, p)
			mg.removePlayerFromRoster(p)
			if len(activePlayers) < mg.minimumPlayers() {
				return nil
			}
			current = current % len(activePlayers)
			continue
		}

		input := strings.TrimSpace(string(buf[:n]))

		// Mid-game commands
		if strings.EqualFold(input, "RESTART") {
			mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d requested restart. Ending this round.\n", GenerateTimestampPrefix(), p.ID))
			return nil
		}
		if strings.EqualFold(input, "QUIT") {
			mg.writeToPlayer(p, GenerateTimestampPrefix()+"Goodbye!\n")
			_ = p.Conn.Close()
			activePlayers = removePlayer(activePlayers, p)
			mg.removePlayerFromRoster(p)
			if len(activePlayers) < mg.minimumPlayers() {
				return nil
			}
			current = current % len(activePlayers)
			continue
		}

		// Validate guess
		guessNum, err := ValidateGuess(input)
		if err != nil {
			mg.writeToPlayer(p, GenerateTimestampPrefix()+err.Error()+"\n")
			current = (current + 1) % len(activePlayers)
			continue
		}

		mg.Analytics.RecordGuess(guessNum)

		// Check guess
		if guessNum == secret {
			mg.writeToPlayer(p, GenerateTimestampPrefix()+"Congratulations! You guessed the correct number!\n")
			mg.broadcastExcept(activePlayers, p, fmt.Sprintf("%sPlayer %d guessed the correct number!\n", GenerateTimestampPrefix(), p.ID))
			return p
		}

		// Wrong guess — send feedback with hints
		feedback := GenerateFeedback(secret, guessNum)
		mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d guessed %d -> %s\n", GenerateTimestampPrefix(), p.ID, guessNum, feedback))
		current = (current + 1) % len(activePlayers)
	}
}

// --- Thread-safe player management ---

func (mg *MultiplayerGame) addPlayer(p *Player) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	mg.players = append(mg.players, p)
}

func (mg *MultiplayerGame) playerCount() int {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	return len(mg.players)
}

func (mg *MultiplayerGame) snapshotPlayers() []*Player {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	snapshot := make([]*Player, len(mg.players))
	copy(snapshot, mg.players)
	return snapshot
}

func (mg *MultiplayerGame) minimumPlayers() int {
	if mg.MaxPlayers <= 1 {
		return 1
	}
	return 2
}

func (mg *MultiplayerGame) closeAllPlayers() {
	for _, p := range mg.snapshotPlayers() {
		_ = p.Conn.Close()
	}
}

func (mg *MultiplayerGame) removePlayerFromRoster(p *Player) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	mg.players = removePlayer(mg.players, p)
}

func (mg *MultiplayerGame) isPlayerActive(p *Player) bool {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	for _, pl := range mg.players {
		if pl == p {
			return true
		}
	}
	return false
}

func (mg *MultiplayerGame) filterActivePlayers(players []*Player) []*Player {
	filtered := make([]*Player, 0, len(players))
	for _, p := range players {
		if mg.isPlayerActive(p) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func (mg *MultiplayerGame) disconnectPlayer(p *Player, reason string) bool {
	mg.mu.Lock()
	found := false
	for _, pl := range mg.players {
		if pl == p {
			found = true
			break
		}
	}
	if found {
		mg.players = removePlayer(mg.players, p)
	}
	mg.mu.Unlock()

	if !found {
		return false
	}

	_ = p.Conn.Close()
	log.Printf("Player %d disconnected: %s", p.ID, reason)
	mg.broadcast(fmt.Sprintf("%sPlayer %d disconnected: %s\n", GenerateTimestampPrefix(), p.ID, reason))
	return true
}

// removePlayer returns a new slice without the given player.
func removePlayer(players []*Player, p *Player) []*Player {
	result := make([]*Player, 0, len(players))
	for _, pl := range players {
		if pl != p {
			result = append(result, pl)
		}
	}
	return result
}

// --- Messaging helpers ---

func (mg *MultiplayerGame) writeToPlayer(p *Player, s string) {
	_, err := p.Conn.Write([]byte(s))
	if err != nil {
		log.Printf("Error writing to player %d: %v", p.ID, err)
		mg.disconnectPlayer(p, "connection lost")
	}
}

// broadcast sends to all players in mg.players (thread-safe).
func (mg *MultiplayerGame) broadcast(s string) {
	mg.mu.Lock()
	snapshot := make([]*Player, len(mg.players))
	copy(snapshot, mg.players)
	mg.mu.Unlock()

	for _, p := range snapshot {
		_, err := p.Conn.Write([]byte(s))
		if err != nil {
			log.Printf("Error writing to player %d: %v", p.ID, err)
			mg.disconnectPlayer(p, "connection lost")
		}
	}
}

// broadcastAll sends to a specific list of active players.
func (mg *MultiplayerGame) broadcastAll(players []*Player, s string) {
	for _, p := range players {
		_, err := p.Conn.Write([]byte(s))
		if err != nil {
			log.Printf("Error writing to player %d: %v", p.ID, err)
			mg.disconnectPlayer(p, "connection lost")
		}
	}
}

// broadcastExcept sends to all active players except one.
func (mg *MultiplayerGame) broadcastExcept(players []*Player, except *Player, s string) {
	for _, p := range players {
		if p == except {
			continue
		}
		_, err := p.Conn.Write([]byte(s))
		if err != nil {
			log.Printf("Error writing to player %d: %v", p.ID, err)
			mg.disconnectPlayer(p, "connection lost")
		}
	}
}
