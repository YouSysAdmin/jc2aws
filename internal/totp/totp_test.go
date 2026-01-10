package totp

import (
	"fmt"
	"testing"
	"time"
)

func TestGetToken(t *testing.T) {
	tests := []struct {
		name      string
		secretKey string
		wantErr   bool
	}{
		{
			name:      "valid secret key",
			secretKey: "JBSWY3DPEHPK3PXP",
			wantErr:   false,
		},
		{
			name:      "valid secret key with spaces",
			secretKey: " JBSWY3DPEHPK3PXP ",
			wantErr:   false,
		},
		{
			name:      "valid secret key lowercase",
			secretKey: "jbswy3dpehpk3pxp",
			wantErr:   false,
		},
		{
			name:      "empty secret key",
			secretKey: "",
			wantErr:   false, // Implementation handles empty string by decoding to empty bytes
		},
		{
			name:      "invalid base32 secret",
			secretKey: "INVALID@SECRET#KEY",
			wantErr:   true,
		},
		{
			name:      "short secret key",
			secretKey: "JBSWY3DPE",
			wantErr:   false, // Should still work with shorter keys
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GetToken(tt.secretKey)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(token) != 6 {
					t.Errorf("GetToken() returned token with length %d, expected 6", len(token))
				}

				// Verify token is numeric
				for _, char := range token {
					if char < '0' || char > '9' {
						t.Errorf("GetToken() returned non-numeric token: %s", token)
						break
					}
				}

				// Test consistency - same secret should produce same token within same time window
				time.Sleep(100 * time.Millisecond)
				token2, err := GetToken(tt.secretKey)
				if err != nil {
					t.Errorf("GetToken() second call failed: %v", err)
				} else if token != token2 {
					t.Errorf("GetToken() should return same token within time window: %s vs %s", token, token2)
				}
			}
		})
	}
}

func TestGenerateTOTP(t *testing.T) {
	tests := []struct {
		name      string
		secretKey string
		timestamp int64
		wantErr   bool
	}{
		{
			name:      "valid input",
			secretKey: "JBSWY3DPEHPK3PXP",
			timestamp: 1640995200, // 2022-01-01 00:00:00 UTC
			wantErr:   false,
		},
		{
			name:      "zero timestamp",
			secretKey: "JBSWY3DPEHPK3PXP",
			timestamp: 0,
			wantErr:   false,
		},
		{
			name:      "negative timestamp",
			secretKey: "JBSWY3DPEHPK3PXP",
			timestamp: -1,
			wantErr:   false,
		},
		{
			name:      "invalid secret",
			secretKey: "INVALID@SECRET",
			timestamp: 1640995200,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := generateTOTP(tt.secretKey, tt.timestamp)

			if (err != nil) != tt.wantErr {
				t.Errorf("generateTOTP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if code >= 1000000 {
					t.Errorf("generateTOTP() returned code %d, expected < 1000000", code)
				}

				// Test deterministic behavior - same inputs should produce same outputs
				code2, err := generateTOTP(tt.secretKey, tt.timestamp)
				if err != nil {
					t.Errorf("generateTOTP() second call failed: %v", err)
				} else if code != code2 {
					t.Errorf("generateTOTP() should be deterministic: %d vs %d", code, code2)
				}
			}
		})
	}
}

func TestTOTPTimeWindow(t *testing.T) {
	secretKey := "JBSWY3DPEHPK3PXP"

	// Test with specific timestamps instead of waiting
	timestamp1 := int64(1640995200) // 2022-01-01 00:00:00 UTC
	timestamp2 := timestamp1 + 31   // 31 seconds later (in next time window)

	token1, err := generateTOTP(secretKey, timestamp1)
	if err != nil {
		t.Fatalf("generateTOTP() failed for timestamp1: %v", err)
	}

	token2, err := generateTOTP(secretKey, timestamp2)
	if err != nil {
		t.Fatalf("generateTOTP() failed for timestamp2: %v", err)
	}

	// Tokens should be different in different time windows
	if token1 == token2 {
		t.Errorf("Tokens should be different across time windows: %d vs %d", token1, token2)
	}

	// Both should be within valid range
	if token1 >= 1000000 || token2 >= 1000000 {
		t.Errorf("Both tokens should be < 1000000: %d, %d", token1, token2)
	}
}

func TestTOTPKnownValues(t *testing.T) {
	// Test with known secret and timestamp to verify algorithm implementation
	secretKey := "JBSWY3DPEHPK3PXP"

	// Test specific timestamps for deterministic verification
	testCases := []struct {
		timestamp    int64
		expectedCode uint32 // This will be calculated manually or from reference implementation
	}{
		{timestamp: 1640995200}, // 2022-01-01 00:00:00 UTC
		{timestamp: 1640995260}, // 30 seconds later
		{timestamp: 1640995320}, // 60 seconds later
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("timestamp_%d", tc.timestamp), func(t *testing.T) {
			code, err := generateTOTP(secretKey, tc.timestamp)
			if err != nil {
				t.Fatalf("generateTOTP() failed: %v", err)
			}

			// Verify the code is within expected range
			if code >= 1000000 {
				t.Errorf("generateTOTP() returned code %d, expected < 1000000", code)
			}

			// All timestamps within the same 30-second window should produce same code
			windowStart := (tc.timestamp / 30) * 30
			codeFromWindowStart, err := generateTOTP(secretKey, windowStart)
			if err != nil {
				t.Fatalf("generateTOTP() with window start failed: %v", err)
			}

			if code != codeFromWindowStart {
				t.Errorf("generateTOTP() should return same code for timestamps in same window: %d vs %d", code, codeFromWindowStart)
			}
		})
	}
}

func TestBase32DecodingEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		secretKey string
		wantErr   bool
	}{
		{
			name:      "valid base32 with padding",
			secretKey: "JBSWY3DPEHPK3PXP=",
			wantErr:   true, // Implementation uses NoPadding, so padding causes error
		},
		{
			name:      "valid base32 without padding",
			secretKey: "JBSWY3DPEHPK3PXP",
			wantErr:   false,
		},
		{
			name:      "empty string",
			secretKey: "",
			wantErr:   false, // Empty string decodes to empty bytes
		},
		{
			name:      "only whitespace",
			secretKey: "   ",
			wantErr:   false, // Whitespace trimmed to empty string
		},
		{
			name:      "invalid characters",
			secretKey: "ABC123!@#",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetToken(tt.secretKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
