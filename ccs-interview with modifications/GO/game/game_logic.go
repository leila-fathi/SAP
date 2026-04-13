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
			if digits[0] != digits[1] && digits[0] != digits[2] && digits[0] != digits[3] &&
				digits[1] != digits[2] && digits[1] != digits[3] &&
				digits[2] != digits[3] {
				return code
			}
		case Medium:
			// existing logic: some transformation for variety
			if sum%2 == 0 {
				code = digits[3]*1000 + digits[2]*100 + digits[1]*10 + digits[0]
			} else {
				for i := 0; i < 4; i++ {
					digits[i] = (digits[i] + 1) % 10
				}
				code = digits[0]*1000 + digits[1]*100 + digits[2]*10 + digits[3]
			}
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

// ValidateGuess ensures the input is exactly 4 digits
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
	return guess, err
}

// GenerateFeedback gives hints about the guess vs the secret code
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

	correct := 0
	misplaced := 0
	hints := []string{}

	for i := 0; i < 4; i++ {
		if secretDigits[i] == guessDigits[i] {
			correct++
		} else if contains(secretDigits[:], guessDigits[i]) {
			misplaced++
		}
	}

	// Bonus hint: location-based
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

// contains checks if an int is in a slice
func contains(arr []int, n int) bool {
	for _, v := range arr {
		if v == n {
			return true
		}
	}
	return false
}

// GenerateTimestampPrefix generates a textual prefix containing the current time
func GenerateTimestampPrefix() string {
	currentTime := time.Now()
	timestamp := currentTime.Unix()
	prefix := fmt.Sprintf("TIME: %d -", timestamp)
	return prefix
}
