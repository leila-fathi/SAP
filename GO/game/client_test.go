package game

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeServer creates a TCP listener on a random port, accepts one connection,
// and runs the handler function against it.
func fakeServer(t *testing.T, handler func(conn net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}()

	return addr
}

func TestClient_ReceivesServerMessages(t *testing.T) {
	tests := []struct {
		name         string
		serverMsgs   []string
		wantContains []string
	}{
		{
			name:         "welcome message",
			serverMsgs:   []string{"Welcome to the game!\n"},
			wantContains: []string{"Welcome"},
		},
		{
			name:         "multiple messages in sequence",
			serverMsgs:   []string{"Welcome\n", "Your turn.\n"},
			wantContains: []string{"Welcome", "Your turn"},
		},
		{
			name:         "shutdown message",
			serverMsgs:   []string{"Shutting down session.\n"},
			wantContains: []string{"Shutting down"},
		},
		{
			name:         "goodbye message",
			serverMsgs:   []string{"Goodbye!\n"},
			wantContains: []string{"Goodbye"},
		},
		{
			name:         "game feedback",
			serverMsgs:   []string{"Try again! Correct: 1, Misplaced: 2\n"},
			wantContains: []string{"Try again", "Correct: 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := fakeServer(t, func(conn net.Conn) {
				for _, msg := range tt.serverMsgs {
					_, _ = conn.Write([]byte(msg))
					time.Sleep(100 * time.Millisecond)
				}
				// Keep connection open long enough for reads
				buf := make([]byte, 256)
				conn.Read(buf)
			})

			conn := connectClient(t, addr)
			defer conn.Close()

			var all string
			for range tt.wantContains {
				all += readMsg(t, conn, 2*time.Second)
			}
			for _, want := range tt.wantContains {
				assert.Contains(t, all, want)
			}
		})
	}
}

func TestClient_SendsToServer(t *testing.T) {
	tests := []struct {
		name     string
		send     string
		wantRecv string
	}{
		{"sends guess", "1234", "1234"},
		{"sends QUIT", "QUIT", "QUIT"},
		{"sends EXIT", "EXIT", "EXIT"},
		{"sends RESTART", "RESTART", "RESTART"},
		{"sends with whitespace trimmed by server", "5678", "5678"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			received := make(chan string, 1)

			addr := fakeServer(t, func(conn net.Conn) {
				_, _ = conn.Write([]byte("Ready\n"))
				buf := make([]byte, 1024)
				n, err := conn.Read(buf)
				if err == nil {
					received <- strings.TrimSpace(string(buf[:n]))
				}
			})

			conn := connectClient(t, addr)
			defer conn.Close()

			readMsg(t, conn, 1*time.Second)
			sendMsg(t, conn, tt.send)

			select {
			case got := <-received:
				assert.Equal(t, tt.wantRecv, got)
			case <-time.After(2 * time.Second):
				t.Fatal("server did not receive message")
			}
		})
	}
}

func TestClient_DetectsDisconnect(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc func(conn net.Conn)
	}{
		{
			name: "server closes immediately",
			serverFunc: func(conn net.Conn) {
				_, _ = conn.Write([]byte("Hello\n"))
				time.Sleep(200 * time.Millisecond)
				conn.Close()
			},
		},
		{
			name: "server closes after message",
			serverFunc: func(conn net.Conn) {
				_, _ = conn.Write([]byte("Game over\n"))
				time.Sleep(100 * time.Millisecond)
				conn.Close()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := fakeServer(t, tt.serverFunc)

			conn := connectClient(t, addr)
			defer conn.Close()

			readMsg(t, conn, 1*time.Second) // drain initial message

			time.Sleep(500 * time.Millisecond)
			_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			buf := make([]byte, 1024)
			_, err := conn.Read(buf)
			assert.Error(t, err, "client should detect server disconnect")
		})
	}
}

func TestClient_RapidMessages(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		minRecv  int
	}{
		{"5 rapid messages", 5, 3},
		{"10 rapid messages", 10, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := fakeServer(t, func(conn net.Conn) {
				for i := 0; i < tt.count; i++ {
					_, _ = conn.Write([]byte("Msg\n"))
					time.Sleep(50 * time.Millisecond)
				}
				time.Sleep(500 * time.Millisecond)
				conn.Close()
			})

			conn := connectClient(t, addr)
			defer conn.Close()

			var all string
			for i := 0; i < tt.count; i++ {
				all += readMsg(t, conn, 1*time.Second)
			}
			count := strings.Count(all, "Msg")
			assert.GreaterOrEqual(t, count, tt.minRecv)
		})
	}
}

func TestClient_ConnectionRefused(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:19999", 1*time.Second)
	if conn != nil {
		conn.Close()
	}
	assert.Error(t, err, "should fail when no server is running")
}
