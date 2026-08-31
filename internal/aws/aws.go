package aws

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"gopkg.in/ini.v1"
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

// AwsSamlOutput struct for storing prepared AWS credentials
type AwsSamlOutput struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	Expiration      *time.Time
}

// AwsSamlInput struct for input parameters for next used with the official AWS lib
type AwsSamlInput struct {
	PrincipalArn    string
	RoleArn         string
	SAMLAssertion   string
	Region          string
	DurationSeconds int32
}

// ToAwsInput converter from standard types to official AWS lib types
func (i *AwsSamlInput) ToAwsInput() (s sts.AssumeRoleWithSAMLInput, r string) {
	s = sts.AssumeRoleWithSAMLInput{
		PrincipalArn:    new(i.PrincipalArn),
		RoleArn:         new(i.RoleArn),
		SAMLAssertion:   new(i.SAMLAssertion),
		DurationSeconds: new(i.DurationSeconds),
	}
	r = i.Region

	return s, r
}

// ToAwsSamlOutput converter from official AWS lib types to standart
func ToAwsSamlOutput(credentials *types.Credentials, region string) AwsSamlOutput {
	if credentials == nil {
		return AwsSamlOutput{Region: region}
	}

	out := AwsSamlOutput{
		AccessKeyID:     aws.ToString(credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(credentials.SecretAccessKey),
		SessionToken:    aws.ToString(credentials.SessionToken),
		Region:          region,
	}
	if credentials.Expiration != nil {
		out.Expiration = new(*credentials.Expiration)
	}

	return out
}

// GetCredentials get credentials via assume role with SAML
func GetCredentials(ctx context.Context, input AwsSamlInput) (AwsSamlOutput, error) {
	awsInput, region := input.ToAwsInput()

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
		return AwsSamlOutput{}, fmt.Errorf("failed to assume role with SAML: %w", err)
	}

	return ToAwsSamlOutput(res.Credentials, region), nil
}

// ToEnv output AWS credentials as Environment variables
func (o *AwsSamlOutput) ToEnv() []string {
	return []string{
		fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", o.AccessKeyID),
		fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", o.SecretAccessKey),
		fmt.Sprintf("AWS_SESSION_TOKEN=%s", o.SessionToken),
		fmt.Sprintf("AWS_REGION=%s", o.Region),
		fmt.Sprintf("AWS_DEFAULT_REGION=%s", o.Region),
	}
}

// PrintEnv prepare environment variables output as text
func (o *AwsSamlOutput) PrintEnv() string {
	return strings.Join(o.ToEnv(), "\n") + "\n"
}

// ToAwsCredentials output as AWS profile
// If an input file exists, loading existing profiles and rewriting exist profile or adding a new
func (o *AwsSamlOutput) ToAwsCredentials(profileName string, inputIniFile string) ([]byte, error) {
	if profileName == "" {
		profileName = DefaultAwsProfileName
	}

	profile, err := ini.LooseLoad(inputIniFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", inputIniFile, err)
	}

	section, err := profile.NewSection(profileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile section %q: %w", profileName, err)
	}
	section.Key("aws_access_key_id").SetValue(o.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(o.SecretAccessKey)
	section.Key("aws_session_token").SetValue(o.SessionToken)
	if o.Expiration != nil {
		section.Key("expiration").SetValue(o.Expiration.Format(time.RFC3339))
	}

	var buf bytes.Buffer
	if _, err := profile.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to render credentials file: %w", err)
	}

	return buf.Bytes(), nil
}

// ToAwsConfig output as AWS profile
// If an input file exists, loading existing profiles and rewriting exist profile or adding a new
func (o *AwsSamlOutput) ToAwsConfig(profileName string, inputIniFile string) ([]byte, error) {
	if profileName == "" {
		profileName = DefaultAwsProfileName
	}
	if profileName != DefaultAwsProfileName {
		profileName = "profile " + profileName
	}

	profile, err := ini.LooseLoad(inputIniFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", inputIniFile, err)
	}

	section, err := profile.NewSection(profileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile section %q: %w", profileName, err)
	}
	section.Key("region").SetValue(o.Region)

	var buf bytes.Buffer
	if _, err := profile.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to render config file: %w", err)
	}

	return buf.Bytes(), nil
}
