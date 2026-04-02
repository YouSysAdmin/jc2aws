package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/config"
	"github.com/yousysadmin/jc2aws/pkg"
	"github.com/yousysadmin/jc2aws/pkg/update"
)

// appConfig holds resolved configuration and the parsed config file.
// Simple flags (interactive, update, shellScript, configFilePath) live here
// because they don't participate in the account-defaults resolution.
// All other values (email, password, region, etc.) are read from Viper
// with account-level fallback.
type appConfig struct {
	configFilePath string
	shellScript    string
	interactive    bool
	update         bool

	config *config.Config
}

// ---------------------------------------------------------------------------
// Viper key constants
// ---------------------------------------------------------------------------

const (
	keyEmail         = "email"
	keyPassword      = "password"
	keyMFA           = "mfa"
	keyIdpURL        = "idp-url"
	keyRoleName      = "role-name"
	keyRoleARN       = "role-arn"
	keyPrincipalARN  = "principal-arn"
	keyRegion        = "region"
	keyDuration      = "duration"
	keyAccount       = "account"
	keyOutputFormat  = "output-format"
	keyAwsCliProfile = "aws-cli-profile-name"
	keyNoUpdateCheck = "no-update-check"
	keyShell         = "shell"
	keyShellScript   = "shell-script"
	keyInteractive   = "interactive"
	keyConfig        = "config"
	keyTUIDoneAction = "tui-done-action"
)

// ---------------------------------------------------------------------------
// Account-aware value resolution
// ---------------------------------------------------------------------------

// resolveString returns the Viper value for the given key if it was explicitly
// set (flag, env var, or config-file default).
// Otherwise, it falls back to the account value.
// This keeps account defaults at the lowest priority without mutating Viper state.
func resolveString(key string, acc *config.Account) string {
	if viper.IsSet(key) {
		return viper.GetString(key)
	}
	if acc == nil {
		return ""
	}
	switch key {
	case keyEmail:
		return acc.Email
	case keyPassword:
		return acc.Password
	case keyMFA:
		return acc.MFASecret
	case keyIdpURL:
		return acc.IdpURL
	case keyPrincipalARN:
		return acc.AWSPrincipalArn
	case keyAwsCliProfile:
		if acc.AwsCliProfile != "" {
			return acc.AwsCliProfile
		}
		return acc.Name
	}
	return ""
}

// defaultDuration default credential duration in seconds.
const defaultDuration = 3600

// resolveDuration returns the duration from user provided via flags if set, otherwise
// falls back to the account's duration if set, then the default.
func resolveDuration(acc *config.Account) int {
	if viper.IsSet(keyDuration) {
		return viper.GetInt(keyDuration)
	}
	if acc != nil && acc.Duration != 0 {
		return acc.Duration
	}
	if d := viper.GetInt(keyDuration); d != 0 {
		return d
	}
	return defaultDuration
}

// ---------------------------------------------------------------------------
// CLI Entrypoint
// ---------------------------------------------------------------------------

func main() {
	cfg := &appConfig{
		config: &config.Config{},
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to determine home directory: %v\n", err)
		os.Exit(1)
	}
	cfg.configFilePath = filepath.Join(homeDir, config.DefaultConfigFileName)

	// Viper setup
	viper.SetEnvPrefix("J2A")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Explicit env var bindings for names that don't match the flag -> env
	// for backward compatibly.
	viper.BindEnv(keyRegion, "J2A_REGION", "J2A_AWS_REGION")
	viper.BindEnv(keyConfig, "J2A_CONFIG")

	rootCmd := &cobra.Command{
		Use:     filepath.Base(os.Args[0]), //"jc2aws-tui",
		Short:   "Get AWS credentials via JumpCloud SSO",
		Long:    "Obtaining temporary AWS credentials via JumpCloud SAML authentication.",
		Version: pkg.Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Get config file path from Viper
			cfg.configFilePath = viper.GetString(keyConfig)

			cfgFile, err := config.NewConfig(cfg.configFilePath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					fmt.Fprintf(os.Stderr, "Warning: Config file %s not found\n", cfg.configFilePath)
					return nil
				}
				return fmt.Errorf("failed to load config file %s: %w", cfg.configFilePath, err)
			}
			cfg.config = cfgFile

			// NOTE: Do NOT use `viper.SetDefault` for `output-format` and `duration`.
			// Cobra flag defaults (set via `flags.StringP` / `flags.IntP`) are
			// picked up by `viper.BindPFlags` but do NOT make `viper.IsSet()` true.

			// Feed config-file top-level defaults into Viper via Set so they
			// make viper.IsSet() return true and take priority over account
			// defaults in resolveString/resolveDuration. The !IsSet guard
			// ensures flags and env vars (higher priority) are not overwritten.
			if cfgFile.DefaultEmail != "" && !viper.IsSet(keyEmail) {
				viper.Set(keyEmail, cfgFile.DefaultEmail)
			}
			if cfgFile.DefaultPassword != "" && !viper.IsSet(keyPassword) {
				viper.Set(keyPassword, cfgFile.DefaultPassword)
			}
			if cfgFile.DefaultMFATokenSecret != "" && !viper.IsSet(keyMFA) {
				viper.Set(keyMFA, cfgFile.DefaultMFATokenSecret)
			}
			if cfgFile.NoUpdateCheck && !viper.IsSet(keyNoUpdateCheck) {
				viper.Set(keyNoUpdateCheck, true)
			}
			if cfgFile.DefaultFormat != "" && !viper.IsSet(keyOutputFormat) {
				viper.Set(keyOutputFormat, cfgFile.DefaultFormat)
			}
			if cfgFile.TUIDoneAction != "" && !viper.IsSet(keyTUIDoneAction) {
				viper.Set(keyTUIDoneAction, cfgFile.TUIDoneAction)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.update {
				return update.DownloadAndReplace(pkg.Version, os.Stdout)
			}

			// -s / --shell is a convenience alias for --output-format=shell
			if cmd.Flags().Changed(keyShell) {
				viper.Set(keyOutputFormat, "shell")
			}

			// --shell-script implies shell output format
			if cmd.Flags().Changed(keyShellScript) {
				cfg.shellScript = viper.GetString(keyShellScript)
				viper.Set(keyOutputFormat, "shell")
			}

			// J2A_SHELL env var: treat as output-format=shell (backward compat)
			if v := os.Getenv("J2A_SHELL"); v == "true" || v == "1" {
				viper.Set(keyOutputFormat, "shell")
			}

			// J2A_SHELL_SCRIPT env var: set shell script path (implies shell format)
			if v := os.Getenv("J2A_SHELL_SCRIPT"); v != "" {
				cfg.shellScript = v
				viper.Set(keyOutputFormat, "shell")
			}

			cfg.interactive = viper.GetBool(keyInteractive)

			if cfg.interactive {
				return runInteractive(cfg)
			}
			return runHeadless(cfg)
		},
	}

	flags := rootCmd.Flags()
	flags.StringVarP(&cfg.configFilePath, keyConfig, "c", cfg.configFilePath, "Path to config file")
	flags.StringP(keyEmail, "e", "", "JumpCloud user email")
	flags.StringP(keyPassword, "p", "", "JumpCloud user password")
	flags.StringP(keyMFA, "m", "", "JumpCloud MFA token or secret")
	flags.String(keyIdpURL, "", "JumpCloud IDP URL")
	flags.String(keyRoleName, "", "AWS Role name (from config)")
	flags.String(keyRoleARN, "", "AWS Role ARN")
	flags.String(keyPrincipalARN, "", "AWS Identity provider ARN")
	flags.StringP(keyRegion, "r", "", "AWS region")
	flags.IntP(keyDuration, "d", 3600, "AWS credential expiration time in seconds")
	flags.StringP(keyAccount, "a", "", "Account name from config")
	flags.StringP(keyOutputFormat, "f", "cli", "Credential output format (cli, env, cli-stdout, env-stdout, shell)")
	flags.String(keyAwsCliProfile, "", "AWS CLI profile name")

	// -s / --shell is a convenience alias for --output-format=shell (backward compat).
	flags.BoolP(keyShell, "s", false, "Launch a shell with AWS credentials (alias for -f shell)")
	flags.String(keyShellScript, "", "Path to shell script to run with AWS credentials (implies -s)")
	flags.BoolP(keyInteractive, "i", false, "Launch interactive TUI wizard")
	flags.BoolVar(&cfg.update, "update", false, "Download and install the latest release")
	flags.Bool(keyNoUpdateCheck, false, "Disable automatic update check")

	// Bind all flags to Viper
	viper.BindPFlags(flags)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runInteractive launches the interactive TUI wizard.
func runInteractive(cfg *appConfig) error {
	m := newTuiModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("Error: %w", err)
	}

	fm, ok := finalModel.(tuiModel)
	if !ok {
		return nil
	}

	// ctrl+c abort — exit immediately, no shell, no error
	if fm.quitting {
		return nil
	}

	// Credential error — report it
	if fm.credErr != nil {
		return fmt.Errorf("credential error: %w", fm.credErr)
	}

	if fm.credResult == nil {
		return nil
	}

	format := fm.resolveOutputFormat()
	profileName := firstNonEmpty(
		resolveString(keyAwsCliProfile, fm.account),
		fm.values[stepAwsCliProfile],
	)

	// Shell: launch interactive shell with credential env vars
	if format == "shell" {
		return launchShell(*fm.credResult, cfg.shellScript)
	}

	// Stdout formats: output was deferred to post-TUI for real stdout
	if format == "cli-stdout" || format == "env-stdout" {
		return outputCredentials(*fm.credResult, format, profileName)
	}

	return nil
}

// runHeadless use CLI without launching the TUI.
// Values must be provided via flags, env vars or config file.
func runHeadless(cfg *appConfig) error {
	var acc *config.Account

	// Resolve account if --account is set
	accountName := viper.GetString(keyAccount)
	if accountName != "" {
		if len(cfg.config.Accounts) == 0 {
			return fmt.Errorf("--account flag can't be used without any pre-configured account")
		}
		found, err := cfg.config.FindAccountByName(accountName)
		if err != nil {
			return fmt.Errorf("account %q not found in config", accountName)
		}
		acc = &found
	}

	// Resolve all values (Viper flags/env take priority, then account defaults)
	email := resolveString(keyEmail, acc)
	password := resolveString(keyPassword, acc)
	idpURL := resolveString(keyIdpURL, acc)
	mfaToken := resolveString(keyMFA, acc)
	principalARN := resolveString(keyPrincipalARN, acc)
	roleARN := resolveString(keyRoleARN, acc)
	region := resolveString(keyRegion, acc)
	duration := resolveDuration(acc)
	awsCliProfile := resolveString(keyAwsCliProfile, acc)

	// Resolve --role-name to ARN if needed
	if roleARN == "" {
		roleName := viper.GetString(keyRoleName)
		if roleName != "" && acc != nil {
			role, err := acc.FindAWSRoleArnByName(roleName)
			if err != nil {
				return fmt.Errorf("role %q not found in account %q", roleName, accountName)
			}
			roleARN = role.Arn
		}
	}

	// Validate required fields
	required := []struct {
		value string
		flag  string
	}{
		{email, "--email"},
		{password, "--password"},
		{idpURL, "--idp-url"},
		{principalARN, "--principal-arn"},
		{roleARN, "--role-arn"},
		{region, "--region"},
	}
	for _, r := range required {
		if r.value == "" {
			return fmt.Errorf("%s is required (use -i for interactive mode)", r.flag)
		}
	}

	// Fetch credentials
	cred, err := getCredentials(email, password, idpURL, mfaToken, principalARN, roleARN, region, duration)
	if err != nil {
		return fmt.Errorf("credential error: %w", err)
	}

	// Handle output
	format := viper.GetString(keyOutputFormat)

	if format == "shell" {
		return launchShell(cred, cfg.shellScript)
	}

	return outputCredentials(cred, format, awsCliProfile)
}
