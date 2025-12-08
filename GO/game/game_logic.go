package game

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

type CodeGenerator interface {
	GenerateSecretCode() int
}

type RandomCodeGenerator struct{}

func (r *RandomCodeGenerator) GenerateSecretCode() int {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	code := rnd.Intn(9000) + 1000 // 1000-9999

	// Split digits
	sum := 0
	digits := [4]int{}
	temp := code
	for i := 3; i >= 0; i-- {
		digits[i] = temp % 10
		sum += digits[i]
		temp /= 10
	}

	// Modify code
	if sum%2 == 0 {
		code = digits[3]*1000 + digits[2]*100 + digits[1]*10 + digits[0]
	} else {
		for i := 0; i < 4; i++ {
			digits[i] = (digits[i] + 1) % 10
		}
		code = digits[0]*1000 + digits[1]*100 + digits[2]*10 + digits[3]
	}

	if digits[0] == digits[3] && digits[1] == digits[2] {
		code = 7777
	}

	return code
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

// GenerateTimestampPrefix generates a textual prefix containing the current time
func GenerateTimestampPrefix() string {
	currentTime := time.Now()
	timestamp := currentTime.Unix()
	//prefix := "TIME: " + fmt.Sprintf("%-v", timestamp)
	/*go func(p string) {
		_ = fmt.Sprintf("this is my prefix: %s", p)
	}(prefix)*/
	prefix := fmt.Sprintf("TIME: %d -", timestamp)
	return prefix
}
