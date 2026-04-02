package main

import (
	"testing"

	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/config"
)

// resetViper resets Viper's global state. Call this in every test that
// touches Viper. Note: we do NOT call viper.SetDefault for output-format
// and duration because those defaults come from cobra flag definitions
// (via BindPFlags), which aren't available in unit tests. Tests that need
// those defaults should use viper.Set() explicitly.
func resetViper() {
	viper.Reset()
}

// ---------------------------------------------------------------------------
// resolveString tests
// ---------------------------------------------------------------------------

func TestResolveString_ViperSetWins(t *testing.T) {
	resetViper()
	viper.Set(keyEmail, "flag@example.com")

	acc := &config.Account{Email: "acc@example.com"}
	got := resolveString(keyEmail, acc)
	if got != "flag@example.com" {
		t.Errorf("resolveString: want %q (Viper), got %q", "flag@example.com", got)
	}
}

func TestResolveString_FallsBackToAccount(t *testing.T) {
	resetViper()

	acc := &config.Account{Email: "acc@example.com"}
	got := resolveString(keyEmail, acc)
	if got != "acc@example.com" {
		t.Errorf("resolveString: want %q (account), got %q", "acc@example.com", got)
	}
}

func TestResolveString_NilAccountReturnsEmpty(t *testing.T) {
	resetViper()

	got := resolveString(keyEmail, nil)
	if got != "" {
		t.Errorf("resolveString: want empty, got %q", got)
	}
}

func TestResolveString_AllAccountFields(t *testing.T) {
	resetViper()

	acc := &config.Account{
		Email:           "e",
		Password:        "p",
		MFASecret:       "m",
		IdpURL:          "u",
		AWSPrincipalArn: "a",
		AwsCliProfile:   "c",
		Name:            "myacc",
	}

	tests := []struct {
		key  string
		want string
	}{
		{keyEmail, "e"},
		{keyPassword, "p"},
		{keyMFA, "m"},
		{keyIdpURL, "u"},
		{keyPrincipalARN, "a"},
		{keyAwsCliProfile, "c"},
	}
	for _, tt := range tests {
		got := resolveString(tt.key, acc)
		if got != tt.want {
			t.Errorf("resolveString(%q): want %q, got %q", tt.key, tt.want, got)
		}
	}
}

func TestResolveString_AwsCliProfileFallsBackToName(t *testing.T) {
	resetViper()

	acc := &config.Account{Name: "staging", AwsCliProfile: ""}
	got := resolveString(keyAwsCliProfile, acc)
	if got != "staging" {
		t.Errorf("resolveString(%q): want %q (account name), got %q", keyAwsCliProfile, "staging", got)
	}
}

func TestResolveString_ViperDefaultCountsAsSet(t *testing.T) {
	resetViper()
	// viper.Set (NOT SetDefault) for config-file defaults — makes IsSet true,
	// so Viper value wins over account defaults (correct behavior).
	viper.Set(keyEmail, "default@example.com")

	acc := &config.Account{Email: "acc@example.com"}
	got := resolveString(keyEmail, acc)
	if got != "default@example.com" {
		t.Errorf("resolveString with viper.Set: want %q (config default), got %q", "default@example.com", got)
	}
}

// ---------------------------------------------------------------------------
// resolveDuration tests
// ---------------------------------------------------------------------------

func TestResolveDuration_ViperSetWins(t *testing.T) {
	resetViper()
	viper.Set(keyDuration, 1800)

	acc := &config.Account{Duration: 7200}
	got := resolveDuration(acc)
	if got != 1800 {
		t.Errorf("resolveDuration: want 1800 (Viper), got %d", got)
	}
}

func TestResolveDuration_FallsBackToAccount(t *testing.T) {
	resetViper()

	acc := &config.Account{Duration: 7200}
	got := resolveDuration(acc)
	if got != 7200 {
		t.Errorf("resolveDuration: want 7200 (account), got %d", got)
	}
}

func TestResolveDuration_FallsBackToDefault(t *testing.T) {
	resetViper()

	got := resolveDuration(nil)
	if got != 3600 {
		t.Errorf("resolveDuration: want 3600 (default), got %d", got)
	}
}

func TestResolveDuration_AccountZeroUsesDefault(t *testing.T) {
	resetViper()

	acc := &config.Account{Duration: 0}
	got := resolveDuration(acc)
	if got != 3600 {
		t.Errorf("resolveDuration: want 3600 (default), got %d", got)
	}
}

// ---------------------------------------------------------------------------
// appConfig defaults
// ---------------------------------------------------------------------------

func TestAppConfigDefaults(t *testing.T) {
	resetViper()

	cfg := &appConfig{
		config: &config.Config{},
	}

	// In real usage, cobra flag defaults provide output-format="cli" and
	// duration=3600 via BindPFlags. In unit tests without cobra, these
	// are empty/zero. The hardcoded defaultDuration constant covers duration.
	if defaultDuration != 3600 {
		t.Errorf("defaultDuration constant: want 3600, got %d", defaultDuration)
	}

	// appConfig itself should only have config-related fields
	if cfg.interactive {
		t.Error("interactive should be false by default")
	}
	if cfg.update {
		t.Error("update should be false by default")
	}
}
