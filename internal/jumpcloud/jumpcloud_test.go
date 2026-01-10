package jumpcloud

import (
	"encoding/json"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		idpURL   string
		mfaToken string
		wantErr  bool
	}{
		{
			name:     "valid credentials",
			email:    "test@example.com",
			password: "password123",
			idpURL:   "https://example.com/saml",
			mfaToken: "123456",
			wantErr:  false,
		},
		{
			name:     "missing email",
			email:    "",
			password: "password123",
			idpURL:   "https://example.com/saml",
			mfaToken: "123456",
			wantErr:  true,
		},
		{
			name:     "missing password",
			email:    "test@example.com",
			password: "",
			idpURL:   "https://example.com/saml",
			mfaToken: "123456",
			wantErr:  true,
		},
		{
			name:     "missing idpURL",
			email:    "test@example.com",
			password: "password123",
			idpURL:   "",
			mfaToken: "123456",
			wantErr:  true,
		},
		{
			name:     "valid without MFA",
			email:    "test@example.com",
			password: "password123",
			idpURL:   "https://example.com/saml",
			mfaToken: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.email, tt.password, tt.idpURL, tt.mfaToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewWithConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  JumpCloud
		wantErr bool
	}{
		{
			name: "valid config",
			config: JumpCloud{
				Email:                "test@example.com",
				Password:             "password123",
				IdpURL:               "https://example.com/saml",
				MFAToken:             "123456",
				MaxRequestTimeout:    10,
				MaxConnectionTimeout: 5,
			},
			wantErr: false,
		},
		{
			name: "config with default timeouts",
			config: JumpCloud{
				Email:    "test@example.com",
				Password: "password123",
				IdpURL:   "https://example.com/saml",
				MFAToken: "123456",
			},
			wantErr: false,
		},
		{
			name: "invalid config - missing email",
			config: JumpCloud{
				Email:                "",
				Password:             "password123",
				IdpURL:               "https://example.com/saml",
				MaxRequestTimeout:    10,
				MaxConnectionTimeout: 5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewWithConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWithConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result.MaxRequestTimeout == 0 {
					t.Error("Expected default MaxRequestTimeout to be set")
				}
				if result.MaxConnectionTimeout == 0 {
					t.Error("Expected default MaxConnectionTimeout to be set")
				}
			}
		})
	}
}

func TestAuthRequestSerialization(t *testing.T) {
	authReq := authRequest{
		Email:    "test@example.com",
		Password: "password123",
		Otp:      "123456",
	}

	data, err := json.Marshal(authReq)
	if err != nil {
		t.Fatalf("Failed to marshal auth request: %v", err)
	}

	var unmarshaled authRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal auth request: %v", err)
	}

	if unmarshaled.Email != authReq.Email {
		t.Errorf("Expected email %s, got %s", authReq.Email, unmarshaled.Email)
	}

	if unmarshaled.Password != authReq.Password {
		t.Errorf("Expected password %s, got %s", authReq.Password, unmarshaled.Password)
	}

	if unmarshaled.Otp != authReq.Otp {
		t.Errorf("Expected OTP %s, got %s", authReq.Otp, unmarshaled.Otp)
	}
}

func TestJumpCloudTimeouts(t *testing.T) {
	jc := JumpCloud{
		Email:                "test@example.com",
		Password:             "password123",
		IdpURL:               "https://example.com/saml",
		MaxRequestTimeout:    5,
		MaxConnectionTimeout: 2,
	}

	if jc.MaxRequestTimeout != 5 {
		t.Errorf("Expected MaxRequestTimeout 5, got %d", jc.MaxRequestTimeout)
	}

	if jc.MaxConnectionTimeout != 2 {
		t.Errorf("Expected MaxConnectionTimeout 2, got %d", jc.MaxConnectionTimeout)
	}
}

func TestAuthResponseParsing(t *testing.T) {
	tests := []struct {
		name      string
		jsonData  string
		wantErr   bool
		expectMsg string
		expectMFA bool
	}{
		{
			name:      "auth success",
			jsonData:  `{"message": "Authentication successful"}`,
			wantErr:   false,
			expectMsg: "Authentication successful",
			expectMFA: false,
		},
		{
			name:      "auth failed",
			jsonData:  `{"message": "Authentication failed."}`,
			wantErr:   false,
			expectMsg: "Authentication failed.",
			expectMFA: false,
		},
		{
			name:      "MFA required",
			jsonData:  `{"factors":[{"type":"totp","status":"available"}],"message":"MFA required."}`,
			wantErr:   false,
			expectMsg: "MFA required.",
			expectMFA: true,
		},
		{
			name:     "invalid JSON",
			jsonData: `{"invalid": json}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var responseData authResponse
			err := json.Unmarshal([]byte(tt.jsonData), &responseData)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if responseData.Message != tt.expectMsg {
					t.Errorf("Expected message %s, got %s", tt.expectMsg, responseData.Message)
				}

				if tt.expectMFA && len(responseData.Factors) == 0 {
					t.Error("Expected MFA factors to be present")
				}
			}
		})
	}
}

func TestXSRFResponseParsing(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		wantErr     bool
		expectToken string
	}{
		{
			name:        "valid XSRF response",
			jsonData:    `{"xsrf": "test-token-123"}`,
			wantErr:     false,
			expectToken: "test-token-123",
		},
		{
			name:        "empty XSRF token",
			jsonData:    `{"xsrf": ""}`,
			wantErr:     false,
			expectToken: "",
		},
		{
			name:     "invalid JSON",
			jsonData: `{"invalid": json}`,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var xsrfData xsfrResponse
			err := json.Unmarshal([]byte(tt.jsonData), &xsrfData)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && xsrfData.Token != tt.expectToken {
				t.Errorf("Expected token %s, got %s", tt.expectToken, xsrfData.Token)
			}
		})
	}
}
