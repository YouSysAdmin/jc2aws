// Package alibaba implements cloud.Provider for Alibaba Cloud: it exchanges a
// SAML assertion for temporary RAM/STS credentials and renders them in the
// shapes the aliyun CLI and the Alibaba Cloud SDKs expect.
package alibaba

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	"github.com/alibabacloud-go/tea/dara"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// DefaultProfileName is used when no profile name is provided.
const DefaultProfileName = "default"

// Session duration bounds accepted by Alibaba Cloud STS, in seconds. The real
// ceiling is the assumed role's MaxSessionDuration, which the API enforces; the
// upper bound here is the widest a role may be configured for.
const (
	minDurationSeconds = 900
	maxDurationSeconds = 43200
)

// expirationLayout is the format STS uses for Credentials.Expiration.
//
// The literal Z is deliberate: the service documents UTC only, and parsing with
// time.RFC3339 would silently accept offsets the service never emits, hiding a
// contract change instead of surfacing it.
const expirationLayout = "2006-01-02T15:04:05Z"

// regionRe matches a syntactically plausible Alibaba Cloud region ID.
var regionRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)

// Provider implements cloud.Provider for Alibaba Cloud.
type Provider struct {
	// endpoint overrides the SDK's regional endpoint derivation. It is only set
	// by tests; in production the SDK builds sts.<region>.aliyuncs.com itself.
	endpoint string
	// protocol overrides the request scheme, likewise only for tests.
	protocol string
}

// Option customises a Provider.
type Option func(*Provider)

// WithEndpoint pins the STS endpoint instead of letting the SDK derive
// sts.<region>.aliyuncs.com. An endpoint carrying an explicit "http://" scheme
// also switches the request to plain HTTP, which is how tests point the client
// at an httptest server.
func WithEndpoint(endpoint string) Option {
	return func(p *Provider) {
		if rest, ok := strings.CutPrefix(endpoint, "http://"); ok {
			p.endpoint, p.protocol = rest, "HTTP"
			return
		}
		p.endpoint = strings.TrimPrefix(endpoint, "https://")
	}
}

// New returns the Alibaba Cloud provider configured with the supplied options.
func New(opts ...Option) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var _ cloud.Provider = (*Provider)(nil)

// Name returns the canonical provider name "alibaba".
func (*Provider) Name() string { return cloud.NameAlibaba }

// Info returns the display metadata used by the CLI help text and the TUI.
func (*Provider) Info() cloud.Info {
	return cloud.Info{
		DisplayName:      "Alibaba Cloud",
		ProviderARNLabel: "SAML Provider ARN",
		RoleARNLabel:     "RAM Role ARN",
		CLIDescription:   "Write to ~/.aliyun/config.json and ~/.alibabacloud/credentials",
	}
}

// Regions returns a copy of the built-in Alibaba Cloud region list.
func (*Provider) Regions() []string { return slices.Clone(RegionsList) }

// SessionDurationRange returns the inclusive credential lifetime bounds, in
// seconds, accepted by Alibaba Cloud STS.
func (*Provider) SessionDurationRange() (int, int) {
	return minDurationSeconds, maxDurationSeconds
}

// ValidateRoleARN reports whether s is a well-formed RAM role ARN.
func (*Provider) ValidateRoleARN(s string) error {
	_, err := ParseRoleARN(s)
	return err
}

// ValidateProviderARN reports whether s is a well-formed RAM SAML provider ARN.
func (*Provider) ValidateProviderARN(s string) error {
	_, err := ParseSAMLProviderARN(s)
	return err
}

// ValidateRegion reports whether s is a syntactically valid region ID.
//
// Unlike the AWS provider this is a syntax check rather than a membership test:
// RegionsList only drives the interactive picker, and rejecting a region merely
// because it is newer than this build would be worse than passing it through to
// the API.
func (*Provider) ValidateRegion(s string) error {
	if s == "" {
		return errors.New("region must not be empty")
	}
	if !regionRe.MatchString(s) {
		return fmt.Errorf("invalid region %q for Alibaba Cloud", s)
	}
	return nil
}

// ValidateDuration reports whether d is within the STS session duration bounds.
func (p *Provider) ValidateDuration(d int32) error {
	minDuration, maxDuration := p.SessionDurationRange()
	if int(d) < minDuration || int(d) > maxDuration {
		return fmt.Errorf("session duration %d is out of the allowed range %d-%d seconds",
			d, minDuration, maxDuration)
	}
	return nil
}

// ParseExpiration parses an STS Expiration timestamp.
func ParseExpiration(s string) (*time.Time, error) {
	t, err := time.Parse(expirationLayout, s)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sts expiration %q: %w", s, err)
	}
	return &t, nil
}

// AssumeRoleWithSAML exchanges a SAML assertion for temporary STS credentials.
func (p *Provider) AssumeRoleWithSAML(ctx context.Context, in cloud.SAMLInput) (cloud.Credentials, error) {
	if err := p.ValidateRegion(in.Region); err != nil {
		return cloud.Credentials{}, err
	}
	if err := p.ValidateRoleARN(in.RoleARN); err != nil {
		return cloud.Credentials{}, err
	}
	if err := p.ValidateProviderARN(in.ProviderARN); err != nil {
		return cloud.Credentials{}, err
	}
	if err := p.ValidateDuration(in.DurationSeconds); err != nil {
		return cloud.Credentials{}, err
	}

	// Credential is deliberately left nil. AssumeRoleWithSAML is declared with
	// AuthType "Anonymous", so the SDK skips both its nil-credential guard and
	// request signing: the assertion is the authentication. Endpoint is left
	// unset too, so the SDK derives the regional sts.<region>.aliyuncs.com.
	cfg := &openapiutil.Config{RegionId: dara.String(in.Region)}
	if p.endpoint != "" {
		cfg.Endpoint = dara.String(p.endpoint)
	}
	if p.protocol != "" {
		cfg.Protocol = dara.String(p.protocol)
	}

	client, err := stsclient.NewClient(cfg)
	if err != nil {
		return cloud.Credentials{}, fmt.Errorf("failed to create alibaba sts client: %w", err)
	}

	req := &stsclient.AssumeRoleWithSAMLRequest{
		RoleArn:         dara.String(in.RoleARN),
		SAMLProviderArn: dara.String(in.ProviderARN),
		SAMLAssertion:   dara.String(in.SAMLAssertion),
		DurationSeconds: dara.Int64(int64(in.DurationSeconds)),
	}

	// The context-free AssumeRoleWithSAML overload would silently drop ctx.
	res, err := client.AssumeRoleWithSAMLWithContext(ctx, req, &dara.RuntimeOptions{})
	if err != nil {
		return cloud.Credentials{}, fmt.Errorf("failed to assume role with SAML: %w", err)
	}
	if res == nil {
		return cloud.Credentials{}, errors.New("alibaba sts returned an empty response")
	}

	return toCredentials(res.Body, in.Region)
}

// toCredentials maps the SDK response body onto the provider-neutral type.
func toCredentials(body *stsclient.AssumeRoleWithSAMLResponseBody, region string) (cloud.Credentials, error) {
	if body == nil || body.Credentials == nil {
		return cloud.Credentials{}, errors.New("alibaba sts returned no credentials")
	}

	c := body.Credentials
	out := cloud.Credentials{
		Provider:        cloud.NameAlibaba,
		AccessKeyID:     dara.StringValue(c.AccessKeyId),
		SecretAccessKey: dara.StringValue(c.AccessKeySecret),
		SessionToken:    dara.StringValue(c.SecurityToken),
		Region:          region,
	}
	if exp := dara.StringValue(c.Expiration); exp != "" {
		t, err := ParseExpiration(exp)
		if err != nil {
			return cloud.Credentials{}, err
		}
		out.Expiration = t
	}

	return out, nil
}

// Env returns the Alibaba Cloud credential environment variables as
// "KEY=value" strings.
//
// Both region variables are emitted on purpose: the aliyun CLI reads
// ALIBABA_CLOUD_REGION_ID while the Terraform alicloud provider reads
// ALIBABA_CLOUD_REGION. The legacy ALICLOUD_* names are not emitted, because
// every consumer that accepts them also accepts the ALIBABA_CLOUD_* spellings.
func (*Provider) Env(cred cloud.Credentials) []string {
	return []string{
		fmt.Sprintf("ALIBABA_CLOUD_ACCESS_KEY_ID=%s", cred.AccessKeyID),
		fmt.Sprintf("ALIBABA_CLOUD_ACCESS_KEY_SECRET=%s", cred.SecretAccessKey),
		fmt.Sprintf("ALIBABA_CLOUD_SECURITY_TOKEN=%s", cred.SessionToken),
		fmt.Sprintf("ALIBABA_CLOUD_REGION_ID=%s", cred.Region),
		fmt.Sprintf("ALIBABA_CLOUD_REGION=%s", cred.Region),
	}
}
