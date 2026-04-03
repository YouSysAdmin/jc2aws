package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/config"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestConfig creates a minimal appConfig with a config containing the given accounts.
func newTestConfig(accounts []config.Account) *appConfig {
	return &appConfig{
		config: &config.Config{
			Accounts: accounts,
		},
	}
}

// testAccounts returns a standard pair of accounts for tests.
func testAccounts() []config.Account {
	return []config.Account{
		{
			Name:            "prod",
			Description:     "Production account",
			Email:           "prod@example.com",
			Password:        "prodpass",
			MFASecret:       "prodmfa",
			IdpURL:          "https://sso.jumpcloud.com/saml2/prod",
			AWSPrincipalArn: "arn:aws:iam::111:saml-provider/prod",
			AwsCliProfile:   "prod-profile",
			Duration:        7200,
			AWSRoleArns: []config.AWSRole{
				{Name: "admin", Arn: "arn:aws:iam::111:role/admin", Description: "Admin role"},
				{Name: "readonly", Arn: "arn:aws:iam::111:role/readonly"},
			},
			AWSRegions: []string{"us-east-1", "eu-west-1"},
		},
		{
			Name:        "staging",
			Description: "Staging account",
		},
	}
}

// stepValue returns the display value for a step in the model's step metadata.
func stepValue(m tuiModel, id stepID) string {
	return m.stepDisplay(id)
}

// stepSrc returns the source for a step.
func stepSrc(m tuiModel, id stepID) string {
	return m.stepSource(id)
}

// ---------------------------------------------------------------------------
// resolveOutputFormat tests
// ---------------------------------------------------------------------------

func TestResolveOutputFormat_PresetFlagWins(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "env")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg: cfg,
		values: map[stepID]string{stepOutputFormat: "shell"},
	}

	got := m.resolveOutputFormat()
	if got != "env" {
		t.Errorf("resolveOutputFormat: want %q (preset flag), got %q", "env", got)
	}
}

func TestResolveOutputFormat_InteractiveValue(t *testing.T) {
	resetViper()
	// outputFormat not explicitly set — only default "cli" exists

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg: cfg,
		values: map[stepID]string{stepOutputFormat: "shell"},
	}

	got := m.resolveOutputFormat()
	if got != "shell" {
		t.Errorf("resolveOutputFormat: want %q (interactive), got %q", "shell", got)
	}
}

func TestResolveOutputFormat_FallbackToDefault(t *testing.T) {
	resetViper()
	// In real usage, cobra flag default provides "cli" via BindPFlags.
	// In unit tests, simulate this with a default value that doesn't
	// make IsSet() true — we can't easily do this without cobra, so
	// we test the fallback behavior: with nothing set and no interactive
	// value, resolveOutputFormat returns whatever viper.GetString returns
	// (empty in tests, "cli" in production via cobra flag default).
	// This test verifies the fallback path is exercised.
	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg: cfg,
		values: map[stepID]string{},
	}

	got := m.resolveOutputFormat()
	// Without cobra flags bound, Viper returns "" for unset keys.
	// The important behavior: IsSet is false, no interactive value,
	// so it falls through to viper.GetString (the flag default path).
	if got != "" {
		t.Errorf("resolveOutputFormat without cobra: want empty (no flag default), got %q", got)
	}
}

// ---------------------------------------------------------------------------
// initStep auto-skip tests
// ---------------------------------------------------------------------------

func TestInitStep_AccountPreset(t *testing.T) {
	resetViper()
	viper.Set(keyAccount, "prod")

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	// Account should be auto-skipped and account resolved
	if stepValue(m, stepAccount) != "prod" {
		t.Errorf("account should be preset to 'prod', got %q", stepValue(m, stepAccount))
	}
	if stepSrc(m, stepAccount) != sourcePreset {
		t.Errorf("account source should be %q, got %q", sourcePreset, stepSrc(m, stepAccount))
	}
	if m.account == nil {
		t.Fatal("account pointer should be set after preset account selection")
	}
	if m.account.Name != "prod" {
		t.Errorf("account.Name: want %q, got %q", "prod", m.account.Name)
	}
	// Should have advanced past account step
	if m.current == stepAccount {
		t.Error("current step should have advanced past stepAccount")
	}
}

func TestInitStep_NoAccounts(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil) // no accounts
	m := newTuiModel(cfg)

	if stepValue(m, stepAccount) != "(no config)" {
		t.Errorf("account should show '(no config)' when no accounts, got %q", stepValue(m, stepAccount))
	}
	if m.current == stepAccount {
		t.Error("should advance past stepAccount when no accounts")
	}
}

func TestInitStep_AccountInteractive(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	// account not set — should show interactive select

	m := newTuiModel(cfg)

	if m.current != stepAccount {
		t.Errorf("should stop at stepAccount for interactive selection, got step %d", m.current)
	}
	if m.compType != "select" {
		t.Errorf("compType should be 'select', got %q", m.compType)
	}
}

func TestInitStep_EmailPreset(t *testing.T) {
	resetViper()
	viper.Set(keyEmail, "user@example.com")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepEmail,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepEmail) != "user@example.com" {
		t.Errorf("email preset: want %q, got %q", "user@example.com", stepValue(m, stepEmail))
	}
	if m.current == stepEmail {
		t.Error("should advance past stepEmail when preset")
	}
}

func TestInitStep_EmailInteractive(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepEmail,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if m.current != stepEmail {
		t.Errorf("should stay at stepEmail for interactive, got step %d", m.current)
	}
	if m.compType != "input" {
		t.Errorf("compType should be 'input', got %q", m.compType)
	}
}

func TestInitStep_PasswordPreset(t *testing.T) {
	resetViper()
	viper.Set(keyPassword, "secret")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepPassword,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepPassword) != "(set)" {
		t.Errorf("password preset display: want '(set)', got %q", stepValue(m, stepPassword))
	}
	if m.current == stepPassword {
		t.Error("should advance past stepPassword when preset")
	}
}

func TestInitStep_RegionPreset(t *testing.T) {
	resetViper()
	viper.Set(keyRegion, "eu-west-1")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRegion,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepRegion) != "eu-west-1" {
		t.Errorf("region preset: want %q, got %q", "eu-west-1", stepValue(m, stepRegion))
	}
	if m.current == stepRegion {
		t.Error("should advance past stepRegion when preset")
	}
}

func TestInitStep_RegionInteractive(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRegion,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if m.compType != "select" {
		t.Errorf("compType should be 'select' for region, got %q", m.compType)
	}
}

func TestInitStep_OutputFormatPreset(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "shell")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepOutputFormat,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepOutputFormat) != "shell" {
		t.Errorf("outputFormat preset: want %q, got %q", "shell", stepValue(m, stepOutputFormat))
	}
	if m.current == stepOutputFormat {
		t.Error("should advance past stepOutputFormat when preset")
	}
}

func TestInitStep_OutputFormatInteractive(t *testing.T) {
	resetViper()
	// Only default "cli" — not explicitly set

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepOutputFormat,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if m.compType != "select" {
		t.Errorf("compType should be 'select' for output format, got %q", m.compType)
	}
}

func TestInitStep_RoleARNPreset(t *testing.T) {
	resetViper()
	viper.Set(keyRoleARN, "arn:aws:iam::111:role/direct")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRole,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepRole) != "arn:aws:iam::111:role/direct" {
		t.Errorf("roleARN preset: want %q, got %q", "arn:aws:iam::111:role/direct", stepValue(m, stepRole))
	}
}

func TestInitStep_RoleNamePreset(t *testing.T) {
	resetViper()
	viper.Set(keyRoleName, "admin")

	cfg := newTestConfig(nil)
	acc := testAccounts()[0]
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRole,
		values:  make(map[stepID]string),
		account: &acc,
	}
	m.initStep()

	// Should display the role name, but store the ARN
	if stepValue(m, stepRole) != "admin" {
		t.Errorf("role preset display: want %q, got %q", "admin", stepValue(m, stepRole))
	}
	if m.values[stepRole] != "arn:aws:iam::111:role/admin" {
		t.Errorf("role ARN in values: want %q, got %q", "arn:aws:iam::111:role/admin", m.values[stepRole])
	}
}

func TestInitStep_RoleSelectWithAccount(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	acc := testAccounts()[0]

	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRole,
		values:  make(map[stepID]string),
		account: &acc,
	}
	m.initStep()

	if m.compType != "select" {
		t.Errorf("compType should be 'select' for role with account roles, got %q", m.compType)
	}
}

func TestInitStep_RoleInputWithoutAccount(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRole,
		values:  make(map[stepID]string),
		account: nil,
	}
	m.initStep()

	if m.compType != "input" {
		t.Errorf("compType should be 'input' for role without account, got %q", m.compType)
	}
}

func TestInitStep_IdpURLPreset(t *testing.T) {
	resetViper()
	viper.Set(keyIdpURL, "https://sso.jumpcloud.com/saml2/test")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepIdpURL,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepIdpURL) != "https://sso.jumpcloud.com/saml2/test" {
		t.Errorf("idpURL preset: want url, got %q", stepValue(m, stepIdpURL))
	}
	if m.current == stepIdpURL {
		t.Error("should advance past stepIdpURL when preset")
	}
}

func TestInitStep_PrincipalARNPreset(t *testing.T) {
	resetViper()
	viper.Set(keyPrincipalARN, "arn:aws:iam::111:saml-provider/jumpcloud")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepPrincipalARN,
		values:  make(map[stepID]string),
	}
	m.initStep()

	// Should be truncated since > 30 chars
	display := stepValue(m, stepPrincipalARN)
	arn := "arn:aws:iam::111:saml-provider/jumpcloud"
	if len(arn) > 30 && display[:3] != "..." {
		t.Errorf("long principalARN should be truncated, got %q", display)
	}
}

func TestInitStep_MFAPreset(t *testing.T) {
	resetViper()
	viper.Set(keyMFA, "123456")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepMFA,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepMFA) != "(set)" {
		t.Errorf("MFA preset display: want '(set)', got %q", stepValue(m, stepMFA))
	}
}

// ---------------------------------------------------------------------------
// initStep — stepAwsCliProfile conditional skip
// ---------------------------------------------------------------------------

func TestInitStep_AwsCliProfileSkippedForNonCliFormat(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "env")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAwsCliProfile,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if stepValue(m, stepAwsCliProfile) != "(n/a)" {
		t.Errorf("profile should be '(n/a)' for non-cli format, got %q", stepValue(m, stepAwsCliProfile))
	}
}

func TestInitStep_AwsCliProfileShownForCliFormat(t *testing.T) {
	resetViper()
	// outputFormat not explicitly set — interactive selected "cli"

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAwsCliProfile,
		values:  map[stepID]string{stepOutputFormat: "cli"},
	}
	m.initStep()

	if m.compType != "input" {
		t.Errorf("compType should be 'input' for cli profile with cli format, got %q", m.compType)
	}
}

func TestInitStep_AwsCliProfilePresetValue(t *testing.T) {
	resetViper()
	viper.Set(keyAwsCliProfile, "myprofile")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAwsCliProfile,
		values:  map[stepID]string{stepOutputFormat: "cli"},
	}
	m.initStep()

	if stepValue(m, stepAwsCliProfile) != "myprofile" {
		t.Errorf("profile should be 'myprofile', got %q", stepValue(m, stepAwsCliProfile))
	}
	if m.current == stepAwsCliProfile {
		t.Error("should advance past stepAwsCliProfile when preset")
	}
}

func TestInitStep_AwsCliProfileFromAccount(t *testing.T) {
	resetViper()
	acc := config.Account{Name: "myacc", AwsCliProfile: "acc-profile"}

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAwsCliProfile,
		values:  map[stepID]string{stepOutputFormat: "cli"},
		account: &acc,
	}
	m.initStep()

	if stepValue(m, stepAwsCliProfile) != "acc-profile" {
		t.Errorf("profile should come from account: want %q, got %q", "acc-profile", stepValue(m, stepAwsCliProfile))
	}
}

// ---------------------------------------------------------------------------
// initStep — stepConfirm
// ---------------------------------------------------------------------------

func TestInitStep_Confirm(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepConfirm,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if m.compType != "choice" {
		t.Errorf("compType should be 'choice' at confirm step, got %q", m.compType)
	}
}

// ---------------------------------------------------------------------------
// initStep — stepDone branching by output format
// ---------------------------------------------------------------------------

func TestInitStep_DoneShellFormat(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "shell")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepDone,
		values:  make(map[stepID]string),
	}
	m.initStep()

	if m.compType != "await-key" {
		t.Errorf("compType should be 'await-key' for shell format, got %q", m.compType)
	}
}

func TestInitStep_DonePresetStdout(t *testing.T) {
	for _, format := range []string{"cli-stdout", "env-stdout"} {
		t.Run(format, func(t *testing.T) {
			resetViper()
			viper.Set(keyOutputFormat, format)

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:  cfg,
				steps:   allStepMeta(),
				current: stepDone,
				values:  make(map[stepID]string),
			}
			m.initStep()

			if m.compType != "" {
				t.Errorf("compType should be empty for preset %s (immediate quit), got %q", format, m.compType)
			}
			if !m.done {
				t.Errorf("done should be true for preset %s", format)
			}
		})
	}
}

func TestInitStep_DoneFileBasedFormat(t *testing.T) {
	for _, format := range []string{"cli", "env"} {
		t.Run(format+"/default-exit", func(t *testing.T) {
			resetViper()
			viper.Set(keyOutputFormat, format)
			// No tui_done_action set — defaults to exit

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:  cfg,
				steps:   allStepMeta(),
				current: stepDone,
				values:  make(map[stepID]string),
			}
			m.initStep()

			if m.compType != "" {
				t.Errorf("compType should be empty for default exit, got %q", m.compType)
			}
			if !m.done {
				t.Errorf("done should be true for default exit")
			}
		})

		t.Run(format+"/exit", func(t *testing.T) {
			resetViper()
			viper.Set(keyOutputFormat, format)
			viper.Set(keyTUIDoneAction, "exit")

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:  cfg,
				steps:   allStepMeta(),
				current: stepDone,
				values:  make(map[stepID]string),
			}
			m.initStep()

			if m.compType != "" {
				t.Errorf("compType should be empty for exit, got %q", m.compType)
			}
			if !m.done {
				t.Errorf("done should be true for exit")
			}
		})

		t.Run(format+"/menu", func(t *testing.T) {
			resetViper()
			viper.Set(keyOutputFormat, format)
			viper.Set(keyTUIDoneAction, "menu")

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:  cfg,
				steps:   allStepMeta(),
				current: stepDone,
				values:  make(map[stepID]string),
			}
			m.initStep()

			if m.compType != "choice" {
				t.Errorf("compType should be 'choice' for menu, got %q", m.compType)
			}
			if m.done {
				t.Errorf("done should be false for menu")
			}
		})

		t.Run(format+"/wait", func(t *testing.T) {
			resetViper()
			viper.Set(keyOutputFormat, format)
			viper.Set(keyTUIDoneAction, "wait")

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:  cfg,
				steps:   allStepMeta(),
				current: stepDone,
				values:  make(map[stepID]string),
			}
			m.initStep()

			if m.compType != "await-key" {
				t.Errorf("compType should be 'await-key' for wait, got %q", m.compType)
			}
			if m.done {
				t.Errorf("done should be false for wait")
			}
		})
	}
}

func TestInitStep_DoneInteractiveStdout(t *testing.T) {
	// When stdout format was interactively selected (not preset),
	// should still immediately quit (deferred to post-TUI output).
	for _, format := range []string{"cli-stdout", "env-stdout"} {
		t.Run(format, func(t *testing.T) {
			resetViper()
			// outputFormat not explicitly set via viper.Set

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:  cfg,
				steps:   allStepMeta(),
				current: stepDone,
				values:  map[stepID]string{stepOutputFormat: format},
			}
			m.initStep()

			if m.compType != "" {
				t.Errorf("compType should be empty for interactive %s (immediate quit), got %q", format, m.compType)
			}
			if !m.done {
				t.Errorf("done should be true for interactive %s", format)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleSelectResult tests
// ---------------------------------------------------------------------------

func TestHandleSelectResult_Account(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
	}

	m.handleSelectResult(selectItem{name: "prod"})

	if m.account == nil {
		t.Fatal("account should be set after selection")
	}
	if m.account.Name != "prod" {
		t.Errorf("account.Name: want %q, got %q", "prod", m.account.Name)
	}
	if stepSrc(m, stepAccount) != sourceInteractive {
		t.Errorf("source should be %q, got %q", sourceInteractive, stepSrc(m, stepAccount))
	}
	if stepValue(m, stepAccount) != "prod" {
		t.Errorf("display: want %q, got %q", "prod", stepValue(m, stepAccount))
	}
	// Account defaults should now be resolved via resolveString with the account pointer
	email := resolveString(keyEmail, m.account)
	if email != "prod@example.com" {
		t.Errorf("account email via resolveString: want %q, got %q", "prod@example.com", email)
	}
}

func TestHandleSelectResult_RoleStoresARN(t *testing.T) {
	resetViper()

	acc := testAccounts()[0]
	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRole,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.handleSelectResult(selectItem{name: "admin"})

	if m.values[stepRole] != "arn:aws:iam::111:role/admin" {
		t.Errorf("role ARN: want %q, got %q", "arn:aws:iam::111:role/admin", m.values[stepRole])
	}
	if stepValue(m, stepRole) != "admin" {
		t.Errorf("display: want %q, got %q", "admin", stepValue(m, stepRole))
	}
	if stepSrc(m, stepRole) != sourceInteractive {
		t.Errorf("source should be %q, got %q", sourceInteractive, stepSrc(m, stepRole))
	}
}

func TestHandleSelectResult_Region(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepRegion,
		values:  make(map[stepID]string),
	}

	m.handleSelectResult(selectItem{name: "eu-west-1"})

	if m.values[stepRegion] != "eu-west-1" {
		t.Errorf("region: want %q, got %q", "eu-west-1", m.values[stepRegion])
	}
}

func TestHandleSelectResult_OutputFormat(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepOutputFormat,
		values:  make(map[stepID]string),
	}

	m.handleSelectResult(selectItem{name: "env-stdout"})

	if m.values[stepOutputFormat] != "env-stdout" {
		t.Errorf("outputFormat: want %q, got %q", "env-stdout", m.values[stepOutputFormat])
	}
}

// ---------------------------------------------------------------------------
// handleInputResult tests
// ---------------------------------------------------------------------------

func TestHandleInputResult_Email(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepEmail, values: make(map[stepID]string)}
	m.handleInputResult("user@example.com")

	if m.values[stepEmail] != "user@example.com" {
		t.Errorf("email value: want %q, got %q", "user@example.com", m.values[stepEmail])
	}
	if stepValue(m, stepEmail) != "user@example.com" {
		t.Errorf("email display: want %q, got %q", "user@example.com", stepValue(m, stepEmail))
	}
}

func TestHandleInputResult_PasswordMasked(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepPassword, values: make(map[stepID]string)}
	m.handleInputResult("secret123")

	if m.values[stepPassword] != "secret123" {
		t.Errorf("password raw value: want %q, got %q", "secret123", m.values[stepPassword])
	}
	if stepValue(m, stepPassword) != "(set)" {
		t.Errorf("password display should be masked: want '(set)', got %q", stepValue(m, stepPassword))
	}
}

func TestHandleInputResult_MFAMasked(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepMFA, values: make(map[stepID]string)}
	m.handleInputResult("123456")

	if m.values[stepMFA] != "123456" {
		t.Errorf("mfa raw value: want %q, got %q", "123456", m.values[stepMFA])
	}
	if stepValue(m, stepMFA) != "(set)" {
		t.Errorf("mfa display should be masked: want '(set)', got %q", stepValue(m, stepMFA))
	}
}

func TestHandleInputResult_RoleTruncatesARN(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	longARN := "arn:aws:iam::123456789012:role/very-long-role-name-that-exceeds-30"
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepRole, values: make(map[stepID]string)}
	m.handleInputResult(longARN)

	if m.values[stepRole] != longARN {
		t.Errorf("role raw value should be full ARN: got %q", m.values[stepRole])
	}
	display := stepValue(m, stepRole)
	if len(display) > 30 {
		t.Errorf("role display should be truncated to <=30 chars, got %d: %q", len(display), display)
	}
}

func TestHandleInputResult_PrincipalARNTruncated(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	longARN := "arn:aws:iam::123456789012:saml-provider/jumpcloud-provider"
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepPrincipalARN, values: make(map[stepID]string)}
	m.handleInputResult(longARN)

	if m.values[stepPrincipalARN] != longARN {
		t.Error("raw value should be full ARN")
	}
	display := stepValue(m, stepPrincipalARN)
	if len(display) > 30 {
		t.Errorf("display should be truncated, got %d chars: %q", len(display), display)
	}
}

func TestHandleInputResult_IdpURL(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepIdpURL, values: make(map[stepID]string)}
	m.handleInputResult("https://sso.jumpcloud.com/saml2/test")

	if m.values[stepIdpURL] != "https://sso.jumpcloud.com/saml2/test" {
		t.Errorf("idpURL value: want url, got %q", m.values[stepIdpURL])
	}
}

func TestHandleInputResult_AwsCliProfile(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepAwsCliProfile, values: make(map[stepID]string)}
	m.handleInputResult("my-profile")

	if m.values[stepAwsCliProfile] != "my-profile" {
		t.Errorf("profile value: want %q, got %q", "my-profile", m.values[stepAwsCliProfile])
	}
	if stepValue(m, stepAwsCliProfile) != "my-profile" {
		t.Errorf("profile display: want %q, got %q", "my-profile", stepValue(m, stepAwsCliProfile))
	}
}

func TestHandleInputResult_AdvancesStep(t *testing.T) {
	resetViper()
	viper.Set(keyPassword, "preset") // so password step skips

	cfg := newTestConfig(nil)
	m := tuiModel{appCfg: cfg, steps: allStepMeta(), current: stepEmail, values: make(map[stepID]string)}
	m.handleInputResult("user@example.com")

	// Should have advanced past email, and since password is preset, past password too
	if m.current <= stepEmail {
		t.Errorf("should have advanced past stepEmail, got step %d", m.current)
	}
}

// ---------------------------------------------------------------------------
// handleChoiceResult tests
// ---------------------------------------------------------------------------

func TestHandleChoiceResult_ConfirmAdvancesToFetching(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepConfirm,
		values:  make(map[stepID]string),
	}
	m.choiceComp = newChoiceModel("Confirm", []string{"Confirm", "Restart"})
	m.choiceComp.chosen = confirmChoiceConfirm

	result, _ := m.handleChoiceResult()
	rm := result.(tuiModel)

	if rm.current != stepFetching {
		t.Errorf("should transition to stepFetching, got step %d", rm.current)
	}
	if rm.compType != "spinner" {
		t.Errorf("compType should be 'spinner', got %q", rm.compType)
	}
}

func TestHandleChoiceResult_ConfirmRestart(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepConfirm,
		values:  map[stepID]string{stepEmail: "old@example.com"},
	}
	m.choiceComp = newChoiceModel("Confirm", []string{"Confirm", "Restart"})
	m.choiceComp.chosen = confirmChoiceRestart

	result, _ := m.handleChoiceResult()
	rm := result.(tuiModel)

	if rm.current != stepAccount {
		t.Errorf("restart should return to stepAccount, got step %d", rm.current)
	}
	if len(rm.values) != 0 {
		t.Errorf("restart should clear values, got %d values", len(rm.values))
	}
}

func TestHandleChoiceResult_DoneRunAgain(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepDone,
		values:  map[stepID]string{stepRegion: "us-east-1"},
	}
	m.choiceComp = newChoiceModel("What next?", []string{"Run again", "Quit"})
	m.choiceComp.chosen = doneChoiceRunAgain

	result, _ := m.handleChoiceResult()
	rm := result.(tuiModel)

	if rm.current != stepAccount {
		t.Errorf("Run again should return to stepAccount, got step %d", rm.current)
	}
}

func TestHandleChoiceResult_DoneQuit(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepDone,
		values:  make(map[stepID]string),
	}
	m.choiceComp = newChoiceModel("What next?", []string{"Run again", "Quit"})
	m.choiceComp.chosen = doneChoiceQuit

	result, cmd := m.handleChoiceResult()
	rm := result.(tuiModel)

	if !rm.done {
		t.Error("Quit should set done=true")
	}
	if rm.quitting {
		t.Error("Quit should NOT set quitting=true (that's for ctrl+c)")
	}
	if cmd == nil {
		t.Error("Quit should return a tea.Cmd (tea.Quit)")
	}
}

// ---------------------------------------------------------------------------
// restart tests
// ---------------------------------------------------------------------------

func TestRestart_ClearsState(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	acc := testAccounts()[0]
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepConfirm,
		values:  map[stepID]string{stepEmail: "user@example.com", stepRegion: "us-east-1"},
		account: &acc,
		width:   200,
		height:  50,
	}

	nm := m.restart()

	if nm.current != stepAccount {
		t.Errorf("restart should return to stepAccount, got step %d", nm.current)
	}
	if len(nm.values) != 0 {
		t.Errorf("restart should clear values, got %d", len(nm.values))
	}
	if nm.account != nil {
		t.Error("restart should clear account")
	}
	// Terminal dimensions should be preserved
	if nm.width != 200 {
		t.Errorf("restart should preserve width: want 200, got %d", nm.width)
	}
	if nm.height != 50 {
		t.Errorf("restart should preserve height: want 50, got %d", nm.height)
	}
}

func TestRestart_ProducesValidModel(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)
	// Simulate having gone through the wizard
	m.values[stepEmail] = "user@example.com"
	m.current = stepDone
	// Simulate real terminal size
	m.width = 160
	m.height = 40

	nm := m.restart()

	// Should be a fresh model at the first interactive step
	if nm.current != stepAccount {
		t.Errorf("restart model should start at stepAccount, got %d", nm.current)
	}
	if nm.compType != "select" {
		t.Errorf("restart model should have select component, got %q", nm.compType)
	}
	// Terminal dimensions should be preserved
	if nm.width != 160 {
		t.Errorf("restart should preserve width: want 160, got %d", nm.width)
	}
	if nm.height != 40 {
		t.Errorf("restart should preserve height: want 40, got %d", nm.height)
	}
}

// ---------------------------------------------------------------------------
// ESC restart tests (via Update)
// ---------------------------------------------------------------------------

func TestESC_RestartFromSelect(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)
	if m.compType != "select" {
		t.Skipf("expected select compType at start, got %q", m.compType)
	}
	// Simulate real terminal size
	m.width = 200
	m.height = 50

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := result.(tuiModel)

	if rm.current != stepAccount {
		t.Errorf("ESC should restart to stepAccount, got %d", rm.current)
	}
	// Terminal dimensions should be preserved across ESC restart
	if rm.width != 200 {
		t.Errorf("ESC restart should preserve width: want 200, got %d", rm.width)
	}
	if rm.height != 50 {
		t.Errorf("ESC restart should preserve height: want 50, got %d", rm.height)
	}
}

func TestESC_RestartFromInput(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil) // no accounts, will skip account
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepEmail,
		values:   make(map[stepID]string),
		compType: "input",
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := result.(tuiModel)

	// After ESC, should be at stepAccount of a fresh model
	if rm.current > stepAccount+1 {
		// It will skip account (no accounts) and land at role or further
		// The key assertion: it should be a fresh model, not at stepEmail
		if _, exists := rm.values[stepEmail]; exists {
			t.Error("ESC restart should produce a fresh model without old values")
		}
	}
}

func TestESC_RestartFromChoice(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepConfirm,
		values:   make(map[stepID]string),
		compType: "choice",
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := result.(tuiModel)

	if rm.current != stepAccount {
		t.Errorf("ESC from choice should restart: got step %d", rm.current)
	}
}

func TestESC_RestartFromAwaitKey(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepDone,
		values:   make(map[stepID]string),
		compType: "await-key",
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := result.(tuiModel)

	if rm.current != stepAccount {
		t.Errorf("ESC from await-key should restart: got step %d", rm.current)
	}
}

func TestESC_BlockedDuringSpinner(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepFetching,
		values:   make(map[stepID]string),
		compType: "spinner",
	}

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := result.(tuiModel)

	if rm.current != stepFetching {
		t.Errorf("ESC during spinner should be ignored: got step %d", rm.current)
	}
	if rm.compType != "spinner" {
		t.Errorf("compType should still be 'spinner', got %q", rm.compType)
	}
}

// ---------------------------------------------------------------------------
// ctrl+c tests
// ---------------------------------------------------------------------------

func TestCtrlC_SetsQuitting(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	rm := result.(tuiModel)

	if !rm.quitting {
		t.Error("ctrl+c should set quitting=true")
	}
	if rm.done {
		t.Error("ctrl+c should not set done=true")
	}
	if cmd == nil {
		t.Error("ctrl+c should return tea.Quit command")
	}
}

// ---------------------------------------------------------------------------
// credentialResultMsg handling
// ---------------------------------------------------------------------------

func TestUpdate_CredentialResultError(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepFetching,
		values:   make(map[stepID]string),
		compType: "spinner",
	}

	result, _ := m.Update(credentialResultMsg{err: &validationError{"auth failed"}})
	rm := result.(tuiModel)

	if rm.credErr == nil {
		t.Error("credErr should be set")
	}
	if rm.current != stepDone {
		t.Errorf("should transition to stepDone on error, got %d", rm.current)
	}
	if rm.compType != "choice" {
		t.Errorf("compType should be 'choice' (error menu) on credential error, got %q", rm.compType)
	}
	if rm.done {
		t.Error("done should be false on error — user must see the error")
	}
}

func TestUpdate_CredentialResultSuccess(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "cli")

	exp := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cred := aws.AwsSamlOutput{
		AccessKeyID:     "AKIA",
		SecretAccessKey: "SECRET",
		SessionToken:    "TOKEN",
		Region:          "us-east-1",
		Expiration:      &exp,
	}

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepFetching,
		values:   make(map[stepID]string),
		compType: "spinner",
	}

	result, cmd := m.Update(credentialResultMsg{cred: cred})
	rm := result.(tuiModel)

	if rm.credResult == nil {
		t.Error("credResult should be set")
	}
	if rm.credResult.AccessKeyID != "AKIA" {
		t.Errorf("credential access key: want %q, got %q", "AKIA", rm.credResult.AccessKeyID)
	}
	if cmd == nil {
		t.Error("should return a command for writeOutput")
	}
}

// ---------------------------------------------------------------------------
// outputResultMsg handling
// ---------------------------------------------------------------------------

func TestUpdate_OutputResultSuccess(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "cli")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepFetching,
		values:   make(map[stepID]string),
		compType: "spinner",
	}

	result, _ := m.Update(outputResultMsg{err: nil})
	rm := result.(tuiModel)

	if !rm.outputDone {
		t.Error("outputDone should be true")
	}
	if rm.current != stepDone {
		t.Errorf("should transition to stepDone, got %d", rm.current)
	}
}

func TestUpdate_OutputResultWithText(t *testing.T) {
	// With the stdout refactor, outputResultMsg no longer carries text.
	// Stdout formats now always defer to post-TUI and immediately quit.
	resetViper()
	// outputFormat not explicitly set — interactive selection

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepFetching,
		values:   map[stepID]string{stepOutputFormat: "env-stdout"},
		compType: "spinner",
	}

	result, cmd := m.Update(outputResultMsg{})
	rm := result.(tuiModel)

	if !rm.outputDone {
		t.Error("outputDone should be true")
	}
	// For stdout formats, initStep sets done=true for immediate quit
	if !rm.done {
		t.Error("done should be true for stdout format")
	}
	if cmd == nil {
		t.Error("should return tea.Quit command for stdout format")
	}
}

func TestUpdate_OutputResultError(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepFetching,
		values:   make(map[stepID]string),
		compType: "spinner",
	}

	result, _ := m.Update(outputResultMsg{err: &validationError{"write failed"}})
	rm := result.(tuiModel)

	if rm.outputErr == nil {
		t.Error("outputErr should be set")
	}
	if !rm.outputDone {
		t.Error("outputDone should be true even on error")
	}
	if rm.compType != "choice" {
		t.Errorf("compType should be 'choice' (error menu) on output error, got %q", rm.compType)
	}
	if rm.done {
		t.Error("done should be false on output error — user must see the error")
	}
}

func TestInitStep_DoneErrorIgnoresTUIDoneAction(t *testing.T) {
	// Even with tui_done_action=exit, errors must show the menu.
	for _, action := range []string{"", "exit"} {
		name := "default"
		if action != "" {
			name = action
		}
		t.Run(name, func(t *testing.T) {
			resetViper()
			viper.Set(keyOutputFormat, "cli")
			if action != "" {
				viper.Set(keyTUIDoneAction, action)
			}

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:  cfg,
				steps:   allStepMeta(),
				current: stepDone,
				values:  make(map[stepID]string),
				credErr: &validationError{"auth failed"},
			}
			m.initStep()

			if m.compType != "choice" {
				t.Errorf("compType should be 'choice' on error regardless of tui_done_action=%q, got %q", action, m.compType)
			}
			if m.done {
				t.Errorf("done should be false on error regardless of tui_done_action=%q", action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// preResolveSteps tests
// ---------------------------------------------------------------------------

func TestPreResolveSteps_WithFullAccount(t *testing.T) {
	resetViper()

	acc := testAccounts()[0] // prod: has email, password, mfa, idpURL, principalARN, awsCliProfile
	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	// Email should be pre-resolved from account
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email source: want %q, got %q", sourcePreset, stepSrc(m, stepEmail))
	}
	if stepValue(m, stepEmail) != "prod@example.com" {
		t.Errorf("email value: want %q, got %q", "prod@example.com", stepValue(m, stepEmail))
	}

	// Password should be pre-resolved from account
	if stepSrc(m, stepPassword) != sourcePreset {
		t.Errorf("password source: want %q, got %q", sourcePreset, stepSrc(m, stepPassword))
	}
	if stepValue(m, stepPassword) != "(set)" {
		t.Errorf("password value: want %q, got %q", "(set)", stepValue(m, stepPassword))
	}

	// IDP URL
	if stepSrc(m, stepIdpURL) != sourcePreset {
		t.Errorf("idpURL source: want %q, got %q", sourcePreset, stepSrc(m, stepIdpURL))
	}

	// Principal ARN
	if stepSrc(m, stepPrincipalARN) != sourcePreset {
		t.Errorf("principalARN source: want %q, got %q", sourcePreset, stepSrc(m, stepPrincipalARN))
	}

	// MFA
	if stepSrc(m, stepMFA) != sourcePreset {
		t.Errorf("mfa source: want %q, got %q", sourcePreset, stepSrc(m, stepMFA))
	}
	if stepValue(m, stepMFA) != "(set)" {
		t.Errorf("mfa value: want %q, got %q", "(set)", stepValue(m, stepMFA))
	}

	// AWS CLI Profile — output format not set, so resolveOutputFormat returns ""
	// which is neither "cli" nor "cli-stdout", so profile should be "(n/a)"
	if stepSrc(m, stepAwsCliProfile) != sourcePreset {
		t.Errorf("awsCliProfile source: want %q, got %q", sourcePreset, stepSrc(m, stepAwsCliProfile))
	}

	// Region should be pre-resolved from account's AWSRegions
	// (resolveString for keyRegion doesn't fall back to account regions, so no preset unless Viper has it)
}

func TestPreResolveSteps_WithViperOnly(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "env")
	viper.Set(keyEmail, "viper@example.com")
	viper.Set(keyRegion, "eu-west-1")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil, // no account
	}

	m.preResolveSteps()

	// Output format should be pre-resolved from Viper
	if stepSrc(m, stepOutputFormat) != sourcePreset {
		t.Errorf("outputFormat source: want %q, got %q", sourcePreset, stepSrc(m, stepOutputFormat))
	}
	if stepValue(m, stepOutputFormat) != "env" {
		t.Errorf("outputFormat value: want %q, got %q", "env", stepValue(m, stepOutputFormat))
	}

	// Email from Viper
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email source: want %q, got %q", sourcePreset, stepSrc(m, stepEmail))
	}
	if stepValue(m, stepEmail) != "viper@example.com" {
		t.Errorf("email value: want %q, got %q", "viper@example.com", stepValue(m, stepEmail))
	}

	// Region from Viper
	if stepSrc(m, stepRegion) != sourcePreset {
		t.Errorf("region source: want %q, got %q", sourcePreset, stepSrc(m, stepRegion))
	}
	if stepValue(m, stepRegion) != "eu-west-1" {
		t.Errorf("region value: want %q, got %q", "eu-west-1", stepValue(m, stepRegion))
	}
}

func TestPreResolveSteps_NoAccount(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil,
	}

	m.preResolveSteps()

	// Nothing should be pre-resolved (no account, no Viper settings)
	for _, s := range m.steps {
		// AWS CLI Profile gets "(n/a)" because format is "" (not cli/cli-stdout)
		if s.id == stepAwsCliProfile {
			continue
		}
		if s.source == sourcePreset {
			t.Errorf("step %d (%s) should not be preset with no account and no Viper, got source=%q value=%q",
				s.id, s.title, s.source, s.value)
		}
	}
}

func TestPreResolveSteps_PartialAccount(t *testing.T) {
	resetViper()

	acc := config.Account{
		Name:  "partial",
		Email: "partial@example.com",
		// No password, no MFA, no IDP URL, no principal ARN
	}
	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	// Email should be pre-resolved
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email should be preset, got source=%q", stepSrc(m, stepEmail))
	}
	if stepValue(m, stepEmail) != "partial@example.com" {
		t.Errorf("email value: want %q, got %q", "partial@example.com", stepValue(m, stepEmail))
	}

	// Password should NOT be pre-resolved
	if stepSrc(m, stepPassword) == sourcePreset {
		t.Error("password should not be preset for partial account")
	}

	// MFA should NOT be pre-resolved
	if stepSrc(m, stepMFA) == sourcePreset {
		t.Error("mfa should not be preset for partial account")
	}

	// IDP URL should NOT be pre-resolved
	if stepSrc(m, stepIdpURL) == sourcePreset {
		t.Error("idpURL should not be preset for partial account")
	}

	// Principal ARN should NOT be pre-resolved
	if stepSrc(m, stepPrincipalARN) == sourcePreset {
		t.Error("principalARN should not be preset for partial account")
	}
}

func TestPreResolveSteps_RoleARNFromViper(t *testing.T) {
	resetViper()
	viper.Set(keyRoleARN, "arn:aws:iam::111:role/direct")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil,
	}

	m.preResolveSteps()

	if stepSrc(m, stepRole) != sourcePreset {
		t.Errorf("role source: want %q, got %q", sourcePreset, stepSrc(m, stepRole))
	}
	if stepValue(m, stepRole) != "arn:aws:iam::111:role/direct" {
		t.Errorf("role value: want ARN, got %q", stepValue(m, stepRole))
	}
}

func TestPreResolveSteps_RoleNameFromViper(t *testing.T) {
	resetViper()
	viper.Set(keyRoleName, "admin")

	acc := testAccounts()[0] // has admin role
	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	if stepSrc(m, stepRole) != sourcePreset {
		t.Errorf("role source: want %q, got %q", sourcePreset, stepSrc(m, stepRole))
	}
	if stepValue(m, stepRole) != "admin" {
		t.Errorf("role display: want %q, got %q", "admin", stepValue(m, stepRole))
	}
}

func TestPreResolveSteps_AwsCliProfileForCliFormat(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "cli")

	acc := config.Account{Name: "test", AwsCliProfile: "my-profile"}
	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	if stepSrc(m, stepAwsCliProfile) != sourcePreset {
		t.Errorf("awsCliProfile source: want %q, got %q", sourcePreset, stepSrc(m, stepAwsCliProfile))
	}
	if stepValue(m, stepAwsCliProfile) != "my-profile" {
		t.Errorf("awsCliProfile value: want %q, got %q", "my-profile", stepValue(m, stepAwsCliProfile))
	}
}

func TestPreResolveSteps_AwsCliProfileNAForNonCliFormat(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "env")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil,
	}

	m.preResolveSteps()

	if stepSrc(m, stepAwsCliProfile) != sourcePreset {
		t.Errorf("awsCliProfile source: want %q, got %q", sourcePreset, stepSrc(m, stepAwsCliProfile))
	}
	if stepValue(m, stepAwsCliProfile) != "(n/a)" {
		t.Errorf("awsCliProfile value: want %q, got %q", "(n/a)", stepValue(m, stepAwsCliProfile))
	}
}

func TestPreResolveSteps_CalledFromNewTuiModel(t *testing.T) {
	resetViper()
	viper.Set(keyAccount, "prod")

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	// After account preset, preResolveSteps should have pre-resolved
	// account fields from the prod account
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email should be preset after newTuiModel with account preset, got %q", stepSrc(m, stepEmail))
	}
	if stepSrc(m, stepPassword) != sourcePreset {
		t.Errorf("password should be preset after newTuiModel with account preset, got %q", stepSrc(m, stepPassword))
	}
	if stepSrc(m, stepMFA) != sourcePreset {
		t.Errorf("mfa should be preset after newTuiModel with account preset, got %q", stepSrc(m, stepMFA))
	}
}

func TestPreResolveSteps_CalledFromHandleSelectResult(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
	}

	m.handleSelectResult(selectItem{name: "prod"})

	// After interactive account selection, preResolveSteps should have
	// pre-resolved the account fields
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email should be preset after account selection, got source=%q", stepSrc(m, stepEmail))
	}
	if stepSrc(m, stepPassword) != sourcePreset {
		t.Errorf("password should be preset after account selection, got source=%q", stepSrc(m, stepPassword))
	}
}

// ---------------------------------------------------------------------------
// preResolveSteps tests
// ---------------------------------------------------------------------------

func TestPreResolveSteps_WithFullAccount(t *testing.T) {
	resetViper()

	acc := testAccounts()[0] // prod: has email, password, mfa, idpURL, principalARN, awsCliProfile
	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	// Email should be pre-resolved from account
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email source: want %q, got %q", sourcePreset, stepSrc(m, stepEmail))
	}
	if stepValue(m, stepEmail) != "prod@example.com" {
		t.Errorf("email value: want %q, got %q", "prod@example.com", stepValue(m, stepEmail))
	}

	// Password should be pre-resolved from account
	if stepSrc(m, stepPassword) != sourcePreset {
		t.Errorf("password source: want %q, got %q", sourcePreset, stepSrc(m, stepPassword))
	}
	if stepValue(m, stepPassword) != "(set)" {
		t.Errorf("password value: want %q, got %q", "(set)", stepValue(m, stepPassword))
	}

	// IDP URL
	if stepSrc(m, stepIdpURL) != sourcePreset {
		t.Errorf("idpURL source: want %q, got %q", sourcePreset, stepSrc(m, stepIdpURL))
	}

	// Principal ARN
	if stepSrc(m, stepPrincipalARN) != sourcePreset {
		t.Errorf("principalARN source: want %q, got %q", sourcePreset, stepSrc(m, stepPrincipalARN))
	}

	// MFA
	if stepSrc(m, stepMFA) != sourcePreset {
		t.Errorf("mfa source: want %q, got %q", sourcePreset, stepSrc(m, stepMFA))
	}
	if stepValue(m, stepMFA) != "(set)" {
		t.Errorf("mfa value: want %q, got %q", "(set)", stepValue(m, stepMFA))
	}

	// AWS CLI Profile — output format not set, so resolveOutputFormat returns ""
	// which is neither "cli" nor "cli-stdout", so profile should be "(n/a)"
	if stepSrc(m, stepAwsCliProfile) != sourcePreset {
		t.Errorf("awsCliProfile source: want %q, got %q", sourcePreset, stepSrc(m, stepAwsCliProfile))
	}

	// Region should be pre-resolved from account's AWSRegions
	// (resolveString for keyRegion doesn't fall back to account regions, so no preset unless Viper has it)
}

func TestPreResolveSteps_WithViperOnly(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "env")
	viper.Set(keyEmail, "viper@example.com")
	viper.Set(keyRegion, "eu-west-1")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil, // no account
	}

	m.preResolveSteps()

	// Output format should be pre-resolved from Viper
	if stepSrc(m, stepOutputFormat) != sourcePreset {
		t.Errorf("outputFormat source: want %q, got %q", sourcePreset, stepSrc(m, stepOutputFormat))
	}
	if stepValue(m, stepOutputFormat) != "env" {
		t.Errorf("outputFormat value: want %q, got %q", "env", stepValue(m, stepOutputFormat))
	}

	// Email from Viper
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email source: want %q, got %q", sourcePreset, stepSrc(m, stepEmail))
	}
	if stepValue(m, stepEmail) != "viper@example.com" {
		t.Errorf("email value: want %q, got %q", "viper@example.com", stepValue(m, stepEmail))
	}

	// Region from Viper
	if stepSrc(m, stepRegion) != sourcePreset {
		t.Errorf("region source: want %q, got %q", sourcePreset, stepSrc(m, stepRegion))
	}
	if stepValue(m, stepRegion) != "eu-west-1" {
		t.Errorf("region value: want %q, got %q", "eu-west-1", stepValue(m, stepRegion))
	}
}

func TestPreResolveSteps_NoAccount(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil,
	}

	m.preResolveSteps()

	// Nothing should be pre-resolved (no account, no Viper settings)
	for _, s := range m.steps {
		// AWS CLI Profile gets "(n/a)" because format is "" (not cli/cli-stdout)
		if s.id == stepAwsCliProfile {
			continue
		}
		if s.source == sourcePreset {
			t.Errorf("step %d (%s) should not be preset with no account and no Viper, got source=%q value=%q",
				s.id, s.title, s.source, s.value)
		}
	}
}

func TestPreResolveSteps_PartialAccount(t *testing.T) {
	resetViper()

	acc := config.Account{
		Name:  "partial",
		Email: "partial@example.com",
		// No password, no MFA, no IDP URL, no principal ARN
	}
	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	// Email should be pre-resolved
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email should be preset, got source=%q", stepSrc(m, stepEmail))
	}
	if stepValue(m, stepEmail) != "partial@example.com" {
		t.Errorf("email value: want %q, got %q", "partial@example.com", stepValue(m, stepEmail))
	}

	// Password should NOT be pre-resolved
	if stepSrc(m, stepPassword) == sourcePreset {
		t.Error("password should not be preset for partial account")
	}

	// MFA should NOT be pre-resolved
	if stepSrc(m, stepMFA) == sourcePreset {
		t.Error("mfa should not be preset for partial account")
	}

	// IDP URL should NOT be pre-resolved
	if stepSrc(m, stepIdpURL) == sourcePreset {
		t.Error("idpURL should not be preset for partial account")
	}

	// Principal ARN should NOT be pre-resolved
	if stepSrc(m, stepPrincipalARN) == sourcePreset {
		t.Error("principalARN should not be preset for partial account")
	}
}

func TestPreResolveSteps_RoleARNFromViper(t *testing.T) {
	resetViper()
	viper.Set(keyRoleARN, "arn:aws:iam::111:role/direct")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil,
	}

	m.preResolveSteps()

	if stepSrc(m, stepRole) != sourcePreset {
		t.Errorf("role source: want %q, got %q", sourcePreset, stepSrc(m, stepRole))
	}
	if stepValue(m, stepRole) != "arn:aws:iam::111:role/direct" {
		t.Errorf("role value: want ARN, got %q", stepValue(m, stepRole))
	}
}

func TestPreResolveSteps_RoleNameFromViper(t *testing.T) {
	resetViper()
	viper.Set(keyRoleName, "admin")

	acc := testAccounts()[0] // has admin role
	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	if stepSrc(m, stepRole) != sourcePreset {
		t.Errorf("role source: want %q, got %q", sourcePreset, stepSrc(m, stepRole))
	}
	if stepValue(m, stepRole) != "admin" {
		t.Errorf("role display: want %q, got %q", "admin", stepValue(m, stepRole))
	}
}

func TestPreResolveSteps_AwsCliProfileForCliFormat(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "cli")

	acc := config.Account{Name: "test", AwsCliProfile: "my-profile"}
	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: &acc,
	}

	m.preResolveSteps()

	if stepSrc(m, stepAwsCliProfile) != sourcePreset {
		t.Errorf("awsCliProfile source: want %q, got %q", sourcePreset, stepSrc(m, stepAwsCliProfile))
	}
	if stepValue(m, stepAwsCliProfile) != "my-profile" {
		t.Errorf("awsCliProfile value: want %q, got %q", "my-profile", stepValue(m, stepAwsCliProfile))
	}
}

func TestPreResolveSteps_AwsCliProfileNAForNonCliFormat(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "env")

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
		account: nil,
	}

	m.preResolveSteps()

	if stepSrc(m, stepAwsCliProfile) != sourcePreset {
		t.Errorf("awsCliProfile source: want %q, got %q", sourcePreset, stepSrc(m, stepAwsCliProfile))
	}
	if stepValue(m, stepAwsCliProfile) != "(n/a)" {
		t.Errorf("awsCliProfile value: want %q, got %q", "(n/a)", stepValue(m, stepAwsCliProfile))
	}
}

func TestPreResolveSteps_CalledFromNewTuiModel(t *testing.T) {
	resetViper()
	viper.Set(keyAccount, "prod")

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	// After account preset, preResolveSteps should have pre-resolved
	// account fields from the prod account
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email should be preset after newTuiModel with account preset, got %q", stepSrc(m, stepEmail))
	}
	if stepSrc(m, stepPassword) != sourcePreset {
		t.Errorf("password should be preset after newTuiModel with account preset, got %q", stepSrc(m, stepPassword))
	}
	if stepSrc(m, stepMFA) != sourcePreset {
		t.Errorf("mfa should be preset after newTuiModel with account preset, got %q", stepSrc(m, stepMFA))
	}
}

func TestPreResolveSteps_CalledFromHandleSelectResult(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:  cfg,
		steps:   allStepMeta(),
		current: stepAccount,
		values:  make(map[stepID]string),
	}

	m.handleSelectResult(selectItem{name: "prod"})

	// After interactive account selection, preResolveSteps should have
	// pre-resolved the account fields
	if stepSrc(m, stepEmail) != sourcePreset {
		t.Errorf("email should be preset after account selection, got source=%q", stepSrc(m, stepEmail))
	}
	if stepSrc(m, stepPassword) != sourcePreset {
		t.Errorf("password should be preset after account selection, got source=%q", stepSrc(m, stepPassword))
	}
}

// ---------------------------------------------------------------------------
// writeOutput tests
// ---------------------------------------------------------------------------

func TestWriteOutput_ShellReturnsEmptyResult(t *testing.T) {
	resetViper()
	viper.Set(keyOutputFormat, "shell")

	exp := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cred := &aws.AwsSamlOutput{
		AccessKeyID: "AKIA", SecretAccessKey: "SECRET",
		SessionToken: "TOKEN", Region: "us-east-1", Expiration: &exp,
	}

	cfg := newTestConfig(nil)
	m := tuiModel{appCfg: cfg, credResult: cred, values: make(map[stepID]string)}
	cmd := m.writeOutput()
	msg := cmd()

	result, ok := msg.(outputResultMsg)
	if !ok {
		t.Fatalf("expected outputResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Errorf("shell writeOutput should not error: %v", result.err)
	}
}

func TestWriteOutput_PresetStdoutReturnsEmptyResult(t *testing.T) {
	exp := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cred := &aws.AwsSamlOutput{
		AccessKeyID: "AKIA", SecretAccessKey: "SECRET",
		SessionToken: "TOKEN", Region: "us-east-1", Expiration: &exp,
	}

	for _, format := range []string{"cli-stdout", "env-stdout"} {
		t.Run(format, func(t *testing.T) {
			resetViper()
			viper.Set(keyOutputFormat, format)

			cfg := newTestConfig(nil)
			m := tuiModel{appCfg: cfg, credResult: cred, values: make(map[stepID]string)}
			cmd := m.writeOutput()
			msg := cmd()

			result, ok := msg.(outputResultMsg)
			if !ok {
				t.Fatalf("expected outputResultMsg, got %T", msg)
			}
			if result.err != nil {
				t.Errorf("preset stdout should not error: %v", result.err)
			}
		})
	}
}

func TestWriteOutput_InteractiveStdoutDefersToPostTUI(t *testing.T) {
	// After the stdout refactor, interactive stdout is handled the same as
	// preset stdout: deferred to post-TUI for real stdout printing.
	exp := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cred := &aws.AwsSamlOutput{
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "SECRET",
		SessionToken: "TOKEN", Region: "us-east-1", Expiration: &exp,
	}

	for _, format := range []string{"cli-stdout", "env-stdout"} {
		t.Run(format, func(t *testing.T) {
			resetViper()
			// outputFormat not explicitly set — interactive
			// awsCliProfile set via Viper
			viper.Set(keyAwsCliProfile, "test-profile")

			cfg := newTestConfig(nil)
			m := tuiModel{
				appCfg:     cfg,
				credResult: cred,
				values:     map[stepID]string{stepOutputFormat: format},
			}
			cmd := m.writeOutput()
			msg := cmd()

			result, ok := msg.(outputResultMsg)
			if !ok {
				t.Fatalf("expected outputResultMsg, got %T", msg)
			}
			if result.err != nil {
				t.Errorf("interactive stdout should not error: %v", result.err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// await-key compType tests
// ---------------------------------------------------------------------------

func TestAwaitKey_AnyKeypressExits(t *testing.T) {
	resetViper()

	cfg := newTestConfig(nil)
	m := tuiModel{
		appCfg:   cfg,
		steps:    allStepMeta(),
		current:  stepDone,
		values:   make(map[stepID]string),
		compType: "await-key",
	}

	// Send a regular keypress (not ESC, which restarts)
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := result.(tuiModel)

	if !rm.done {
		t.Error("any keypress in await-key should set done=true")
	}
	if cmd == nil {
		t.Error("should return tea.Quit command")
	}
}

// ---------------------------------------------------------------------------
// WindowSizeMsg tests
// ---------------------------------------------------------------------------

func TestUpdate_WindowSizeMsg(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	rm := result.(tuiModel)

	if rm.width != 120 {
		t.Errorf("width: want 120, got %d", rm.width)
	}
	if rm.height != 40 {
		t.Errorf("height: want 40, got %d", rm.height)
	}
}

// ---------------------------------------------------------------------------
// newTuiModel tests
// ---------------------------------------------------------------------------

func TestNewTuiModel_Defaults(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	if m.appCfg != cfg {
		t.Error("appCfg should be set")
	}
	if m.values == nil {
		t.Error("values should be initialized")
	}
	if m.quitting {
		t.Error("quitting should be false")
	}
	if m.done {
		t.Error("done should be false")
	}
	if m.width != 80 {
		t.Errorf("default width: want 80, got %d", m.width)
	}
	if m.height != 24 {
		t.Errorf("default height: want 24, got %d", m.height)
	}
}

func TestNewTuiModel_AllPresetsSkipToConfirm(t *testing.T) {
	resetViper()
	viper.Set(keyAccount, "prod")
	viper.Set(keyRoleARN, "arn:aws:iam::111:role/admin")
	viper.Set(keyRegion, "us-east-1")
	viper.Set(keyEmail, "user@example.com")
	viper.Set(keyPassword, "pass")
	viper.Set(keyIdpURL, "https://sso.jumpcloud.com/saml2/test")
	viper.Set(keyPrincipalARN, "arn:aws:iam::111:saml-provider/jc")
	viper.Set(keyOutputFormat, "env")
	viper.Set(keyMFA, "123456")

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	if m.current != stepConfirm {
		t.Errorf("with all presets, should land at stepConfirm, got step %d", m.current)
	}
	if m.compType != "choice" {
		t.Errorf("compType at confirm should be 'choice', got %q", m.compType)
	}
}

// ---------------------------------------------------------------------------
// Update check conditional tests
// ---------------------------------------------------------------------------

func TestUpdateCheckMsg_SetsUpdateVersion(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	result, _ := m.Update(updateCheckMsg{latestVersion: "1.2.3"})
	rm := result.(tuiModel)

	if rm.updateVersion != "1.2.3" {
		t.Errorf("updateVersion: want %q, got %q", "1.2.3", rm.updateVersion)
	}
}

func TestUpdateCheckMsg_EmptyDoesNothing(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)

	result, _ := m.Update(updateCheckMsg{latestVersion: ""})
	rm := result.(tuiModel)

	if rm.updateVersion != "" {
		t.Errorf("updateVersion should be empty, got %q", rm.updateVersion)
	}
}

func TestInit_NoUpdateCheckSkipsUpdateCmd(t *testing.T) {
	resetViper()
	viper.Set(keyNoUpdateCheck, true)

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)
	cmd := m.Init()

	// Execute all batched commands and collect messages.
	// With noUpdateCheck=true, none of the messages should be updateCheckMsg.
	if cmd == nil {
		t.Fatal("Init should return a command even with noUpdateCheck")
	}

	// We cannot easily inspect tea.Batch internals, but we can verify
	// the model's Init still works without panicking.
	// The structural test: Init with noUpdateCheck=true should still
	// return a non-nil Cmd (WindowSize + initCmd).
}

func TestInit_UpdateCheckIncludedByDefault(t *testing.T) {
	resetViper()
	// noUpdateCheck defaults to false

	cfg := newTestConfig(testAccounts())
	m := newTuiModel(cfg)
	cmd := m.Init()

	if cmd == nil {
		t.Fatal("Init should return a command")
	}
}

func TestRestart_PreservesUpdateVersion(t *testing.T) {
	resetViper()

	cfg := newTestConfig(testAccounts())
	m := tuiModel{
		appCfg:        cfg,
		steps:         allStepMeta(),
		current:       stepConfirm,
		values:        map[stepID]string{stepEmail: "user@example.com"},
		updateVersion: "2.0.0",
		width:         100,
		height:        40,
	}

	nm := m.restart()

	if nm.updateVersion != "2.0.0" {
		t.Errorf("restart should preserve updateVersion: want %q, got %q", "2.0.0", nm.updateVersion)
	}
}
