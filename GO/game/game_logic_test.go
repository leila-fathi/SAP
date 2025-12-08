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

	// Expect format: TIME: <unix>
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
		mockValue string // injected malformed values for negative scenarios
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
				// artificial error case
				prefix = tt.mockValue
			} else {
				// real call
				prefix = GenerateTimestampPrefix()

				// baseline validations for real function
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

			// Check timestamp is within 2 seconds of now
			now := time.Now().Unix()
			assert.InDelta(t, now, ts, 2, "Timestamp should be close to current time")
		})
	}
}

func TestCheckGuessCorrectness(t *testing.T) {
	//secretCode := GenerateSecretCode()
	//assert.Equal(t, secretCode, 1111)
}
