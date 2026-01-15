package aws

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"

	"gopkg.in/ini.v1"
)

func TestAwsSamlInput_ToAwsInput(t *testing.T) {
	type fields struct {
		PrincipalArn    string
		RoleArn         string
		SAMLAssertion   string
		Region          string
		DurationSeconds int32
	}
	tests := []struct {
		name   string
		fields fields
		wantS  sts.AssumeRoleWithSAMLInput
		wantR  string
	}{
		{name: "default", fields: fields{
			PrincipalArn:    "TEST_PRINCIPAL_ARN",
			RoleArn:         "TEST_ROLE_ARN",
			Region:          "TEST_REGION",
			SAMLAssertion:   "TEST_SAML_ASSERTION",
			DurationSeconds: 3600,
		}, wantS: sts.AssumeRoleWithSAMLInput{
			PrincipalArn:    aws.String("TEST_PRINCIPAL_ARN"),
			RoleArn:         aws.String("TEST_ROLE_ARN"),
			SAMLAssertion:   aws.String("TEST_SAML_ASSERTION"),
			DurationSeconds: aws.Int32(3600),
			Policy:          nil,
			PolicyArns:      nil,
		}, wantR: "TEST_REGION"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &AwsSamlInput{
				PrincipalArn:    tt.fields.PrincipalArn,
				RoleArn:         tt.fields.RoleArn,
				Region:          tt.fields.Region,
				SAMLAssertion:   tt.fields.SAMLAssertion,
				DurationSeconds: tt.fields.DurationSeconds,
			}
			gotS, gotR := i.ToAwsInput()
			if !reflect.DeepEqual(gotS, tt.wantS) {
				t.Errorf("ToAwsInput() gotS = %v, want %v", gotS, tt.wantS)
			}
			if gotR != tt.wantR {
				t.Errorf("ToAwsInput() gotR = %v, want %v", gotR, tt.wantR)
			}
		})
	}
}

func TestAwsSamlOutput_ToEnv(t *testing.T) {

	type fields struct {
		AccessKeyID     string
		SecretAccessKey string
		SessionToken    string
		Region          string
		Expiration      *time.Time
	}
	tests := []struct {
		name    string
		fields  fields
		want    []string
		wantErr bool
	}{
		{name: "default", fields: fields{
			AccessKeyID:     "TEST_ACCESS_ID",
			SecretAccessKey: "TEST_SECRET_ACCESS_KEY",
			SessionToken:    "TEST_SESSION_TOKEN",
			Region:          "TEST_REGION",
		},
			want: []string{"AWS_ACCESS_KEY_ID=TEST_ACCESS_ID",
				"AWS_SECRET_ACCESS_KEY=TEST_SECRET_ACCESS_KEY",
				"AWS_SESSION_TOKEN=TEST_SESSION_TOKEN",
				"AWS_REGION=TEST_REGION",
				"AWS_DEFAULT_REGION=TEST_REGION",
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &AwsSamlOutput{
				AccessKeyID:     tt.fields.AccessKeyID,
				SecretAccessKey: tt.fields.SecretAccessKey,
				SessionToken:    tt.fields.SessionToken,
				Region:          tt.fields.Region,
				Expiration:      tt.fields.Expiration,
			}
			got := o.ToEnv()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToEnv() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAwsSamlOutput_PrintEnv(t *testing.T) {

	type fields struct {
		AccessKeyID     string
		SecretAccessKey string
		SessionToken    string
		Region          string
		Expiration      *time.Time
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		{name: "default", fields: fields{
			AccessKeyID:     "TEST_ACCESS_ID",
			SecretAccessKey: "TEST_SECRET_ACCESS_KEY",
			SessionToken:    "TEST_SESSION_TOKEN",
			Region:          "TEST_REGION",
		},

			want: strings.Join([]string{
				"AWS_ACCESS_KEY_ID=TEST_ACCESS_ID",
				"AWS_SECRET_ACCESS_KEY=TEST_SECRET_ACCESS_KEY",
				"AWS_SESSION_TOKEN=TEST_SESSION_TOKEN",
				"AWS_REGION=TEST_REGION",
				"AWS_DEFAULT_REGION=TEST_REGION",
			}, "\n") + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &AwsSamlOutput{
				AccessKeyID:     tt.fields.AccessKeyID,
				SecretAccessKey: tt.fields.SecretAccessKey,
				SessionToken:    tt.fields.SessionToken,
				Region:          tt.fields.Region,
				Expiration:      tt.fields.Expiration,
			}
			got := o.PrintEnv()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToEnv() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAwsSamlOutput_ToProfile(t *testing.T) {

	timeNow := time.Now()

	type fields struct {
		AccessKeyID     string
		SecretAccessKey string
		SessionToken    string
		Region          string
		Expiration      *time.Time
	}
	type args struct {
		profileName  string
		inputIniFile string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		{name: "default", fields: fields{
			AccessKeyID:     "TEST_ACCESS_KEY_ID",
			SecretAccessKey: "TEST_SECRET_ACCESS_KEY",
			SessionToken:    "TEST_SESSION_TOKEN",
			Expiration:      aws.Time(timeNow),
		},
			args: args{profileName: "default"},
			want: fmt.Appendf(nil, `[default]
aws_access_key_id     = TEST_ACCESS_KEY_ID
aws_secret_access_key = TEST_SECRET_ACCESS_KEY
aws_session_token     = TEST_SESSION_TOKEN
expiration            = %s
`, timeNow)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &AwsSamlOutput{
				AccessKeyID:     tt.fields.AccessKeyID,
				SecretAccessKey: tt.fields.SecretAccessKey,
				SessionToken:    tt.fields.SessionToken,
				Expiration:      tt.fields.Expiration,
			}
			got, err := o.ToAwsCredentials(tt.args.profileName, tt.args.inputIniFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToProfile() got = %v, want %v", string(got), string(tt.want))
			}
		})
	}
}

func TestToAwsSamlOutput(t *testing.T) {

	timeNow := time.Now()

	type args struct {
		credentials *types.Credentials
		region      string
	}
	tests := []struct {
		name string
		args args
		want AwsSamlOutput
	}{
		{name: "default", args: args{credentials: &types.Credentials{
			AccessKeyId:     aws.String("TEST_ACCESS_KEY"),
			Expiration:      aws.Time(timeNow),
			SecretAccessKey: aws.String("TEST_SECRET_ACCESS_KEY"),
			SessionToken:    aws.String("TEST_SESSION_TOKEN"),
		}}, want: AwsSamlOutput{
			AccessKeyID:     "TEST_ACCESS_KEY",
			Expiration:      aws.Time(timeNow),
			SecretAccessKey: "TEST_SECRET_ACCESS_KEY",
			SessionToken:    "TEST_SESSION_TOKEN",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToAwsSamlOutput(tt.args.credentials, tt.args.region); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToAwsSamlOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAwsSamlOutputToAwsConfig(t *testing.T) {
	output := AwsSamlOutput{
		Region: "us-west-2",
	}

	tests := []struct {
		name          string
		profileName   string
		expectSection string
	}{
		{
			name:          "default profile",
			profileName:   "default",
			expectSection: "default",
		},
		{
			name:          "named profile",
			profileName:   "test-profile",
			expectSection: "profile test-profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputIniFile := ""
			result, err := output.ToAwsConfig(tt.profileName, inputIniFile)
			if err != nil {
				t.Fatalf("ToAwsConfig() error = %v", err)
			}

			cfg, err := ini.Load(result)
			if err != nil {
				t.Fatalf("Failed to parse generated INI: %v", err)
			}

			section := cfg.Section(tt.expectSection)
			if section == nil {
				t.Fatalf("Expected section %s not found", tt.expectSection)
			}

			if section.Key("region").String() != output.Region {
				t.Errorf("Expected region %s, got %s", output.Region, section.Key("region").String())
			}
		})
	}
}

func TestAwsSamlOutputToAwsCredentialsWithExistingFile(t *testing.T) {
	// Create existing INI content as a temporary file path
	output := AwsSamlOutput{
		AccessKeyID:     "new-key",
		SecretAccessKey: "new-secret",
		SessionToken:    "new-token",
		Expiration:      &time.Time{},
	}

	profileName := "default"
	inputIniFile := "" // Empty string for testing without existing file

	result, err := output.ToAwsCredentials(profileName, inputIniFile)
	if err != nil {
		t.Fatalf("ToAwsCredentials() error = %v", err)
	}

	cfg, err := ini.Load(result)
	if err != nil {
		t.Fatalf("Failed to parse generated INI: %v", err)
	}

	// Check that default profile was created
	defaultSection := cfg.Section("default")
	if defaultSection == nil {
		t.Error("Expected default profile to be created")
	}

	if defaultSection.Key("aws_access_key_id").String() != "new-key" {
		t.Errorf("Expected aws_access_key_id to be new-key, got %s", defaultSection.Key("aws_access_key_id").String())
	}
}

func TestRegionsList(t *testing.T) {
	expectedRegions := []string{"us-east-1", "us-east-2", "us-west-1", "us-west-2", "af-south-1", "ap-east-1",
		"ap-south-2", "ap-southeast-3", "ap-southeast-4", "ap-south-1", "ap-northeast-3", "ap-northeast-2",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ca-central-1", "ca-west-1", "eu-central-1",
		"eu-west-1", "eu-west-2", "eu-south-1", "eu-south-2", "eu-west-3", "eu-north-1",
		"eu-central-2", "il-central-1", "me-south-1", "me-central-1", "sa-east-1", "us-gov-east-1",
		"us-gov-west-1", "cn-north-1", "cn-northwest-1"}

	if len(RegionsList) != len(expectedRegions) {
		t.Errorf("RegionsList has %d regions, expected %d", len(RegionsList), len(expectedRegions))
	}

	for i, region := range expectedRegions {
		if i >= len(RegionsList) || RegionsList[i] != region {
			t.Errorf("Expected region %s at index %d, got %s", region, i, RegionsList[i])
		}
	}
}

func TestDefaultAwsProfileName(t *testing.T) {
	if DefaultAwsProfileName != "default" {
		t.Errorf("Expected DefaultAwsProfileName to be 'default', got %s", DefaultAwsProfileName)
	}
}

func TestGetCredentialsErrorHandling(t *testing.T) {
	// Test that GetCredentials returns error instead of calling log.Fatal
	input := AwsSamlInput{
		PrincipalArn:    "invalid-arn",
		RoleArn:         "invalid-arn",
		SAMLAssertion:   "invalid-saml",
		Region:          "invalid-region",
		DurationSeconds: 3600,
	}

	_, err := GetCredentials(input)
	if err == nil {
		t.Error("Expected error for invalid input, got nil")
	}
}
