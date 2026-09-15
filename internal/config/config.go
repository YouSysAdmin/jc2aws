package config

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

const DefaultConfigFileName = ".jc2aws.yaml"

// ErrAccountNotFound is returned when an account name is not present in the config.
var ErrAccountNotFound = errors.New("account not found")

// Config of TUI/CLI
type Config struct {
	DefaultEmail          string    `yaml:"default_email"`
	DefaultPassword       string    `yaml:"default_password"`
	DefaultMFATokenSecret string    `yaml:"default_mfa_token_secret"`
	NoUpdateCheck         bool      `yaml:"no_update_check"`
	DefaultFormat         string    `yaml:"default_format"`
	TUIDoneAction         string    `yaml:"tui_done_action"`
	Accounts              []Account `yaml:"accounts"`
}

// expandHome expands a leading ~ in the given path to the user's home directory.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

// NewConfig read config from file and return filled Config struct
func NewConfig(path string) (conf *Config, err error) {
	conf = &Config{}
	path = expandHome(path)

	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return conf, fmt.Errorf("config file %s not found: %w", path, err)
	}

	// The config file may hold plaintext passwords and MFA secrets.
	if err == nil && fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: config file %s is accessible by other users (mode %o); consider chmod 600\n",
			path, fi.Mode().Perm())
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return conf, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(file, conf); err != nil {
		return conf, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Reject an unknown provider outright. Silently defaulting to AWS would
	// hand the user AWS credentials when they asked for another vendor, which
	// is far worse than refusing to start.
	for _, a := range conf.Accounts {
		if !cloud.IsKnown(a.Provider) {
			return conf, fmt.Errorf("account %q: unknown provider %q (supported: %s)",
				a.Name, a.Provider, strings.Join(cloud.Names(), ", "))
		}
		for _, w := range a.Warnings() {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
		}
	}

	return conf, nil
}

// applyDefaults fills account-level blanks from the config-wide defaults.
func (c *Config) applyDefaults(a Account) Account {
	// Normalize here too: an Account built as a struct literal, as the TUI and
	// the tests do, never passed through UnmarshalYAML.
	a.Provider = cloud.Normalize(a.Provider)
	a.Email = cmp.Or(a.Email, c.DefaultEmail)
	a.Password = cmp.Or(a.Password, c.DefaultPassword)
	a.MFASecret = cmp.Or(a.MFASecret, c.DefaultMFATokenSecret)
	return a
}

// GetAccounts return list of accounts
func (c *Config) GetAccounts() (accounts []Account) {
	// sets default email, password and mfa if it is not set for an account separately
	for _, a := range c.Accounts {
		accounts = append(accounts, c.applyDefaults(a))
	}
	return accounts
}

// FindAccountByName return account by account name from accounts list
func (c *Config) FindAccountByName(name string) (account Account, err error) {
	idx := slices.IndexFunc(c.Accounts, func(a Account) bool { return a.Name == name })
	if idx < 0 {
		return account, fmt.Errorf("%w: %s", ErrAccountNotFound, name)
	}

	return c.applyDefaults(c.Accounts[idx]), nil
}
