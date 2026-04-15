package game

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

// Difficulty levels
type Difficulty string

const (
	Easy   Difficulty = "easy"
	Medium Difficulty = "medium"
	Hard   Difficulty = "hard"
)

// CodeGenerator interface
type CodeGenerator interface {
	GenerateSecretCode() int
	GenerateSecretCodeWithDifficulty(d Difficulty) int
}

// RandomCodeGenerator implements CodeGenerator
type RandomCodeGenerator struct{}

// GenerateSecretCode generates a 4-digit code (default)
func (r *RandomCodeGenerator) GenerateSecretCode() int {
	return r.GenerateSecretCodeWithDifficulty(Medium)
}

// GenerateSecretCodeWithDifficulty supports Easy, Medium, Hard modes
func (r *RandomCodeGenerator) GenerateSecretCodeWithDifficulty(d Difficulty) int {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		code := rnd.Intn(9000) + 1000 // 1000-9999

		// Split digits
		digits := [4]int{}
		sum := 0
		temp := code
		for i := 3; i >= 0; i-- {
			digits[i] = temp % 10
			sum += digits[i]
			temp /= 10
		}

		switch d {
		case Easy:
			// Easy: all digits unique
			if digits[0] != digits[1] && digits[0] != digits[2] && digits[0] != digits[3] &&
				digits[1] != digits[2] && digits[1] != digits[3] &&
				digits[2] != digits[3] {
				return code
			}
		case Medium:
			// Medium: just return any valid 4-digit code
			return code
		case Hard:
			// Hard: at least one repeating digit and sum must be prime
			hasRepeat := digits[0] == digits[1] || digits[0] == digits[2] || digits[0] == digits[3] ||
				digits[1] == digits[2] || digits[1] == digits[3] || digits[2] == digits[3]
			if hasRepeat && isPrime(sum) {
				return code
			}
		}
	}
}

// isPrime checks if a number is prime
func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	for i := 2; i*i <= n; i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}

// ValidateGuess ensures the input is exactly 4 digits in range 1000-9999
func ValidateGuess(input string) (int, error) {
	if len(input) != 4 {
		return 0, fmt.Errorf("guess must be exactly 4 digits")
	}
	for _, c := range input {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("guess must contain only digits")
		}
	}
	guess, err := strconv.Atoi(input)
	if err != nil {
		return 0, err
	}
	if guess < 1000 || guess > 9999 {
		return 0, fmt.Errorf("guess must be between 1000 and 9999")
	}
	return guess, nil
}

// GenerateFeedback gives hints about the guess vs the secret code.
// Uses the standard Mastermind algorithm: first count exact matches,
// then count misplaced digits from the remaining unmatched pools.
func GenerateFeedback(secret, guess int) string {
	secretDigits := [4]int{}
	guessDigits := [4]int{}
	tempSecret := secret
	tempGuess := guess
	for i := 3; i >= 0; i-- {
		secretDigits[i] = tempSecret % 10
		guessDigits[i] = tempGuess % 10
		tempSecret /= 10
		tempGuess /= 10
	}

	// Pass 1: count exact matches
	correct := 0
	secretRemain := [4]int{}
	guessRemain := [4]int{}
	remainCount := 0
	for i := 0; i < 4; i++ {
		if secretDigits[i] == guessDigits[i] {
			correct++
		} else {
			secretRemain[remainCount] = secretDigits[i]
			guessRemain[remainCount] = guessDigits[i]
			remainCount++
		}
	}

	// Pass 2: count misplaced from unmatched digits
	misplaced := 0
	secretPool := make(map[int]int)
	for i := 0; i < remainCount; i++ {
		secretPool[secretRemain[i]]++
	}
	for i := 0; i < remainCount; i++ {
		if secretPool[guessRemain[i]] > 0 {
			misplaced++
			secretPool[guessRemain[i]]--
		}
	}

	// Location hints for exact matches
	hints := []string{}
	for i := 0; i < 4; i++ {
		if secretDigits[i] == guessDigits[i] {
			if i < 2 {
				hints = append(hints, "one correct digit is in the first half")
			} else {
				hints = append(hints, "one correct digit is in the second half")
			}
		}
	}

	return fmt.Sprintf("Correct: %d, Misplaced: %d, Hints: %v", correct, misplaced, hints)
}

// GenerateTimestampPrefix generates a textual prefix containing the current time
func GenerateTimestampPrefix() string {
	currentTime := time.Now()
	timestamp := currentTime.Unix()
	prefix := fmt.Sprintf("TIME: %d - ", timestamp)
	return prefix
}
