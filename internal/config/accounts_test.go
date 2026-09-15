package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// writeConfig writes body to a temporary config file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "jc2aws.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestUnmarshalNeutralKeys(t *testing.T) {
	cfg, err := NewConfig(writeConfig(t, `
accounts:
  - name: prod-ali
    provider: alibaba
    cli_profile: ali
    principal_arn: "acs:ram::1250000000000000:saml-provider/jumpcloud"
    role_arns:
      - name: admin
        arn: "acs:ram::1250000000000000:role/admin"
    regions:
      - eu-central-1
    jc_idp_url: https://sso.jumpcloud.com/saml2/ali
    session_duration: 3600
`))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	a := cfg.Accounts[0]
	if a.Provider != cloud.NameAlibaba {
		t.Errorf("Provider = %q, want %q", a.Provider, cloud.NameAlibaba)
	}
	if a.CLIProfile != "ali" {
		t.Errorf("CLIProfile = %q, want %q", a.CLIProfile, "ali")
	}
	if a.PrincipalARN != "acs:ram::1250000000000000:saml-provider/jumpcloud" {
		t.Errorf("PrincipalARN = %q", a.PrincipalARN)
	}
	if len(a.Roles) != 1 || a.Roles[0].Name != "admin" {
		t.Errorf("Roles = %+v", a.Roles)
	}
	if !slices.Equal(a.Regions, []string{"eu-central-1"}) {
		t.Errorf("Regions = %v", a.Regions)
	}
	if a.Duration != 3600 {
		t.Errorf("Duration = %d, want 3600", a.Duration)
	}
}

func TestUnmarshalLegacyAWSKeys(t *testing.T) {
	cfg, err := NewConfig(writeConfig(t, `
accounts:
  - name: my-prod
    aws_cli_profile: prod
    aws_principal_arn: "arn:aws:iam::000000000000:saml-provider/jumpcloud"
    aws_role_arns:
      - name: admin
        arn: "arn:aws:iam::000000000000:role/jumpcloud-admin"
    aws_regions:
      - us-east-1
      - ca-central-1
    jc_idp_url: https://sso.jumpcloud.com/saml2/my-prod
    session_timeout: 43200
`))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	a := cfg.Accounts[0]
	if a.Provider != cloud.NameAWS {
		t.Errorf("Provider = %q, want %q (missing provider must default to aws)", a.Provider, cloud.NameAWS)
	}
	if a.CLIProfile != "prod" {
		t.Errorf("CLIProfile = %q, want %q", a.CLIProfile, "prod")
	}
	if a.PrincipalARN != "arn:aws:iam::000000000000:saml-provider/jumpcloud" {
		t.Errorf("PrincipalARN = %q", a.PrincipalARN)
	}
	if len(a.Roles) != 1 || a.Roles[0].Arn != "arn:aws:iam::000000000000:role/jumpcloud-admin" {
		t.Errorf("Roles = %+v", a.Roles)
	}
	if !slices.Equal(a.Regions, []string{"us-east-1", "ca-central-1"}) {
		t.Errorf("Regions = %v", a.Regions)
	}
	if a.Duration != 43200 {
		t.Errorf("Duration = %d, want 43200 (session_timeout must migrate)", a.Duration)
	}
}

func TestUnmarshalNeutralKeysWinOverAliases(t *testing.T) {
	cfg, err := NewConfig(writeConfig(t, `
accounts:
  - name: both
    cli_profile: neutral
    aws_cli_profile: legacy
    principal_arn: "arn:aws:iam::000000000000:saml-provider/neutral"
    aws_principal_arn: "arn:aws:iam::000000000000:saml-provider/legacy"
    role_arns:
      - name: neutral-role
        arn: "arn:aws:iam::000000000000:role/neutral"
    aws_role_arns:
      - name: legacy-role
        arn: "arn:aws:iam::000000000000:role/legacy"
    regions: [eu-west-1]
    aws_regions: [us-east-1]
    session_duration: 1800
    session_timeout: 43200
`))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	a := cfg.Accounts[0]
	if a.CLIProfile != "neutral" {
		t.Errorf("CLIProfile = %q, want %q", a.CLIProfile, "neutral")
	}
	if !strings.HasSuffix(a.PrincipalARN, "/neutral") {
		t.Errorf("PrincipalARN = %q, want the neutral key to win", a.PrincipalARN)
	}
	if len(a.Roles) != 1 || a.Roles[0].Name != "neutral-role" {
		t.Errorf("Roles = %+v, want the neutral key to win", a.Roles)
	}
	if !slices.Equal(a.Regions, []string{"eu-west-1"}) {
		t.Errorf("Regions = %v, want the neutral key to win", a.Regions)
	}
	if a.Duration != 1800 {
		t.Errorf("Duration = %d, want session_duration to win over session_timeout", a.Duration)
	}

	if len(a.Warnings()) == 0 {
		t.Error("Warnings() is empty; setting both spellings should warn")
	}
}

func TestUnmarshalProviderNormalization(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{name: "missing", yaml: "provider:", want: cloud.NameAWS},
		{name: "empty string", yaml: `provider: ""`, want: cloud.NameAWS},
		{name: "uppercase", yaml: "provider: ALIBABA", want: cloud.NameAlibaba},
		{name: "padded", yaml: `provider: "  Alibaba  "`, want: cloud.NameAlibaba},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewConfig(writeConfig(t, "accounts:\n  - name: a\n    "+tt.yaml+"\n"))
			if err != nil {
				t.Fatalf("NewConfig() error = %v", err)
			}
			if got := cfg.Accounts[0].Provider; got != tt.want {
				t.Errorf("Provider = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewConfigRejectsUnknownProvider(t *testing.T) {
	_, err := NewConfig(writeConfig(t, "accounts:\n  - name: a\n    provider: gcp\n"))
	if err == nil {
		t.Fatal("NewConfig() accepted an unknown provider; it must fail loudly")
	}
	if !strings.Contains(err.Error(), "gcp") {
		t.Errorf("error %q does not name the offending provider", err)
	}
}

func TestApplyDefaultsNormalizesProviderOnLiterals(t *testing.T) {
	// The TUI and tests build Account values directly, bypassing UnmarshalYAML.
	c := &Config{}
	got := c.applyDefaults(Account{Name: "a"})
	if got.Provider != cloud.DefaultName {
		t.Errorf("applyDefaults left Provider = %q, want %q", got.Provider, cloud.DefaultName)
	}
}

func TestFindRoleByName(t *testing.T) {
	a := Account{Roles: []Role{
		{Name: "admin", Arn: "arn:aws:iam::000000000000:role/admin"},
		{Name: "read-only", Arn: "arn:aws:iam::000000000000:role/ro"},
	}}

	tests := []struct {
		name    string
		lookup  string
		wantArn string
		wantErr bool
	}{
		{name: "first role", lookup: "admin", wantArn: "arn:aws:iam::000000000000:role/admin"},
		{name: "second role", lookup: "read-only", wantArn: "arn:aws:iam::000000000000:role/ro"},
		{name: "missing role", lookup: "nope", wantErr: true},
		{name: "empty name", lookup: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.FindRoleByName(tt.lookup)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FindRoleByName(%q) error = %v, wantErr %v", tt.lookup, err, tt.wantErr)
			}
			if !tt.wantErr && got.Arn != tt.wantArn {
				t.Errorf("FindRoleByName(%q).Arn = %q, want %q", tt.lookup, got.Arn, tt.wantArn)
			}
		})
	}
}

func TestWarningsForLegacyKeysOnNonAWSProvider(t *testing.T) {
	cfg, err := NewConfig(writeConfig(t, `
accounts:
  - name: mixed
    provider: alibaba
    aws_principal_arn: "acs:ram::1250000000000000:saml-provider/jc"
`))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	warnings := strings.Join(cfg.Accounts[0].Warnings(), "\n")
	if !strings.Contains(warnings, "aws_-prefixed") {
		t.Errorf("Warnings() = %q, want a note about aws_-prefixed keys on a non-AWS account", warnings)
	}
}
