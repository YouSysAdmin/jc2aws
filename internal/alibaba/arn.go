package alibaba

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidRoleARN is returned for a malformed RAM role ARN.
var ErrInvalidRoleARN = errors.New("invalid role arn")

// ErrInvalidSAMLProviderARN is returned for a malformed RAM SAML provider ARN.
var ErrInvalidSAMLProviderARN = errors.New("invalid saml provider arn")

// The empty partition and region fields in "acs:ram::" are load-bearing: RAM is
// a global service, so an ARN carrying a region is a misconfiguration that
// deserves a clear error rather than an opaque STS rejection.
var (
	roleARNRe         = regexp.MustCompile(`^acs:ram::([0-9]{6,32}):role/([A-Za-z0-9+=,.@_-]{1,64})$`)
	samlProviderARNRe = regexp.MustCompile(`^acs:ram::([0-9]{6,32}):saml-provider/([A-Za-z0-9+=,.@_-]{1,128})$`)
)

// RoleARN identifies a RAM role by account ID and role name.
type RoleARN struct {
	AccountID string
	Name      string
}

// SAMLProviderARN identifies a RAM SAML identity provider.
type SAMLProviderARN struct {
	AccountID string
	Name      string
}

// ParseRoleARN parses acs:ram::<account_id>:role/<role_name>.
func ParseRoleARN(s string) (RoleARN, error) {
	m := roleARNRe.FindStringSubmatch(s)
	if m == nil {
		return RoleARN{}, fmt.Errorf("%w %q: want acs:ram::<account_id>:role/<role_name>", ErrInvalidRoleARN, s)
	}
	return RoleARN{AccountID: m[1], Name: m[2]}, nil
}

// ParseSAMLProviderARN parses acs:ram::<account_id>:saml-provider/<provider_name>.
func ParseSAMLProviderARN(s string) (SAMLProviderARN, error) {
	m := samlProviderARNRe.FindStringSubmatch(s)
	if m == nil {
		return SAMLProviderARN{}, fmt.Errorf("%w %q: want acs:ram::<account_id>:saml-provider/<provider_name>",
			ErrInvalidSAMLProviderARN, s)
	}
	return SAMLProviderARN{AccountID: m[1], Name: m[2]}, nil
}
