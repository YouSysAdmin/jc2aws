package config

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// ErrRoleNotFound is returned when a role name is not present in the account.
var ErrRoleNotFound = errors.New("role not found")

// Account store information about a configured cloud account.
//
// Field names are provider-neutral. See UnmarshalYAML for the accepted YAML
// keys, including the deprecated aws_-prefixed aliases.
//
// Note: Account decodes but does not round-trip through yaml.Marshal, because
// UnmarshalYAML collapses two key sets into one field set. Nothing in this
// repository marshals it.
type Account struct {
	Name        string
	Description string
	// Provider is the canonical cloud provider name; never empty after decoding.
	Provider string
	// CLIProfile overrides the vendor CLI profile name, which otherwise
	// defaults to Name at the point of use.
	CLIProfile string
	Email      string
	Password   string
	MFASecret  string
	// PrincipalARN is the SAML identity provider ARN.
	PrincipalARN string
	Roles        []Role
	Regions      []string
	IdpURL       string
	Duration     int

	// deprecations collects non-fatal config-hygiene warnings noticed while
	// decoding, surfaced through Warnings.
	deprecations []string
}

// Warnings returns non-fatal configuration problems noticed while decoding this
// account, such as a deprecated key or a neutral key shadowing its aws_ alias.
func (a *Account) Warnings() []string { return a.deprecations }

// Role store information about an assumable role.
type Role struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Arn         string `yaml:"arn"`
}

// accountYAML mirrors Account plus the deprecated aws_-prefixed aliases. It
// exists only so Account.UnmarshalYAML can collapse both key sets into a single
// field set.
type accountYAML struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Provider    string `yaml:"provider"`
	Email       string `yaml:"email"`
	Password    string `yaml:"password"`
	MFASecret   string `yaml:"mfa_token_secret"`
	IdpURL      string `yaml:"jc_idp_url"`
	Duration    int    `yaml:"session_duration"`

	// Preferred, provider-neutral keys.
	CLIProfile   string   `yaml:"cli_profile"`
	PrincipalARN string   `yaml:"principal_arn"`
	Roles        []Role   `yaml:"role_arns"`
	Regions      []string `yaml:"regions"`

	// Deprecated: aws_-prefixed aliases, honoured when the neutral key is absent.
	AWSCLIProfile   string   `yaml:"aws_cli_profile"`
	AWSPrincipalARN string   `yaml:"aws_principal_arn"`
	AWSRoles        []Role   `yaml:"aws_role_arns"`
	AWSRegions      []string `yaml:"aws_regions"`
	// Deprecated: use session_duration instead. Will be removed in a future release.
	SessionTimeout int `yaml:"session_timeout"`
}

// UnmarshalYAML decodes an account, accepting both the provider-neutral keys
// (provider, principal_arn, role_arns, regions, cli_profile) and the deprecated
// aws_-prefixed aliases (aws_principal_arn, aws_role_arns, aws_regions,
// aws_cli_profile). When both spellings are present the neutral key wins.
//
// The deprecated session_timeout key is migrated to session_duration here.
func (a *Account) UnmarshalYAML(value *yaml.Node) error {
	var raw accountYAML
	if err := value.Decode(&raw); err != nil {
		return fmt.Errorf("failed to decode account: %w", err)
	}

	*a = Account{
		Name:         raw.Name,
		Description:  raw.Description,
		Provider:     cloud.Normalize(raw.Provider),
		Email:        raw.Email,
		Password:     raw.Password,
		MFASecret:    raw.MFASecret,
		IdpURL:       raw.IdpURL,
		CLIProfile:   cmp.Or(raw.CLIProfile, raw.AWSCLIProfile),
		PrincipalARN: cmp.Or(raw.PrincipalARN, raw.AWSPrincipalARN),
		Duration:     cmp.Or(raw.Duration, raw.SessionTimeout),
	}

	// cmp.Or does not work on slices; pick explicitly.
	a.Roles = raw.Roles
	if len(a.Roles) == 0 {
		a.Roles = raw.AWSRoles
	}
	a.Regions = raw.Regions
	if len(a.Regions) == 0 {
		a.Regions = raw.AWSRegions
	}

	a.deprecations = raw.deprecationWarnings(a.Name)

	return nil
}

// deprecationWarnings reports config-hygiene problems that are worth telling the
// user about but are not worth refusing to start over.
func (raw accountYAML) deprecationWarnings(name string) []string {
	var warnings []string

	both := func(neutral, legacy, neutralKey, legacyKey string) {
		if neutral != "" && legacy != "" {
			warnings = append(warnings, fmt.Sprintf(
				"account %q sets both %s and %s; %s wins", name, neutralKey, legacyKey, neutralKey))
		}
	}
	both(raw.CLIProfile, raw.AWSCLIProfile, "cli_profile", "aws_cli_profile")
	both(raw.PrincipalARN, raw.AWSPrincipalARN, "principal_arn", "aws_principal_arn")
	if len(raw.Roles) > 0 && len(raw.AWSRoles) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"account %q sets both role_arns and aws_role_arns; role_arns wins", name))
	}
	if len(raw.Regions) > 0 && len(raw.AWSRegions) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"account %q sets both regions and aws_regions; regions wins", name))
	}
	if raw.SessionTimeout != 0 {
		warnings = append(warnings, fmt.Sprintf(
			"account %q uses deprecated session_timeout; rename it to session_duration", name))
	}

	// aws_-prefixed keys on a non-AWS account are almost certainly a mistake.
	if cloud.Normalize(raw.Provider) != cloud.NameAWS {
		usesLegacy := raw.AWSCLIProfile != "" || raw.AWSPrincipalARN != "" ||
			len(raw.AWSRoles) > 0 || len(raw.AWSRegions) > 0
		if usesLegacy {
			warnings = append(warnings, fmt.Sprintf(
				"account %q has provider %q but uses aws_-prefixed keys; prefer the neutral keys",
				name, cloud.Normalize(raw.Provider)))
		}
	}

	return warnings
}

// FindRoleByName return the account role with the given name.
func (a *Account) FindRoleByName(name string) (role Role, err error) {
	idx := slices.IndexFunc(a.Roles, func(r Role) bool { return r.Name == name })
	if idx < 0 {
		return role, fmt.Errorf("%w: %s", ErrRoleNotFound, name)
	}
	return a.Roles[idx], nil
}
