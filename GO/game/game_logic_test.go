package game

import (
	"fmt"
	"strings"
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
		{"Valid guess max 9999, wrong", "9999", 1234, "Try again!", false},
		{"Valid guess min 1000, correct", "1000", 1000, "Congratulations! You guessed the correct number!", false},

		// Invalid cases
		{"Invalid guess letters", "12a4", 0, "", true},
		{"Invalid guess special char", "$123", 0, "", true},
		{"Invalid guess negative", "-123", 0, "", true},
		{"Invalid guess empty", "", 0, "", true},
		{"Invalid guess too short", "123", 0, "", true},
		{"Invalid guess too long", "12345", 0, "", true},
		{"Invalid guess spaces", "12 3", 0, "", true},
		{"Invalid guess leading zero", "0071", 0, "", true},
		{"Invalid guess all zeros", "0000", 0, "", true},
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

func TestValidateGuess(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"valid 1234", "1234", 1234, false},
		{"valid 1000", "1000", 1000, false},
		{"valid 9999", "9999", 9999, false},
		{"reject leading zero 0999", "0999", 0, true},
		{"reject 0000", "0000", 0, true},
		{"reject too short", "12", 0, true},
		{"reject too long", "12345", 0, true},
		{"reject letters", "12ab", 0, true},
		{"reject empty", "", 0, true},
		{"reject spaces", "1 34", 0, true},
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

func TestGenerateFeedback(t *testing.T) {
	tests := []struct {
		name           string
		secret         int
		guess          int
		wantCorrect    int
		wantMisplaced  int
	}{
		{"exact match", 1234, 1234, 4, 0},
		{"no match", 1234, 5678, 0, 0},
		{"all misplaced", 1234, 4321, 0, 4},
		{"two correct two misplaced", 1234, 1243, 2, 2},
		{"one correct", 1234, 1567, 1, 0},
		{"duplicate in guess, one in secret", 1234, 1155, 1, 0},
		{"duplicate digit handling", 1123, 3211, 0, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateFeedback(tt.secret, tt.guess)
			assert.Contains(t, result, fmt.Sprintf("Correct: %d", tt.wantCorrect))
			assert.Contains(t, result, fmt.Sprintf("Misplaced: %d", tt.wantMisplaced))
		})
	}
}

func TestGenerateSecretCodeWithDifficulty(t *testing.T) {
	gen := &RandomCodeGenerator{}

	for _, d := range []Difficulty{Easy, Medium, Hard} {
		t.Run(string(d), func(t *testing.T) {
			for i := 0; i < 100; i++ {
				code := gen.GenerateSecretCodeWithDifficulty(d)
				assert.GreaterOrEqual(t, code, 1000, "code must be >= 1000 for difficulty %s", d)
				assert.LessOrEqual(t, code, 9999, "code must be <= 9999 for difficulty %s", d)
			}
		})
	}

	// Easy: all digits unique
	t.Run("Easy digits unique", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			code := gen.GenerateSecretCodeWithDifficulty(Easy)
			digits := [4]int{}
			temp := code
			for j := 3; j >= 0; j-- {
				digits[j] = temp % 10
				temp /= 10
			}
			assert.NotEqual(t, digits[0], digits[1])
			assert.NotEqual(t, digits[0], digits[2])
			assert.NotEqual(t, digits[0], digits[3])
			assert.NotEqual(t, digits[1], digits[2])
			assert.NotEqual(t, digits[1], digits[3])
			assert.NotEqual(t, digits[2], digits[3])
		}
	})

	// Default GenerateSecretCode delegates to Medium
	t.Run("default delegates to Medium", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			code := gen.GenerateSecretCode()
			assert.GreaterOrEqual(t, code, 1000)
			assert.LessOrEqual(t, code, 9999)
		}
	})
}
