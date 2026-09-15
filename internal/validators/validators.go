// Package validators provides named validator functions for user-supplied
// input, shared by the CLI and the interactive TUI.
//
// Validators come in two flavours. Provider-independent ones (email, password,
// idp-url, mfa, output-format) live in a plain registry reachable through Get.
// Provider-dependent ones (role-arn, principal-arn, region) cannot live there,
// because what counts as a valid ARN or region differs per cloud vendor; they
// are obtained through ProviderAware, which binds them to a cloud.Provider.
package validators

import (
	"errors"
	"fmt"
	"maps"
	"net/mail"
	"net/url"
	"slices"
	"strings"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// Provider-independent validator keys.
const (
	KeySkip         = "skip"
	KeyEmail        = "email"
	KeyPassword     = "password"
	KeyIdpURL       = "idp-url"
	KeyMFA          = "mfa"
	KeyOutputFormat = "output-format"
)

// Provider-dependent validator keys. These are deliberately absent from the
// registry behind Get; use ProviderAware to obtain them.
const (
	KeyRoleARN     = "role-arn"
	KeyProviderARN = "principal-arn" // spelled for CLI flag compatibility
	KeyRegion      = "region"
)

// OutputFormats lists the supported credential output formats.
var OutputFormats = []string{"cli", "cli-stdout", "env", "env-stdout", "shell"}

// skip accepts any input.
func skip(input string) error { return nil }

// validatorMap contains the provider-independent validator functions.
var validatorMap = map[string]func(input string) error{
	KeySkip: skip,
	KeyEmail: func(input string) error {
		if _, err := mail.ParseAddress(input); err != nil {
			return fmt.Errorf("invalid e-mail address: %w", err)
		}
		return nil
	},
	KeyPassword: func(input string) error {
		if len(input) < 8 {
			return errors.New("password must be at least 8 characters")
		}
		return nil
	},
	KeyIdpURL: func(input string) error {
		u, err := url.Parse(input)
		if err != nil {
			return fmt.Errorf("invalid idp url: %w", err)
		}
		if u.Scheme != "https" || u.Host == "" {
			return errors.New("idp url must be an absolute https URL")
		}
		return nil
	},
	KeyMFA: func(input string) error {
		if len(input) < 6 {
			return errors.New("mfa must be a 6-digit totp code or mfa secret string")
		}
		return nil
	},
	KeyOutputFormat: func(input string) error {
		if !slices.Contains(OutputFormats, input) {
			return fmt.Errorf("invalid output format %q (supported: %s)", input, strings.Join(OutputFormats, ", "))
		}
		return nil
	},
}

// Get returns the provider-independent validator function for the given key.
// Unknown keys — which include the provider-dependent keys role-arn,
// principal-arn and region — return a validator that accepts any input, so the
// result is always safe to call.
//
// Use ProviderAware when a cloud.Provider is available.
func Get(key string) func(string) error {
	if v, ok := validatorMap[key]; ok {
		return v
	}
	return skip
}

// ProviderAware returns the validator for the given key bound to p. For the
// provider-dependent keys it delegates to p; every other key falls through to
// Get, so ProviderAware is a strict superset of Get.
//
// A nil provider yields the permissive skip validator for the
// provider-dependent keys, which keeps the TUI usable before an account — and
// therefore a provider — has been chosen.
func ProviderAware(key string, p cloud.Provider) func(string) error {
	isProviderKey := key == KeyRoleARN || key == KeyProviderARN || key == KeyRegion

	if p == nil {
		if isProviderKey {
			return skip
		}
		return Get(key)
	}

	switch key {
	case KeyRoleARN:
		return p.ValidateRoleARN
	case KeyProviderARN:
		return p.ValidateProviderARN
	case KeyRegion:
		return p.ValidateRegion
	}

	return Get(key)
}

// Names returns the sorted list of registered provider-independent validator
// names. Provider-dependent keys are listed by ProviderKeys.
func Names() []string {
	return slices.Sorted(maps.Keys(validatorMap))
}

// ProviderKeys returns the sorted list of provider-dependent validator keys,
// which are served by ProviderAware rather than Get.
func ProviderKeys() []string {
	return []string{KeyProviderARN, KeyRegion, KeyRoleARN}
}
