// Package aws implements cloud.Provider for Amazon Web Services: it exchanges a
// SAML assertion for temporary STS credentials and renders them in the shapes
// the AWS CLI expects.
package aws

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// RegionsList Available AWS Regions
var RegionsList = []string{"us-east-1", "us-east-2", "us-west-1", "us-west-2", "af-south-1", "ap-east-1",
	"ap-south-2", "ap-southeast-3", "ap-southeast-4", "ap-south-1", "ap-northeast-3", "ap-northeast-2",
	"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ca-central-1", "ca-west-1", "eu-central-1",
	"eu-west-1", "eu-west-2", "eu-south-1", "eu-south-2", "eu-west-3", "eu-north-1",
	"eu-central-2", "il-central-1", "me-south-1", "me-central-1", "sa-east-1", "us-gov-east-1",
	"us-gov-west-1", "cn-north-1", "cn-northwest-1"}

// DefaultAwsProfileName is used when no profile name is provided.
const DefaultAwsProfileName = "default"

// Session duration bounds accepted by AWS STS, in seconds.
const (
	minDurationSeconds = 900
	maxDurationSeconds = 43200
)

// Provider implements cloud.Provider for Amazon Web Services.
type Provider struct{}

// New returns the AWS cloud provider.
func New() *Provider { return &Provider{} }

var _ cloud.Provider = (*Provider)(nil)

// Name returns the canonical provider name "aws".
func (*Provider) Name() string { return cloud.NameAWS }

// Info returns the display metadata used by the CLI help text and the TUI.
func (*Provider) Info() cloud.Info {
	return cloud.Info{
		DisplayName:      "AWS",
		ProviderARNLabel: "Principal ARN",
		RoleARNLabel:     "Role ARN",
		CLIDescription:   "Write to ~/.aws/credentials and ~/.aws/config",
	}
}

// Regions returns a copy of the built-in AWS region list.
func (*Provider) Regions() []string { return slices.Clone(RegionsList) }

// SessionDurationRange returns the inclusive credential lifetime bounds, in
// seconds, accepted by AWS STS.
func (*Provider) SessionDurationRange() (int, int) {
	return minDurationSeconds, maxDurationSeconds
}

// ValidateRoleARN reports whether s is a syntactically valid IAM role ARN.
func (*Provider) ValidateRoleARN(s string) error {
	if _, err := arn.Parse(s); err != nil {
		return fmt.Errorf("invalid role arn: %w", err)
	}
	return nil
}

// ValidateProviderARN reports whether s is a syntactically valid IAM SAML
// provider ARN, known in the AWS API as the principal ARN.
func (*Provider) ValidateProviderARN(s string) error {
	if _, err := arn.Parse(s); err != nil {
		return fmt.Errorf("invalid principal arn: %w", err)
	}
	return nil
}

// ValidateRegion reports whether s is one of the known AWS regions.
func (p *Provider) ValidateRegion(s string) error {
	if !slices.Contains(RegionsList, s) {
		return fmt.Errorf("invalid region %q for AWS", s)
	}
	return nil
}

// toSTSInput converts the provider-neutral input into the official AWS SDK
// request, returning the region separately because the SDK takes it from the
// client config rather than the request.
func toSTSInput(in cloud.SAMLInput) (s sts.AssumeRoleWithSAMLInput, r string) {
	s = sts.AssumeRoleWithSAMLInput{
		PrincipalArn:    new(in.ProviderARN),
		RoleArn:         new(in.RoleARN),
		SAMLAssertion:   new(in.SAMLAssertion),
		DurationSeconds: new(in.DurationSeconds),
	}
	r = in.Region

	return s, r
}

// toCredentials converts the official AWS SDK credentials into the
// provider-neutral type. A nil input yields credentials carrying only the
// region, matching the SDK's own tolerance for an empty response.
func toCredentials(creds *types.Credentials, region string) cloud.Credentials {
	if creds == nil {
		return cloud.Credentials{Provider: cloud.NameAWS, Region: region}
	}

	out := cloud.Credentials{
		Provider:        cloud.NameAWS,
		AccessKeyID:     aws.ToString(creds.AccessKeyId),
		SecretAccessKey: aws.ToString(creds.SecretAccessKey),
		SessionToken:    aws.ToString(creds.SessionToken),
		Region:          region,
	}
	if creds.Expiration != nil {
		out.Expiration = new(*creds.Expiration)
	}

	return out
}

// AssumeRoleWithSAML exchanges a SAML assertion for temporary STS credentials.
func (*Provider) AssumeRoleWithSAML(ctx context.Context, in cloud.SAMLInput) (cloud.Credentials, error) {
	awsInput, region := toSTSInput(in)

	cfg := aws.Config{ // just stub for the sdk config
		Credentials: credentials.NewStaticCredentialsProvider(
			"AKIAEXAMPLE",
			"SECRETEXAMPLE",
			"",
		),
		Region: region,
	}

	client := sts.NewFromConfig(cfg)

	res, err := client.AssumeRoleWithSAML(ctx, &awsInput)
	if err != nil {
		return cloud.Credentials{}, fmt.Errorf("failed to assume role with SAML: %w", err)
	}

	return toCredentials(res.Credentials, region), nil
}

// Env returns the AWS credential environment variables as "KEY=value" strings.
func (*Provider) Env(cred cloud.Credentials) []string {
	return []string{
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", cred.AccessKeyID),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", cred.SecretAccessKey),
		fmt.Sprintf("AWS_SESSION_TOKEN=%s", cred.SessionToken),
		fmt.Sprintf("AWS_REGION=%s", cred.Region),
		fmt.Sprintf("AWS_DEFAULT_REGION=%s", cred.Region),
	}
}
