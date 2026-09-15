package alibaba

import (
	"errors"
	"testing"
)

func TestParseRoleARN(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantAccount string
		wantName    string
		wantErr     bool
	}{
		{
			name:        "valid",
			input:       "acs:ram::1250000000000000:role/admin",
			wantAccount: "1250000000000000",
			wantName:    "admin",
		},
		{
			name:        "name with punctuation",
			input:       "acs:ram::1250000000000000:role/jump-cloud_admin.v2",
			wantAccount: "1250000000000000",
			wantName:    "jump-cloud_admin.v2",
		},
		{name: "aws arn", input: "arn:aws:iam::000000000000:role/admin", wantErr: true},
		{name: "saml provider arn", input: "acs:ram::1250000000000000:saml-provider/jc", wantErr: true},
		{name: "region populated", input: "acs:ram:cn-hangzhou:1250000000000000:role/admin", wantErr: true},
		{name: "partition populated", input: "acs:ram:x:1250000000000000:role/admin", wantErr: true},
		{name: "missing account", input: "acs:ram:::role/admin", wantErr: true},
		{name: "non-numeric account", input: "acs:ram::not-a-number:role/admin", wantErr: true},
		{name: "empty role name", input: "acs:ram::1250000000000000:role/", wantErr: true},
		{name: "trailing whitespace", input: "acs:ram::1250000000000000:role/admin ", wantErr: true},
		{name: "leading whitespace", input: " acs:ram::1250000000000000:role/admin", wantErr: true},
		{name: "embedded newline", input: "acs:ram::1250000000000000:role/admin\nx", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRoleARN(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRoleARN(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRoleARN) {
					t.Errorf("error %v does not wrap ErrInvalidRoleARN", err)
				}
				return
			}
			if got.AccountID != tt.wantAccount {
				t.Errorf("AccountID = %q, want %q", got.AccountID, tt.wantAccount)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestParseSAMLProviderARN(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantAccount string
		wantName    string
		wantErr     bool
	}{
		{
			name:        "valid",
			input:       "acs:ram::1250000000000000:saml-provider/jumpcloud",
			wantAccount: "1250000000000000",
			wantName:    "jumpcloud",
		},
		{name: "role arn", input: "acs:ram::1250000000000000:role/admin", wantErr: true},
		{name: "aws saml provider arn", input: "arn:aws:iam::000000000000:saml-provider/jc", wantErr: true},
		{name: "region populated", input: "acs:ram:cn-hangzhou:1250000000000000:saml-provider/jc", wantErr: true},
		{name: "empty name", input: "acs:ram::1250000000000000:saml-provider/", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSAMLProviderARN(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSAMLProviderARN(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidSAMLProviderARN) {
					t.Errorf("error %v does not wrap ErrInvalidSAMLProviderARN", err)
				}
				return
			}
			if got.AccountID != tt.wantAccount || got.Name != tt.wantName {
				t.Errorf("got %+v, want account %q name %q", got, tt.wantAccount, tt.wantName)
			}
		})
	}
}

func TestParsedARNsExposeAccountForCrossChecking(t *testing.T) {
	// The parsers return the account ID so a caller can later catch the common
	// misconfiguration of a role and SAML provider from different accounts.
	role, err := ParseRoleARN("acs:ram::1250000000000000:role/admin")
	if err != nil {
		t.Fatalf("ParseRoleARN() error = %v", err)
	}
	idp, err := ParseSAMLProviderARN("acs:ram::9990000000000000:saml-provider/jc")
	if err != nil {
		t.Fatalf("ParseSAMLProviderARN() error = %v", err)
	}
	if role.AccountID == idp.AccountID {
		t.Error("expected differing account IDs in this fixture")
	}
}
