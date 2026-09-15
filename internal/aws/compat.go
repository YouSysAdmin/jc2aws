package aws

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// This file is a temporary compatibility shim that keeps the pre-cloud.Provider
// API alive while the CLI, the TUI and the validators are migrated package by
// package. Everything here is deleted once cmd/jc2aws routes through
// cloud.Provider; do not add to it.

// AwsSamlOutput struct for storing prepared AWS credentials
//
// Deprecated: use cloud.Credentials.
type AwsSamlOutput struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	Expiration      *time.Time
}

// AwsSamlInput struct for input parameters for next used with the official AWS lib
//
// Deprecated: use cloud.SAMLInput.
type AwsSamlInput struct {
	PrincipalArn    string
	RoleArn         string
	SAMLAssertion   string
	Region          string
	DurationSeconds int32
}

// toCloud converts the deprecated input struct into the neutral one.
func (i *AwsSamlInput) toCloud() cloud.SAMLInput {
	return cloud.SAMLInput{
		ProviderARN:     i.PrincipalArn,
		RoleARN:         i.RoleArn,
		SAMLAssertion:   i.SAMLAssertion,
		Region:          i.Region,
		DurationSeconds: i.DurationSeconds,
	}
}

// toCloud converts the deprecated output struct into the neutral one.
func (o *AwsSamlOutput) toCloud() cloud.Credentials {
	return cloud.Credentials{
		Provider:        cloud.NameAWS,
		AccessKeyID:     o.AccessKeyID,
		SecretAccessKey: o.SecretAccessKey,
		SessionToken:    o.SessionToken,
		Region:          o.Region,
		Expiration:      o.Expiration,
	}
}

// fromCloud converts the neutral credentials into the deprecated output struct.
func fromCloud(cred cloud.Credentials) AwsSamlOutput {
	return AwsSamlOutput{
		AccessKeyID:     cred.AccessKeyID,
		SecretAccessKey: cred.SecretAccessKey,
		SessionToken:    cred.SessionToken,
		Region:          cred.Region,
		Expiration:      cred.Expiration,
	}
}

// ToAwsInput converter from standard types to official AWS lib types
//
// Deprecated: use the Provider methods.
func (i *AwsSamlInput) ToAwsInput() (s sts.AssumeRoleWithSAMLInput, r string) {
	return toSTSInput(i.toCloud())
}

// ToAwsSamlOutput converter from official AWS lib types to standart
//
// Deprecated: use the Provider methods.
func ToAwsSamlOutput(credentials *types.Credentials, region string) AwsSamlOutput {
	return fromCloud(toCredentials(credentials, region))
}

// GetCredentials get credentials via assume role with SAML
//
// Deprecated: use Provider.AssumeRoleWithSAML.
func GetCredentials(ctx context.Context, input AwsSamlInput) (AwsSamlOutput, error) {
	cred, err := New().AssumeRoleWithSAML(ctx, input.toCloud())
	if err != nil {
		return AwsSamlOutput{}, err
	}
	return fromCloud(cred), nil
}

// ToEnv output AWS credentials as Environment variables
//
// Deprecated: use Provider.Env.
func (o *AwsSamlOutput) ToEnv() []string {
	return New().Env(o.toCloud())
}

// PrintEnv prepare environment variables output as text
//
// Deprecated: use cloud.EnvString.
func (o *AwsSamlOutput) PrintEnv() string {
	return strings.Join(o.ToEnv(), "\n") + "\n"
}

// ToAwsCredentials output as AWS profile
//
// Deprecated: use Provider.CLIFiles or Provider.StdoutProfile.
func (o *AwsSamlOutput) ToAwsCredentials(profileName string, inputIniFile string) ([]byte, error) {
	return renderCredentialsINI(profileName, inputIniFile, o.toCloud())
}

// ToAwsConfig output as AWS profile
//
// Deprecated: use Provider.CLIFiles.
func (o *AwsSamlOutput) ToAwsConfig(profileName string, inputIniFile string) ([]byte, error) {
	return renderConfigINI(profileName, inputIniFile, o.toCloud())
}
