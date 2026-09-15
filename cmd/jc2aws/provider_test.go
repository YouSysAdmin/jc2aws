package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/cloud"
	"github.com/yousysadmin/jc2aws/internal/config"
)

func TestResolveString_Provider(t *testing.T) {
	tests := []struct {
		name      string
		viperSet  string
		account   *config.Account
		want      string
		setsViper bool
	}{
		{
			name:    "from account",
			account: &config.Account{Provider: cloud.NameAlibaba},
			want:    cloud.NameAlibaba,
		},
		{
			name:      "flag wins over account",
			viperSet:  cloud.NameAWS,
			account:   &config.Account{Provider: cloud.NameAlibaba},
			want:      cloud.NameAWS,
			setsViper: true,
		},
		{
			name:    "nil account",
			account: nil,
			want:    "",
		},
		{
			name:    "unset on account",
			account: &config.Account{},
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()
			if tt.setsViper {
				viper.Set(keyProvider, tt.viperSet)
			}

			if got := resolveString(keyProvider, tt.account); got != tt.want {
				t.Errorf("resolveString(provider) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveProvider(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		account  *config.Account
		wantName string
		wantErr  bool
	}{
		{name: "nil account defaults to aws", account: nil, wantName: cloud.NameAWS},
		{name: "account provider", account: &config.Account{Provider: cloud.NameAlibaba}, wantName: cloud.NameAlibaba},
		{name: "empty account provider defaults to aws", account: &config.Account{}, wantName: cloud.NameAWS},
		{
			name:     "flag overrides the account",
			flag:     cloud.NameAWS,
			account:  &config.Account{Provider: cloud.NameAlibaba},
			wantName: cloud.NameAWS,
		},
		{name: "unknown flag value fails", flag: "gcp", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()
			if tt.flag != "" {
				viper.Set(keyProvider, tt.flag)
			}

			got, err := resolveProvider(tt.account)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, cloud.ErrUnknownProvider) {
					t.Errorf("error %v does not wrap cloud.ErrUnknownProvider", err)
				}
				return
			}
			if got.Name() != tt.wantName {
				t.Errorf("resolveProvider().Name() = %q, want %q", got.Name(), tt.wantName)
			}
		})
	}
}

// TestLegacyCliProfileEnvVarStillBinds guards the rename of
// --aws-cli-profile-name to --cli-profile-name. Before the rename the old env
// var worked purely through AutomaticEnv name mangling of the old key, so
// without an explicit BindEnv it would silently stop being honoured.
func TestLegacyCliProfileEnvVarStillBinds(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		want   string
	}{
		{name: "current env var", envVar: "J2A_CLI_PROFILE_NAME", want: "from-new"},
		{name: "legacy env var", envVar: "J2A_AWS_CLI_PROFILE_NAME", want: "from-legacy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()
			viper.SetEnvPrefix("J2A")
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
			viper.AutomaticEnv()
			if err := viper.BindEnv(keyCLIProfile, "J2A_CLI_PROFILE_NAME", "J2A_AWS_CLI_PROFILE_NAME"); err != nil {
				t.Fatalf("BindEnv failed: %v", err)
			}

			t.Setenv(tt.envVar, tt.want)

			if !viper.IsSet(keyCLIProfile) {
				t.Fatalf("%s did not make %q set", tt.envVar, keyCLIProfile)
			}
			// An account value must not win over an explicitly set env var.
			acc := &config.Account{Name: "acct", CLIProfile: "from-account"}
			if got := resolveString(keyCLIProfile, acc); got != tt.want {
				t.Errorf("resolveString(cli-profile-name) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCliProfileFallsBackToAccountName(t *testing.T) {
	resetViper()

	acc := &config.Account{Name: "staging"}
	if got := resolveString(keyCLIProfile, acc); got != "staging" {
		t.Errorf("resolveString(cli-profile-name) = %q, want the account name %q", got, "staging")
	}
}

func TestGetCredentialsRejectsNilProvider(t *testing.T) {
	// getCredentials runs inside a bubbletea command goroutine, where a nil
	// dereference would kill the program with the alt-screen still active.
	_, err := getCredentials(t.Context(), credentialRequest{
		Email:    "user@example.com",
		Password: "password",
		Duration: 3600,
	})
	if err == nil {
		t.Fatal("getCredentials() accepted a nil provider")
	}
	if !strings.Contains(err.Error(), "provider") {
		t.Errorf("error %q does not mention the missing provider", err)
	}
}

func TestOutputCredentialsRejectsNilProvider(t *testing.T) {
	if err := outputCredentials(nil, cloud.Credentials{}, "env-stdout", ""); err == nil {
		t.Error("outputCredentials() accepted a nil provider")
	}
}

func TestLaunchShellRejectsNilProvider(t *testing.T) {
	if err := launchShell(nil, cloud.Credentials{}, ""); err == nil {
		t.Error("launchShell() accepted a nil provider")
	}
}

func TestGetCredentialsValidatesARNsBeforeAuthenticating(t *testing.T) {
	// The IdP URL points at a closed port: if these cases reached the network
	// the error would be a dial failure, not an ARN complaint.
	tests := []struct {
		name         string
		provider     string
		principalARN string
		roleARN      string
		want         string
	}{
		{
			name:         "alibaba rejects an aws provider arn",
			provider:     cloud.NameAlibaba,
			principalARN: "arn:aws:iam::1:saml-provider/x",
			roleARN:      "acs:ram::1250000000000000:role/admin",
			want:         "saml provider arn",
		},
		{
			name:         "alibaba rejects an aws role arn",
			provider:     cloud.NameAlibaba,
			principalARN: "acs:ram::1250000000000000:saml-provider/jc",
			roleARN:      "arn:aws:iam::1:role/admin",
			want:         "role arn",
		},
		{
			name:         "aws rejects a non-arn",
			provider:     cloud.NameAWS,
			principalARN: "not-an-arn",
			roleARN:      "arn:aws:iam::1:role/admin",
			want:         "principal arn",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()

			p, err := resolveProvider(&config.Account{Provider: tt.provider})
			if err != nil {
				t.Fatalf("resolveProvider() error = %v", err)
			}

			_, err = getCredentials(t.Context(), credentialRequest{
				Provider:     p,
				Email:        "user@example.com",
				Password:     "password",
				IdpURL:       "https://127.0.0.1:1/idp",
				PrincipalARN: tt.principalARN,
				RoleARN:      tt.roleARN,
				Region:       "eu-central-1",
				Duration:     3600,
			})
			if err == nil {
				t.Fatal("getCredentials() accepted a malformed ARN")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q (did it reach the network first?)", err, tt.want)
			}
		})
	}
}
