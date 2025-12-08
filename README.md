# ccs_interview

# Author
Meghdad Mirabi

# Guessing Game (Go TCP Server)

A simple number-guessing game implemented in Go.  
The server listens for TCP connections, receives a 4-digit guess from the client, validates it, compares it with a secret code, and returns a timestamped response.

This project demonstrates:
- Clean architecture with interfaces
- Dependency injection for testability
- gomock-based mocking
- Table-driven unit testing
- Proper random code generation
- TCP server communication

---
# Code Breaker – Multiplayer Go Game


A concurrent, turn-based multiplayer secret-code guessing game written in Go.
The project includes:

- Multiplayer server (N players)
- Turn-based gameplay with optional timeouts
- RESTART / QUIT protocol
- Dependency-injected secret code generator
- Server-side analytics
- Fully interactive TCP client

# Running the Game
## Start the Server

``` go run main.go --mode=server --players=2 --timeout=15 --addr=0.0.0.0:8080 ```

### Flags

| Flag        | Description                                      |
| ----------- | ------------------------------------------------ |
| `--players` | Number of players required to start (default: 2) |
| `--timeout` | Turn timeout in seconds (0 = no timeout)         |
| `--addr`    | Bind address for the server                      |


## Start a Client

```go run main.go --mode=client --addr=localhost:8080```


### Players type:

- 1234 → guess

- RESTART → vote to restart

- QUIT → leave the game

# Core Features
- Turn-Based Multiplayer System

  - Server waits until all required players connect.

  - Automatically rotates through players.

  - Each turn can optionally expire based on a timeout.

- Restart Without Disconnecting

  - When a round ends:

   - All players vote RESTART → new game begins

   - If any player votes QUIT → server shuts down gracefully


- Timestamp Prefix

  - All reply messages include:

```TIME: <unix_timestamp> -```


- Analytics Tracking

  - The server records:

  - Total games played

  - Wins / losses

  - Total guesses

  - Frequency of each guessed number

  - Useful for debugging, dashboards, etc.