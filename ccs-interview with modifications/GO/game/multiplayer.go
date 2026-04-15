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
	Timeout    time.Duration
	Analytics  *Analytics

	listener net.Listener

	mu      sync.Mutex // protects players slice
	players []*Player
}

// PlayerMessage carries input from a player's reader goroutine.
type PlayerMessage struct {
	Player *Player
	Text   string
	Err    error
}

// Player holds connection + id info.
type Player struct {
	ID   int
	Conn net.Conn
	In   chan PlayerMessage // receives all input from this player
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

// startPlayerReader launches a goroutine that reads from the player's connection
// and sends every message to the player's In channel.
func startPlayerReader(p *Player) {
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := p.Conn.Read(buf)
			if err != nil {
				p.In <- PlayerMessage{Player: p, Err: err}
				return
			}
			text := strings.TrimSpace(string(buf[:n]))
			if text != "" {
				p.In <- PlayerMessage{Player: p, Text: text}
			}
		}
	}()
}

// Start listens for player connections and starts the multiplayer loop.
func (mg *MultiplayerGame) Start(address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("error starting multiplayer server: %w", err)
	}
	mg.listener = ln
	defer ln.Close()

	log.Printf("Multiplayer server listening on %s — waiting for %d players...", address, mg.MaxPlayers)

	// Accept connections using a channel so we can also monitor existing players
	newConns := make(chan net.Conn)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			newConns <- conn
		}
	}()

	for len(mg.getActivePlayers()) < mg.MaxPlayers {
		select {
		case conn := <-newConns:
			player := &Player{
				ID:   len(mg.players) + 1,
				Conn: conn,
				In:   make(chan PlayerMessage, 10),
			}
			mg.players = append(mg.players, player)
			startPlayerReader(player)
			log.Printf("Player %d connected from %s", player.ID, conn.RemoteAddr().String())
			mg.writeToPlayer(player, fmt.Sprintf("%sWelcome Player %d! Waiting for other players...\nType QUIT at any time to leave the game.\n", GenerateTimestampPrefix(), player.ID))

		default:
			// Check if any waiting player sent QUIT or disconnected
			for _, p := range mg.players {
				select {
				case msg := <-p.In:
					if msg.Err != nil || strings.EqualFold(msg.Text, "QUIT") || strings.EqualFold(msg.Text, "EXIT") {
						log.Printf("Player %d left during lobby.", p.ID)
						mg.writeToPlayer(p, GenerateTimestampPrefix()+"Goodbye!\n")
						p.Conn.Close()
						p.Conn = nil
						// Notify remaining players in the lobby
						for _, other := range mg.getActivePlayers() {
							mg.writeToPlayer(other, fmt.Sprintf("%sPlayer %d has left. Waiting for more players...\n", GenerateTimestampPrefix(), p.ID))
						}
					}
				default:
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	activePlayers := mg.getActivePlayers()
	mg.broadcastAll(activePlayers, fmt.Sprintf("%sGame started with %d players! Player 1 begins.\n", GenerateTimestampPrefix(), len(activePlayers)))

	// Main loop allows multiple rounds while enough players remain connected.
	for {
		if len(mg.getActivePlayers()) == 0 {
			log.Printf("No players remain. Shutting down multiplayer server.")
			return nil
		}

		secret := mg.CodeGen.GenerateSecretCode()
		log.Printf("Generated secret for session: %d", secret)

		winner := mg.runGameSession(secret)
		mg.Analytics.RecordGame(winner != nil)

		// Check if any players remain
		activePlayers := mg.getActivePlayers()
		if len(activePlayers) == 0 {
			log.Println("All players disconnected.")
			mg.LogAnalytics()
			return nil
		}

		if winner != nil {
			mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d broke the code %d! Game Over.\n", GenerateTimestampPrefix(), winner.ID, secret))
		} else {
			mg.broadcastAll(activePlayers, fmt.Sprintf("%sGame ended without a winner.\n", GenerateTimestampPrefix()))
		}

		mg.broadcastAll(activePlayers, fmt.Sprintf("%sType RESTART to play again or QUIT to leave.\n", GenerateTimestampPrefix()))

		// Collect restart votes from remaining players
		restartVotes := map[int]bool{}
		remaining := len(activePlayers)
		timeout := time.After(120 * time.Second)

	voteLoop:
		for len(restartVotes) < remaining {
			select {
			case <-timeout:
				break voteLoop
			default:
			}

			// Read from any active player
			msg := mg.readFromAny(activePlayers, 2*time.Second)
			if msg == nil {
				continue
			}
			if msg.Err != nil {
				restartVotes[msg.Player.ID] = false
				continue
			}
			if strings.EqualFold(msg.Text, "RESTART") {
				restartVotes[msg.Player.ID] = true
				mg.writeToPlayer(msg.Player, GenerateTimestampPrefix()+"Vote received: RESTART\n")
			} else if strings.EqualFold(msg.Text, "QUIT") || strings.EqualFold(msg.Text, "EXIT") {
				restartVotes[msg.Player.ID] = false
				mg.writeToPlayer(msg.Player, GenerateTimestampPrefix()+"Goodbye!\n")
				msg.Player.Conn.Close()
			} else {
				restartVotes[msg.Player.ID] = false
			}
		}

		allRestart := len(restartVotes) == remaining
		for _, v := range restartVotes {
			if !v {
				allRestart = false
				break
			}
		}

		if !allRestart {
			activePlayers = mg.getActivePlayers()
			mg.broadcastAll(activePlayers, GenerateTimestampPrefix()+"Not all players voted to restart. Shutting down session.\n")
			mg.LogAnalytics()
			for _, p := range activePlayers {
				_ = p.Conn.Close()
			}
			return nil
		}
	}
}

// runGameSession runs a single game round until someone wins or all leave.
func (mg *MultiplayerGame) runGameSession(secret int) *Player {
	current := 0
	activePlayers := mg.getActivePlayers()

	for {
		if len(activePlayers) == 0 {
			return nil
		}

		p := activePlayers[current%len(activePlayers)]
		mg.writeToPlayer(p, fmt.Sprintf("%sYour turn Player %d. Enter a 4-digit guess (or QUIT to leave):\n", GenerateTimestampPrefix(), p.ID))
		mg.broadcastToList(activePlayers, p, fmt.Sprintf("%sPlayer %d is guessing...\n", GenerateTimestampPrefix(), p.ID))

		// Wait for input from ANY player (so anyone can quit anytime)
		var deadline time.Duration
		if mg.Timeout > 0 {
			deadline = mg.Timeout
		} else {
			deadline = 0 // no timeout
		}

		msg := mg.readFromAnyWithTurnTimeout(activePlayers, p, deadline)

		if msg == nil {
			// Turn timeout for active player
			mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d missed their turn (timeout).\n", GenerateTimestampPrefix(), p.ID))
			current = (current + 1) % len(activePlayers)
			continue
		}

		if msg.Err != nil {
			log.Printf("Player %d disconnected: %v", msg.Player.ID, msg.Err)
			_ = msg.Player.Conn.Close()
			activePlayers = removePlayers(activePlayers, msg.Player)
			mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d has disconnected.\n", GenerateTimestampPrefix(), msg.Player.ID))
			if len(activePlayers) < 2 {
				mg.broadcastAll(activePlayers, fmt.Sprintf("%sNot enough players to continue. Game over.\n", GenerateTimestampPrefix()))
				log.Printf("Not enough players remaining. Ending game session.")
				return nil
			}
			current = current % len(activePlayers)
			continue
		}

		// Handle QUIT/EXIT from any player at any time
		if strings.EqualFold(msg.Text, "QUIT") || strings.EqualFold(msg.Text, "EXIT") {
			log.Printf("Player %d quit the game.", msg.Player.ID)
			mg.writeToPlayer(msg.Player, GenerateTimestampPrefix()+"Goodbye!\n")
			_ = msg.Player.Conn.Close()
			activePlayers = removePlayers(activePlayers, msg.Player)
			mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d has left the game.\n", GenerateTimestampPrefix(), msg.Player.ID))
			if len(activePlayers) < 2 {
				mg.broadcastAll(activePlayers, fmt.Sprintf("%sNot enough players to continue. Game over.\n", GenerateTimestampPrefix()))
				log.Printf("Not enough players remaining. Ending game session.")
				return nil
			}
			current = current % len(activePlayers)
			continue
		}

		// If the message is from someone other than the current player, ignore the guess
		if msg.Player != p {
			mg.writeToPlayer(msg.Player, GenerateTimestampPrefix()+"It's not your turn. Please wait.\n")
			continue
		}

		// Handle RESTART from current player
		if strings.EqualFold(msg.Text, "RESTART") {
			mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d requested restart. Ending this round.\n", GenerateTimestampPrefix(), p.ID))
			return nil
		}

		// Validate guess
		guessNum, err := ValidateGuess(msg.Text)
		if err != nil {
			mg.writeToPlayer(p, GenerateTimestampPrefix()+err.Error()+"\n")
			current = (current + 1) % len(activePlayers)
			continue
		}

		mg.Analytics.RecordGuess(guessNum)

		if guessNum == secret {
			mg.writeToPlayer(p, GenerateTimestampPrefix()+"Congratulations! You guessed the correct number!\n")
			mg.broadcastToList(activePlayers, p, fmt.Sprintf("%sPlayer %d guessed the correct number %d!\n", GenerateTimestampPrefix(), p.ID, secret))
			return p
		}

		feedback := GenerateFeedback(secret, guessNum)
		mg.broadcastAll(activePlayers, fmt.Sprintf("%sPlayer %d guessed %d — %s\n", GenerateTimestampPrefix(), p.ID, guessNum, feedback))
		current = (current + 1) % len(activePlayers)
	}
}

// readFromAnyWithTurnTimeout reads from any player's channel.
// Returns nil if the turn player times out. QUIT from any player is returned immediately.
func (mg *MultiplayerGame) readFromAnyWithTurnTimeout(players []*Player, turnPlayer *Player, timeout time.Duration) *PlayerMessage {
	// Build a dynamic select over all player channels
	var timer <-chan time.Time
	if timeout > 0 {
		timer = time.After(timeout)
	}

	for {
		// Check all player channels with a short poll
		for _, p := range players {
			select {
			case msg := <-p.In:
				return &msg
			default:
			}
		}

		// Check timeout
		if timer != nil {
			select {
			case <-timer:
				return nil
			default:
			}
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// readFromAny reads the next message from any player within a timeout.
func (mg *MultiplayerGame) readFromAny(players []*Player, timeout time.Duration) *PlayerMessage {
	deadline := time.After(timeout)
	for {
		for _, p := range players {
			select {
			case msg := <-p.In:
				return &msg
			default:
			}
		}
		select {
		case <-deadline:
			return nil
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// getActivePlayers returns players whose connections are still open.
func (mg *MultiplayerGame) getActivePlayers() []*Player {
	result := []*Player{}
	for _, p := range mg.players {
		if p != nil && p.Conn != nil {
			result = append(result, p)
		}
	}
	return result
}

// removePlayers returns a new slice without the given player.
func removePlayers(players []*Player, remove *Player) []*Player {
	result := []*Player{}
	for _, p := range players {
		if p != remove {
			result = append(result, p)
		}
	}
	return result
}

// disconnectPlayer closes a player's connection and marks them inactive.
func (mg *MultiplayerGame) disconnectPlayer(p *Player, reason string) {
	if p.Conn != nil {
		_ = p.Conn.Close()
		p.Conn = nil
	}
	log.Printf("Player %d disconnected: %s", p.ID, reason)
}

// writeToPlayer writes a message to a single player.
func (mg *MultiplayerGame) writeToPlayer(p *Player, s string) {
	if p.Conn == nil {
		return
	}
	_, err := p.Conn.Write([]byte(s))
	if err != nil {
		log.Printf("Error writing to player %d: %v", p.ID, err)
		mg.disconnectPlayer(p, "connection lost")
	}
}

// broadcastAll sends to all players in the given list.
func (mg *MultiplayerGame) broadcastAll(players []*Player, s string) {
	for _, p := range players {
		if p != nil {
			_, _ = p.Conn.Write([]byte(s))
		}
	}
}

// broadcastToList sends to all players in the list except one.
func (mg *MultiplayerGame) broadcastToList(players []*Player, except *Player, s string) {
	for _, p := range players {
		if p != nil && p != except {
			_, _ = p.Conn.Write([]byte(s))
		}
	}
}

// LogAnalytics prints the current analytics to the server log.
func (mg *MultiplayerGame) LogAnalytics() {
	mg.Analytics.mu.Lock()
	defer mg.Analytics.mu.Unlock()
	log.Printf("Analytics — Total games: %d, Won: %d, Guess distribution: %v",
		mg.Analytics.TotalGames, mg.Analytics.GamesWon, mg.Analytics.GuessCount)
}
