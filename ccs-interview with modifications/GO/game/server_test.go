package game

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedCodeGenerator always returns the same code, useful for deterministic tests.
type fixedCodeGenerator struct {
	code int
}

func (f *fixedCodeGenerator) GenerateSecretCode() int                           { return f.code }
func (f *fixedCodeGenerator) GenerateSecretCodeWithDifficulty(_ Difficulty) int { return f.code }

// helper: connect a TCP client to addr, returns the connection
func connectClient(t *testing.T, addr string) net.Conn {
	t.Helper()
	var conn net.Conn
	var err error
	for i := 0; i < 20; i++ {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			return conn
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, err, "failed to connect to server")
	return conn
}

// helper: read a message from connection with timeout
func readMsg(t *testing.T, conn net.Conn, timeout time.Duration) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	return string(buf[:n])
}

// helper: send a message
func sendMsg(t *testing.T, conn net.Conn, msg string) {
	t.Helper()
	_, err := conn.Write([]byte(msg))
	require.NoError(t, err)
}

// startTestServer starts a single-player game server on a random port and returns the address.
func startTestServer(t *testing.T, secret int) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()

	gen := &fixedCodeGenerator{code: secret}
	g := NewGame(gen)

	go func() {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		s := g.CodeGen.GenerateSecretCode()
		writeToClient(conn, GenerateTimestampPrefix()+"New round! Guess the 4-digit secret code (or QUIT to leave).\n")

		for {
			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil {
				return
			}
			guess := strings.TrimSpace(string(buffer[:n]))
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
			if s == numGuess {
				writeToClient(conn, prefix+"Congratulations! You guessed the correct number!\n")
				writeToClient(conn, prefix+"Type RESTART to play again or QUIT to disconnect.\n")
				// Wait for restart or quit decision
				buf := make([]byte, 256)
				rn, rerr := conn.Read(buf)
				if rerr != nil {
					return
				}
				cmd := strings.TrimSpace(string(buf[:rn]))
				if strings.EqualFold(cmd, "RESTART") {
					s = gen.GenerateSecretCode()
					writeToClient(conn, GenerateTimestampPrefix()+"New round! Guess the 4-digit secret code (or QUIT to leave).\n")
					continue
				}
				writeToClient(conn, GenerateTimestampPrefix()+"Goodbye!\n")
				return
			}
			feedback := GenerateFeedback(s, numGuess)
			writeToClient(conn, prefix+"Try again! "+feedback+"\n")
		}
	}()

	return addr
}

// startMultiplayerServer starts a multiplayer server and returns the address.
func startMultiplayerServer(t *testing.T, secret, players int, timeout time.Duration) (*MultiplayerGame, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()

	gen := &fixedCodeGenerator{code: secret}
	mg := NewMultiplayerGame(gen, players, timeout)

	go func() {
		_ = mg.Start(addr)
	}()

	return mg, addr
}

// collectMessages reads all messages from a connection into a channel in the background.
func collectMessages(conn net.Conn) chan string {
	ch := make(chan string, 100)
	go func() {
		buf := make([]byte, 4096)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			ch <- string(buf[:n])
		}
	}()
	return ch
}

// allCollected drains all buffered messages from a collector channel.
func allCollected(ch chan string, wait time.Duration) string {
	time.Sleep(wait)
	var all string
	for {
		select {
		case msg := <-ch:
			all += msg
		default:
			return all
		}
	}
}

// --- Single-player server tests ---

func TestSinglePlayerServer(t *testing.T) {
	tests := []struct {
		name         string
		secret       int
		guesses      []string
		wantContains []string
	}{
		{
			name:         "correct guess on first try",
			secret:       1234,
			guesses:      []string{"1234"},
			wantContains: []string{"Congratulations"},
		},
		{
			name:         "wrong then correct",
			secret:       1234,
			guesses:      []string{"5678", "1234"},
			wantContains: []string{"Try again", "Correct: 0", "Congratulations"},
		},
		{
			name:         "partial match feedback",
			secret:       1234,
			guesses:      []string{"1567"},
			wantContains: []string{"Correct: 1", "Misplaced: 0"},
		},
		{
			name:         "all misplaced feedback",
			secret:       1234,
			guesses:      []string{"4321"},
			wantContains: []string{"Correct: 0", "Misplaced: 4"},
		},
		{
			name:         "invalid input then correct",
			secret:       5555,
			guesses:      []string{"abcd", "5555"},
			wantContains: []string{"must contain only digits", "Congratulations"},
		},
		{
			name:         "quit command",
			secret:       1234,
			guesses:      []string{"QUIT"},
			wantContains: []string{"Goodbye"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := startTestServer(t, tt.secret)
			conn := connectClient(t, addr)
			defer conn.Close()

			// Read welcome
			readMsg(t, conn, 2*time.Second)

			var allResponses string
			for _, guess := range tt.guesses {
				sendMsg(t, conn, guess)
				resp := readMsg(t, conn, 2*time.Second)
				allResponses += resp
			}

			for _, want := range tt.wantContains {
				assert.Contains(t, allResponses, want)
			}
		})
	}
}

// --- Multiplayer server tests ---

func TestMultiplayer(t *testing.T) {
	tests := []struct {
		name      string
		secret    int
		timeout   time.Duration
		actions   func(t *testing.T, p1, p2 net.Conn)
		waitAfter time.Duration
		wantP1    []string
		wantP2    []string
		p2Closed  bool // if true, skip p2 cleanup and p2 assertions use what was collected before close
	}{
		{
			name:    "player 2 guesses correctly",
			secret:  5678,
			timeout: 5 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				// P1 goes first (wrong guess), then P2 guesses correctly
				sendMsg(t, p1, "1234")
				time.Sleep(1 * time.Second)
				sendMsg(t, p2, "5678")
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{"guessed the correct number"},
			wantP2:    []string{"Congratulations"},
		},
		{
			name:    "player quits mid-game",
			secret:  5678,
			timeout: 5 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				sendMsg(t, p2, "QUIT")
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{"has left the game", "Not enough players to continue the round"},
			wantP2:    []string{"Goodbye"},
		},
		{
			name:    "player disconnects mid-game",
			secret:  5678,
			timeout: 5 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				p2.Close()
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{"has disconnected", "Not enough players to continue the round"},
			wantP2:    []string{},
			p2Closed:  true,
		},
		{
			name:    "turn timeout",
			secret:  5678,
			timeout: 1 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				// Don't send anything
			},
			waitAfter: 3 * time.Second,
			wantP1:    []string{"missed their turn"},
			wantP2:    []string{"missed their turn"},
		},
		{
			name:    "player sends EXIT instead of QUIT",
			secret:  5678,
			timeout: 5 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				sendMsg(t, p2, "EXIT")
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{"has left the game", "Not enough players to continue the round"},
			wantP2:    []string{"Goodbye"},
		},
		{
			name:    "invalid guess shows error",
			secret:  5678,
			timeout: 5 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				sendMsg(t, p1, "abcd")
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{"must contain only digits"},
			wantP2:    []string{},
		},
		{
			name:    "out of range guess shows error",
			secret:  5678,
			timeout: 5 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				sendMsg(t, p1, "0001")
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{"must"},
			wantP2:    []string{},
		},
		{
			name:    "RESTART mid-game ends round",
			secret:  5678,
			timeout: 5 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				sendMsg(t, p1, "RESTART")
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{"requested restart"},
			wantP2:    []string{"requested restart"},
		},
		{
			name:    "guess when not your turn rejected",
			secret:  5678,
			timeout: 10 * time.Second,
			actions: func(t *testing.T, p1, p2 net.Conn) {
				// Player 2 sends guess during Player 1's turn
				sendMsg(t, p2, "1234")
			},
			waitAfter: 2 * time.Second,
			wantP1:    []string{},
			wantP2:    []string{"not your turn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, addr := startMultiplayerServer(t, tt.secret, 2, tt.timeout)

			p1 := connectClient(t, addr)
			defer p1.Close()
			p2 := connectClient(t, addr)
			if !tt.p2Closed {
				defer p2.Close()
			}

			// Collect ALL messages from both players from connection start
			ch1 := collectMessages(p1)
			ch2 := collectMessages(p2)

			// Wait for welcome + game start
			time.Sleep(1500 * time.Millisecond)

			// Run test actions (only send, never read)
			tt.actions(t, p1, p2)

			// Wait for server to process and broadcast
			allP1 := allCollected(ch1, tt.waitAfter)
			allP2 := allCollected(ch2, tt.waitAfter)

			for _, want := range tt.wantP1 {
				assert.Contains(t, allP1, want, "player 1 should see: %s", want)
			}
			for _, want := range tt.wantP2 {
				assert.Contains(t, allP2, want, "player 2 should see: %s", want)
			}
		})
	}
}

// --- Analytics tests ---

func TestAnalytics(t *testing.T) {
	t.Run("RecordGuess", func(t *testing.T) {
		tests := []struct {
			name    string
			guesses []int
			check   int
			want    int
		}{
			{"single guess", []int{1234}, 1234, 1},
			{"repeated guess", []int{1234, 1234}, 1234, 2},
			{"multiple different", []int{1234, 5678}, 5678, 1},
			{"unrecorded guess", []int{1234}, 9999, 0},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				a := NewAnalytics()
				for _, g := range tt.guesses {
					a.RecordGuess(g)
				}
				assert.Equal(t, tt.want, a.GuessCount[tt.check])
			})
		}
	})

	t.Run("RecordGame", func(t *testing.T) {
		tests := []struct {
			name      string
			results   []bool
			wantTotal int
			wantWon   int
		}{
			{"all wins", []bool{true, true}, 2, 2},
			{"all losses", []bool{false, false}, 2, 0},
			{"mixed results", []bool{true, false, true}, 3, 2},
			{"single win", []bool{true}, 1, 1},
			{"single loss", []bool{false}, 1, 0},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				a := NewAnalytics()
				for _, win := range tt.results {
					a.RecordGame(win)
				}
				assert.Equal(t, tt.wantTotal, a.TotalGames)
				assert.Equal(t, tt.wantWon, a.GamesWon)
			})
		}
	})
}

// --- Additional game logic edge cases ---

func TestIsPrime(t *testing.T) {
	tests := []struct {
		n    int
		want bool
	}{
		{0, false}, {1, false}, {2, true}, {3, true},
		{4, false}, {5, true}, {10, false}, {13, true},
		{17, true}, {25, false}, {29, true}, {36, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.n), func(t *testing.T) {
			assert.Equal(t, tt.want, isPrime(tt.n))
		})
	}
}

func TestGenerateSecretCode_HardModeConstraints(t *testing.T) {
	gen := &RandomCodeGenerator{}
	for i := 0; i < 100; i++ {
		code := gen.GenerateSecretCodeWithDifficulty(Hard)

		digits := [4]int{}
		sum := 0
		temp := code
		for j := 3; j >= 0; j-- {
			digits[j] = temp % 10
			sum += digits[j]
			temp /= 10
		}

		hasRepeat := digits[0] == digits[1] || digits[0] == digits[2] || digits[0] == digits[3] ||
			digits[1] == digits[2] || digits[1] == digits[3] || digits[2] == digits[3]
		assert.True(t, hasRepeat, "Hard code %d must have a repeating digit", code)
		assert.True(t, isPrime(sum), "Hard code %d digit sum %d must be prime", code, sum)
	}
}

func TestGenerateFeedback_MoreEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		secret        int
		guess         int
		wantCorrect   int
		wantMisplaced int
	}{
		{"same digit secret", 1111, 1234, 1, 0},
		{"same digit guess", 1234, 1111, 1, 0},
		{"both same digit", 2222, 2222, 4, 0},
		{"partial overlap", 1122, 2211, 0, 4},
		{"three correct one wrong", 1234, 1235, 3, 0},
		{"boundary 1000", 1000, 1000, 4, 0},
		{"boundary 9999", 9999, 9999, 4, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateFeedback(tt.secret, tt.guess)
			assert.Contains(t, result, fmt.Sprintf("Correct: %d", tt.wantCorrect))
			assert.Contains(t, result, fmt.Sprintf("Misplaced: %d", tt.wantMisplaced))
		})
	}
}

func TestGenerateFeedback_HintsContent(t *testing.T) {
	tests := []struct {
		name         string
		secret       int
		guess        int
		wantContains string
	}{
		{"correct in first half", 1234, 1567, "first half"},
		{"correct in second half", 1234, 5634, "second half"},
		{"no correct digits no hints", 1234, 5678, "Hints: []"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateFeedback(tt.secret, tt.guess)
			assert.Contains(t, result, tt.wantContains)
		})
	}
}

func TestSinglePlayerServer_RestartFlow(t *testing.T) {
	addr := startTestServer(t, 1234)
	conn := connectClient(t, addr)
	defer conn.Close()

	readMsg(t, conn, 2*time.Second) // welcome

	// Win the first round
	sendMsg(t, conn, "1234")
	// Both "Congratulations" and "Type RESTART" messages may arrive in a single
	// TCP read due to coalescing, so concatenate both reads before asserting.
	resp := readMsg(t, conn, 2*time.Second)
	resp += readMsg(t, conn, 2*time.Second)
	assert.Contains(t, resp, "Congratulations")
	assert.Contains(t, resp, "RESTART")
}

func TestAnalytics_ConcurrentSafety(t *testing.T) {
	a := NewAnalytics()
	done := make(chan struct{})

	// 10 goroutines recording guesses concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				a.RecordGuess(id*100 + j)
				a.RecordGame(j%2 == 0)
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, 1000, a.TotalGames)
	assert.Equal(t, 500, a.GamesWon)
}
