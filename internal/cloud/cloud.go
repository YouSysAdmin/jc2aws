// Package cloud defines the provider-neutral contract between the jc2aws front
// end (CLI and TUI) and the cloud vendors that can exchange a JumpCloud SAML
// assertion for temporary credentials.
//
// This package must not import any vendor SDK. Implementations live in
// internal/aws and internal/alibaba and are constructed through
// internal/cloud/providers, which is the only package that imports them all.
package cloud

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Canonical provider names, as written in the `provider:` config key.
const (
	// NameAWS identifies Amazon Web Services.
	NameAWS = "aws"
	// NameAlibaba identifies Alibaba Cloud.
	NameAlibaba = "alibaba"
)

// DefaultName is the provider assumed when an account does not set one.
const DefaultName = NameAWS

// ErrUnknownProvider is returned for a provider name that is not supported.
var ErrUnknownProvider = errors.New("unknown cloud provider")

// Names returns the supported provider names in a stable order.
//
// It is a pure string helper that does not instantiate providers, so packages
// which only need to validate a config value can call it without pulling in any
// vendor SDK.
func Names() []string {
	return []string{NameAWS, NameAlibaba}
}

// Normalize canonicalizes a provider name: it trims surrounding space,
// lowercases the result, and maps the empty string to DefaultName.
func Normalize(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return DefaultName
	}
	return name
}

// IsKnown reports whether Normalize(name) is a supported provider.
func IsKnown(name string) bool {
	return slices.Contains(Names(), Normalize(name))
}

// SAMLInput carries everything needed to exchange a SAML assertion for
// temporary credentials. Field names are vendor-neutral.
type SAMLInput struct {
	// ProviderARN identifies the SAML identity provider registered with the
	// cloud vendor: the IAM SAML provider ("principal") ARN on AWS, the RAM
	// SAML provider ARN on Alibaba Cloud.
	ProviderARN string
	// RoleARN is the role to assume.
	RoleARN string
	// SAMLAssertion is the opaque base64 SAMLResponse returned by JumpCloud.
	SAMLAssertion string
	// Region is the vendor region the credentials are scoped to.
	Region string
	// DurationSeconds is the requested credential lifetime.
	DurationSeconds int32
}

// Credentials holds temporary credentials issued by a cloud provider.
//
// It is a plain data type with no behavior: rendering credentials for a shell,
// an env file, or a vendor CLI is provider-specific and lives on Provider.
type Credentials struct {
	// Provider is the canonical name of the provider that issued these
	// credentials. It is informational, used for display only. Never branch on
	// it: use the Provider value you already hold, because a hand-built
	// Credentials literal may leave this field empty.
	Provider string

	// AccessKeyID is the temporary access key identifier.
	AccessKeyID string
	// SecretAccessKey is the temporary secret key.
	SecretAccessKey string
	// SessionToken is the temporary session or security token.
	SessionToken string
	// Region is the region the credentials were requested for.
	Region string
	// Expiration is nil when the provider did not report one.
	Expiration *time.Time
}

// File is a rendered configuration file: a destination path plus its complete
// new contents.
//
// Providers render files; the caller writes them. This keeps a single atomic
// 0600 writer as the only code in the repository that touches credential files
// on disk, and means a rendering failure never leaves a half-written set.
type File struct {
	// Path is the absolute destination path.
	Path string
	// Data is the complete new contents of the file.
	Data []byte
}

// Info describes a provider for display and labelling purposes.
type Info struct {
	// DisplayName is the human-readable vendor name, such as "AWS".
	DisplayName string
	// ProviderARNLabel names the SAML identity provider field in the UI,
	// such as "Principal ARN".
	ProviderARNLabel string
	// RoleARNLabel names the role field in the UI, such as "Role ARN".
	RoleARNLabel string
	// CLIDescription describes what the "cli" output format writes.
	CLIDescription string
}

// Provider exchanges a SAML assertion for temporary credentials of one cloud
// vendor and renders those credentials in the shapes that vendor's tooling
// expects. Implementations must be stateless and safe for concurrent use.
type Provider interface {
	// Name returns the canonical config name, such as "aws".
	Name() string

	// Info returns display metadata used by the CLI help text and the TUI.
	Info() Info

	// Regions returns a copy of the built-in list of known region IDs. The
	// list may lag behind the vendor's real region set, so callers should
	// treat a miss as a warning rather than a hard error.
	Regions() []string

	// SessionDurationRange returns the inclusive bounds, in seconds, that this
	// provider accepts for a credential lifetime.
	SessionDurationRange() (min, max int)

	// ValidateRoleARN reports whether s is a syntactically valid role
	// identifier for this provider.
	ValidateRoleARN(s string) error

	// ValidateProviderARN reports whether s is a syntactically valid SAML
	// identity provider identifier for this provider.
	ValidateProviderARN(s string) error

	// ValidateRegion reports whether s names a region this provider accepts.
	ValidateRegion(s string) error

	// AssumeRoleWithSAML exchanges in.SAMLAssertion for temporary credentials.
	AssumeRoleWithSAML(ctx context.Context, in SAMLInput) (Credentials, error)

	// Env returns the credential environment variables as "KEY=value" strings,
	// in the order this provider's tooling documents them.
	Env(cred Credentials) []string

	// CLIFiles renders every configuration file this provider's CLI needs for
	// the given profile, rooted at homeDir. Existing files at those paths are
	// read and merged so unrelated profiles survive. Nothing is written: the
	// caller owns creating parent directories and writing the bytes.
	//
	// An empty profileName means the provider's default profile.
	CLIFiles(homeDir, profileName string, cred Credentials) ([]File, error)

	// StdoutProfile renders the single blob printed by the "cli-stdout" output
	// format: a self-contained profile snippet with no on-disk merge.
	StdoutProfile(profileName string, cred Credentials) ([]byte, error)
}

// EnvString renders p.Env(cred) as a newline-separated block with a trailing
// newline, suitable for writing to a dotenv file or printing to stdout.
func EnvString(p Provider, cred Credentials) string {
	return strings.Join(p.Env(cred), "\n") + "\n"
}

// ValidateRegionInList reports whether region appears in p.Regions(). It is the
// building block for providers whose region set is a closed, known list.
func ValidateRegionInList(p Provider, region string) error {
	if !slices.Contains(p.Regions(), region) {
		return fmt.Errorf("invalid region %q for %s", region, p.Info().DisplayName)
	}
	return nil
}
