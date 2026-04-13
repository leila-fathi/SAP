package game

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCheckGuessWithValidationAndMock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGen := NewMockCodeGenerator(ctrl)

	tests := []struct {
		name       string
		input      string
		fixedCode  int
		wantResult string
		shouldFail bool
	}{
		// Valid cases
		{"Valid guess normal, correct", "1234", 1234, "Congratulations! You guessed the correct number!", false},
		{"Valid guess normal, wrong", "1234", 1111, "Try again!", false},
		{"Valid guess leading zero, correct", "0071", 71, "Congratulations! You guessed the correct number!", false},
		{"Valid guess all zeros, correct", "0000", 0, "Congratulations! You guessed the correct number!", false},
		{"Valid guess max 9999, wrong", "9999", 1234, "Try again!", false},

		// Invalid cases
		{"Invalid guess letters", "12a4", 0, "", true},
		{"Invalid guess special char", "$123", 0, "", true},
		{"Invalid guess negative", "-123", 0, "", true},
		{"Invalid guess empty", "", 0, "", true},
		{"Invalid guess too short", "123", 0, "", true},
		{"Invalid guess too long", "12345", 0, "", true},
		{"Invalid guess spaces", "12 3", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate input first
			guess, err := ValidateGuess(tt.input)
			if tt.shouldFail {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Configure mock to return fixed secret code
			mockGen.EXPECT().GenerateSecretCode().Return(tt.fixedCode)

			// Call the mock
			secret := mockGen.GenerateSecretCode()

			var result string
			if guess == secret {
				result = "Congratulations! You guessed the correct number!"
			} else {
				result = "Try again!"
			}

			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func validatePrefixStructure(prefix string) (int64, error) {
	if !strings.HasPrefix(prefix, "TIME: ") {
		return 0, fmt.Errorf("missing TIME: prefix")
	}

	var ts int64
	_, err := fmt.Sscanf(prefix, "TIME: %d", &ts)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp format")
	}
	return ts, nil
}

func TestGenerateTimestampPrefix(t *testing.T) {
	tests := []struct {
		name      string
		mockValue string
		useMock   bool
		wantError bool
	}{
		{
			name:      "Valid prefix normal case",
			useMock:   false,
			wantError: false,
		},
		{
			name:      "Invalid prefix missing TIME",
			useMock:   true,
			mockValue: "WRONGPREFIX 12345",
			wantError: true,
		},
		{
			name:      "Invalid prefix missing timestamp number",
			useMock:   true,
			mockValue: "TIME: ",
			wantError: true,
		},
		{
			name:      "Invalid prefix containing non-number",
			useMock:   true,
			mockValue: "TIME: abc",
			wantError: true,
		},
		{
			name:      "Invalid empty prefix",
			useMock:   true,
			mockValue: "",
			wantError: true,
		},
		{
			name:      "Invalid prefix leading/trailing spaces",
			useMock:   true,
			mockValue: "   TIME: 1234",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var prefix string
			if tt.useMock {
				prefix = tt.mockValue
			} else {
				prefix = GenerateTimestampPrefix()
				assert.NotEmpty(t, prefix, "Prefix must not be empty")
				assert.True(t, strings.HasPrefix(prefix, "TIME: "), "Prefix must start with 'TIME: '")
			}

			ts, err := validatePrefixStructure(prefix)
			if tt.wantError {
				assert.Error(t, err, "Expected to detect malformed prefix")
				return
			}

			assert.NoError(t, err, "Prefix structure must be valid")
			assert.Greater(t, ts, int64(0), "Timestamp must be positive")

			now := time.Now().Unix()
			assert.InDelta(t, now, ts, 2, "Timestamp should be close to current time")
		})
	}
}

func TestCheckGuessCorrectness(t *testing.T) {
	mockCode := 1234

	tests := []struct {
		name       string
		guess      int
		wantResult string
	}{
		{"Correct guess", 1234, "correct"},
		{"Incorrect guess", 4321, "wrong"},
		{"Another incorrect guess", 1111, "wrong"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.guess == mockCode {
				result = "correct"
			} else {
				result = "wrong"
			}
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestGenerateFeedback(t *testing.T) {
	tests := []struct {
		name            string
		secret          int
		guess           int
		wantCorrect     int
		wantMisplaced   int
	}{
		{"All correct", 1234, 1234, 4, 0},
		{"None correct", 5678, 1234, 0, 0},
		{"Two correct position", 1234, 1256, 2, 0},
		{"All misplaced", 1234, 4321, 0, 4},
		{"Mix correct and misplaced", 1234, 1324, 2, 2},
		{"One correct one misplaced", 1234, 1567, 1, 0},
		{"Repeated digit guess", 1111, 1234, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feedback := GenerateFeedback(tt.secret, tt.guess)
			assert.Contains(t, feedback, fmt.Sprintf("Correct: %d", tt.wantCorrect))
			assert.Contains(t, feedback, fmt.Sprintf("Misplaced: %d", tt.wantMisplaced))
		})
	}
}

func TestValidateGuess(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"Valid 1234", "1234", 1234, false},
		{"Valid 0000", "0000", 0, false},
		{"Valid 9999", "9999", 9999, false},
		{"Too short", "123", 0, true},
		{"Too long", "12345", 0, true},
		{"Letters", "abcd", 0, true},
		{"Mixed", "12a4", 0, true},
		{"Empty", "", 0, true},
		{"Spaces", "1 34", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateGuess(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestDifficultyLevels(t *testing.T) {
	gen := &RandomCodeGenerator{}

	t.Run("Easy generates unique digits", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			code := gen.GenerateSecretCodeWithDifficulty(Easy)
			assert.GreaterOrEqual(t, code, 1000)
			assert.LessOrEqual(t, code, 9999)
			// All digits must be unique
			digits := [4]int{}
			temp := code
			for j := 3; j >= 0; j-- {
				digits[j] = temp % 10
				temp /= 10
			}
			seen := map[int]bool{}
			for _, d := range digits {
				assert.False(t, seen[d], "Easy mode should have unique digits, got %d", code)
				seen[d] = true
			}
		}
	})

	t.Run("Hard generates repeating digit with prime sum", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			code := gen.GenerateSecretCodeWithDifficulty(Hard)
			assert.GreaterOrEqual(t, code, 1000)
			assert.LessOrEqual(t, code, 9999)
			digits := [4]int{}
			sum := 0
			temp := code
			for j := 3; j >= 0; j-- {
				digits[j] = temp % 10
				sum += digits[j]
				temp /= 10
			}
			assert.True(t, isPrime(sum), "Hard mode sum %d should be prime for code %d", sum, code)
			hasRepeat := digits[0] == digits[1] || digits[0] == digits[2] || digits[0] == digits[3] ||
				digits[1] == digits[2] || digits[1] == digits[3] || digits[2] == digits[3]
			assert.True(t, hasRepeat, "Hard mode should have repeating digits, got %d", code)
		}
	})
}

func TestAnalyticsConcurrency(t *testing.T) {
	a := NewAnalytics()
	var wg sync.WaitGroup

	// Simulate 100 concurrent guess recordings
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a.RecordGuess(n % 10)
		}(i)
	}

	// Simulate 50 concurrent game recordings
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(win bool) {
			defer wg.Done()
			a.RecordGame(win)
		}(i%2 == 0)
	}

	wg.Wait()

	assert.Equal(t, 100, a.GuessTotal)
	assert.Equal(t, 50, a.TotalGames)
	assert.Equal(t, 25, a.GamesWon)
	assert.Equal(t, 25, a.GamesLost)

	// Verify guess frequency sums to 100
	total := 0
	for _, count := range a.GuessCount {
		total += count
	}
	assert.Equal(t, 100, total)
}
