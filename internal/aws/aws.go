package aws

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
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

var DefaultAwsProfileName = "default"

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
		PrincipalArn:    aws.String(i.PrincipalArn),
		RoleArn:         aws.String(i.RoleArn),
		SAMLAssertion:   aws.String(i.SAMLAssertion),
		DurationSeconds: aws.Int32(i.DurationSeconds),
	}
	r = i.Region

	return s, r
}

// NewAwsSamlOutput converter from official AWS lib types to standart
func ToAwsSamlOutput(credentials *types.Credentials, region string) AwsSamlOutput {
	return AwsSamlOutput{
		AccessKeyID:     aws.ToString(credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(credentials.SecretAccessKey),
		SessionToken:    aws.ToString(credentials.SessionToken),
		Expiration:      credentials.Expiration,
		Region:          region,
	}
}

// GetCredentials get credentials via assume role with SAML
func GetCredentials(input AwsSamlInput) AwsSamlOutput {

	awsInput, region := input.ToAwsInput()

	ctx := context.TODO()
	cfg, err := config.LoadDefaultConfig(ctx)
	cfg.Region = region
	if err != nil {
		log.Fatal(err)
	}

	client := sts.NewFromConfig(cfg)

	res, err := client.AssumeRoleWithSAML(ctx, &awsInput)
	if err != nil {
		log.Fatal(err)
	}

	return ToAwsSamlOutput(res.Credentials, region)
}

// ToEnv output AWS credentials as Environment variables
func (o *AwsSamlOutput) ToEnv() ([]byte, error) {
	var buf bytes.Buffer
	_, err := fmt.Fprintf(&buf, "AWS_ACCESS_KEY_ID=%s\nAWS_SECRET_ACCESS_KEY=%s\nAWS_SESSION_TOKEN=%s\nAWS_REGION=%s\nAWS_DEFAULT_REGION=%s\n", o.AccessKeyID, o.SecretAccessKey, o.SessionToken, o.Region, o.Region)

	return buf.Bytes(), err
}

// ToProfile output as AWS profile
// If an input file exists, loading existing profiles and rewriting exist profile or adding a new
func (o *AwsSamlOutput) ToProfile(profileName string, inputIniFile string) ([]byte, error) {
	var buf bytes.Buffer

	profile, err := ini.LooseLoad(inputIniFile)
	if err != nil {
		return nil, err
	}
	section, _ := profile.NewSection(profileName)
	section.Key("aws_access_key_id").SetValue(o.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(o.SecretAccessKey)
	section.Key("aws_session_token").SetValue(o.SessionToken)
	section.Key("expiration").SetValue(o.Expiration.String())
	section.Key("region").SetValue(o.Region)

	_, err = profile.WriteTo(&buf)

	return buf.Bytes(), err
}
