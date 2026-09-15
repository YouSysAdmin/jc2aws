package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/cloud"
	"github.com/yousysadmin/jc2aws/internal/cloud/providers"
	"github.com/yousysadmin/jc2aws/internal/config"
	"github.com/yousysadmin/jc2aws/internal/validators"
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
	keyEmail        = "email"
	keyPassword     = "password"
	keyMFA          = "mfa"
	keyIdpURL       = "idp-url"
	keyRoleName     = "role-name"
	keyRoleARN      = "role-arn"
	keyPrincipalARN = "principal-arn"
	keyRegion       = "region"
	keyProvider     = "provider"
	keyDuration     = "duration"
	keyAccount      = "account"
	keyOutputFormat = "output-format"
	keyCLIProfile   = "cli-profile-name"
	// keyAwsCliProfile is the deprecated spelling of keyCLIProfile.
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
		return acc.PrincipalARN
	case keyProvider:
		return acc.Provider
	case keyCLIProfile:
		if acc.CLIProfile != "" {
			return acc.CLIProfile
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
	return defaultDuration
}

// resolveProvider returns the cloud provider selected by flag, env var, or the
// account's `provider:` key, defaulting to AWS.
func resolveProvider(acc *config.Account) (cloud.Provider, error) {
	return providers.Get(resolveString(keyProvider, acc))
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
	if err := viper.BindEnv(keyRegion, "J2A_REGION", "J2A_AWS_REGION"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to bind env var for %s: %v\n", keyRegion, err)
		os.Exit(1)
	}
	if err := viper.BindEnv(keyConfig, "J2A_CONFIG"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to bind env var for %s: %v\n", keyConfig, err)
		os.Exit(1)
	}
	// The flag was renamed from --aws-cli-profile-name; without this the old
	// env var would silently stop being honoured, because AutomaticEnv derives
	// J2A_AWS_CLI_PROFILE_NAME from the deprecated key name alone.
	if err := viper.BindEnv(keyCLIProfile, "J2A_CLI_PROFILE_NAME", "J2A_AWS_CLI_PROFILE_NAME"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to bind env var for %s: %v\n", keyCLIProfile, err)
		os.Exit(1)
	}

	rootCmd := &cobra.Command{
		Use:          filepath.Base(os.Args[0]), //"jc2aws-tui",
		Short:        "Get cloud credentials via JumpCloud SSO",
		Long:         "Obtaining temporary AWS or Alibaba Cloud credentials via JumpCloud SAML authentication.",
		Version:      pkg.Version,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
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
				return update.DownloadAndReplace(cmd.Context(), pkg.Version, os.Stdout)
			}

			// J2A_SHELL env var: treat as output-format=shell (backward compat).
			// Explicit flags take priority over env vars, so an explicit
			// --output-format is never overridden here.
			if v := os.Getenv("J2A_SHELL"); (v == "true" || v == "1") && !cmd.Flags().Changed(keyOutputFormat) {
				viper.Set(keyOutputFormat, "shell")
			}

			// J2A_SHELL_SCRIPT env var: set shell script path (implies shell format)
			if v := os.Getenv("J2A_SHELL_SCRIPT"); v != "" {
				cfg.shellScript = v
				if !cmd.Flags().Changed(keyOutputFormat) {
					viper.Set(keyOutputFormat, "shell")
				}
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

			// Forward the deprecated profile flag before either mode reads it.
			if cmd.Flags().Changed(keyAwsCliProfile) && !cmd.Flags().Changed(keyCLIProfile) {
				viper.Set(keyCLIProfile, viper.GetString(keyAwsCliProfile))
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
	flags.String(keyRoleName, "", "Role name (from config)")
	flags.String(keyRoleARN, "", "Role ARN")
	flags.String(keyPrincipalARN, "", "SAML identity provider ARN (AWS principal ARN / Alibaba SAML provider ARN)")
	flags.StringP(keyRegion, "r", "", "Cloud region")
	flags.IntP(keyDuration, "d", defaultDuration, "Credential expiration time in seconds")
	flags.StringP(keyAccount, "a", "", "Account name from config")
	flags.StringP(keyOutputFormat, "f", "cli", "Credential output format (cli, env, cli-stdout, env-stdout, shell)")
	flags.String(keyProvider, "", "Cloud provider (aws, alibaba)")
	flags.String(keyCLIProfile, "", "Cloud CLI profile name")
	flags.String(keyAwsCliProfile, "", "Deprecated: use --cli-profile-name")

	// -s / --shell is a convenience alias for --output-format=shell (backward compat).
	flags.BoolP(keyShell, "s", false, "Launch a shell with cloud credentials (alias for -f shell)")
	flags.String(keyShellScript, "", "Path to shell script to run with cloud credentials (implies -s)")
	flags.BoolP(keyInteractive, "i", false, "Launch interactive TUI wizard")
	flags.BoolVar(&cfg.update, "update", false, "Download and install the latest release")
	flags.Bool(keyNoUpdateCheck, false, "Disable automatic update check")

	// Bind all flags to Viper
	if err := viper.BindPFlags(flags); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to bind flags: %v\n", err)
		os.Exit(1)
	}

	// Hide the deprecated flag from help only after binding, so the binding
	// itself still registers.
	if err := flags.MarkDeprecated(keyAwsCliProfile, "use --cli-profile-name"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to deprecate flag %s: %v\n", keyAwsCliProfile, err)
		os.Exit(1)
	}

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
		return err
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
	profileName := cmp.Or(
		resolveString(keyCLIProfile, fm.account),
		fm.values[stepCLIProfile],
	)

	// Stdout formats: the credentials themselves are the output and already make
	// success obvious, so no extra summary is printed.
	if format == "cli-stdout" || format == "env-stdout" {
		return outputCredentials(fm.provider, *fm.credResult, format, profileName)
	}

	summary := fm.accountInfoText()

	// Shell: summary to stderr (stdout belongs to the subshell), then launch.
	if format == "shell" {
		fmt.Fprint(os.Stderr, summary)
		return launchShell(fm.provider, *fm.credResult, cfg.shellScript)
	}

	// File-based formats (cli, env): files were already written inside the TUI.
	// Just print the summary. stdout is free.
	fmt.Fprint(os.Stdout, summary)
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
			return fmt.Errorf("failed to resolve --account: %w", err)
		}
		acc = &found
	}

	// Resolve the provider before anything else so an unknown --provider fails
	// immediately, rather than after a full authentication round-trip.
	prov, err := resolveProvider(acc)
	if err != nil {
		return err
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
	cliProfile := resolveString(keyCLIProfile, acc)

	// Resolve --role-name to ARN if needed
	if roleARN == "" {
		roleName := viper.GetString(keyRoleName)
		if roleName != "" {
			if acc == nil {
				return fmt.Errorf("--role-name requires --account to look the role up in")
			}
			role, err := acc.FindRoleByName(roleName)
			if err != nil {
				return fmt.Errorf("failed to resolve --role-name in account %q: %w", accountName, err)
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

	// Validate output format and region up front, before spending a full
	// authentication round-trip.
	format := viper.GetString(keyOutputFormat)
	if err := validators.Get(validators.KeyOutputFormat)(format); err != nil {
		return err
	}
	if err := validators.ProviderAware(validators.KeyRegion, prov)(region); err != nil {
		// The built-in region list can lag behind the provider; warn instead of blocking.
		fmt.Fprintf(os.Stderr, "Warning: region %q is not in the known %s region list; proceeding anyway\n",
			region, prov.Info().DisplayName)
	}

	// Fetch credentials
	cred, err := getCredentials(context.Background(), credentialRequest{
		Provider:     prov,
		Email:        email,
		Password:     password,
		IdpURL:       idpURL,
		MFA:          mfaToken,
		PrincipalARN: principalARN,
		RoleARN:      roleARN,
		Region:       region,
		Duration:     duration,
	})
	if err != nil {
		return fmt.Errorf("credential error: %w", err)
	}

	// Handle output
	if format == "shell" {
		return launchShell(prov, cred, cfg.shellScript)
	}

	return outputCredentials(prov, cred, format, cliProfile)
}
