package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"reflect"
	"strings"
	"testing"
	"time"
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
			want: `AWS_ACCESS_KEY_ID=TEST_ACCESS_ID
                AWS_SECRET_ACCESS_KEY=TEST_SECRET_ACCESS_KEY
                AWS_SESSION_TOKEN=TEST_SESSION_TOKEN
                AWS_REGION=TEST_REGION
                AWS_DEFAULT_REGION=TEST_REGION`,
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
			if !reflect.DeepEqual(got, strings.ReplaceAll(tt.want, " ", "")) {
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
			Region:          "TEST_REGION",
			Expiration:      aws.Time(timeNow),
		},
			args: args{profileName: "default"},
			want: []byte(fmt.Sprintf(`[default]
aws_access_key_id     = TEST_ACCESS_KEY_ID
aws_secret_access_key = TEST_SECRET_ACCESS_KEY
aws_session_token     = TEST_SESSION_TOKEN
expiration            = %s
region                = TEST_REGION
`, timeNow))},
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
			got, err := o.ToProfile(tt.args.profileName, tt.args.inputIniFile)
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
