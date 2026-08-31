package validators

import (
	"errors"
	"fmt"
	"maps"
	"net/mail"
	"net/url"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/yousysadmin/jc2aws/internal/aws"
)

// OutputFormats lists the supported credential output formats.
var OutputFormats = []string{"cli", "cli-stdout", "env", "env-stdout", "shell"}

// skip accepts any input.
func skip(input string) error { return nil }

// validatorMap contains named validator functions for input parameters.
var validatorMap = map[string]func(input string) error{
	"skip": skip,
	"email": func(input string) error {
		if _, err := mail.ParseAddress(input); err != nil {
			return fmt.Errorf("invalid e-mail address: %w", err)
		}
		return nil
	},
	"password": func(input string) error {
		if len(input) < 8 {
			return errors.New("password must be at least 8 characters")
		}
		return nil
	},
	"idp-url": func(input string) error {
		u, err := url.Parse(input)
		if err != nil {
			return fmt.Errorf("invalid idp url: %w", err)
		}
		if u.Scheme != "https" || u.Host == "" {
			return errors.New("idp url must be an absolute https URL")
		}
		return nil
	},
	"role-arn": func(input string) error {
		if _, err := arn.Parse(input); err != nil {
			return fmt.Errorf("invalid role arn: %w", err)
		}
		return nil
	},
	"principal-arn": func(input string) error {
		if _, err := arn.Parse(input); err != nil {
			return fmt.Errorf("invalid principal arn: %w", err)
		}
		return nil
	},
	"region": func(input string) error {
		if !slices.Contains(aws.RegionsList, input) {
			return errors.New("invalid region")
		}
		return nil
	},
	"mfa": func(input string) error {
		if len(input) < 6 {
			return errors.New("mfa must be a 6-digit totp code or mfa secret string")
		}
		return nil
	},
	"output-format": func(input string) error {
		if !slices.Contains(OutputFormats, input) {
			return fmt.Errorf("invalid output format %q (supported: %s)", input, strings.Join(OutputFormats, ", "))
		}
		return nil
	},
}

// Get returns the validator function for the given key.
// Unknown keys return a validator that accepts any input, so the result is
// always safe to call.
func Get(key string) func(string) error {
	if v, ok := validatorMap[key]; ok {
		return v
	}
	return skip
}

// Names returns the sorted list of registered validator names.
func Names() []string {
	return slices.Sorted(maps.Keys(validatorMap))
}
