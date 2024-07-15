package config

import (
	"fmt"
	"slices"
)

// Account store information about configured AWS accounts
type Account struct {
	Name            string    `yaml:"name"`
	AwsCliProfile   string    `yaml:"aws_cli_profile"`
	Description     string    `yaml:"description"`
	Email           string    `yaml:"email"`
	Password        string    `yaml:"password"`
	MFASecret       string    `yaml:"mfa_token_secret"`
	AWSPrincipalArn string    `yaml:"aws_principal_arn"`
	AWSRoleArns     []AWSRole `yaml:"aws_role_arns"`
	AWSRegions      []string  `yaml:"aws_regions"`
	IdpURL          string    `yaml:"jc_idp_url"`
	Duration        int       `yaml:"session_timeout"`
}

// FindAWSRoleArnByName return account by account name from accounts list
func (a *Account) FindAWSRoleArnByName(name string) (role AWSRole, err error) {
	idx := slices.IndexFunc(a.AWSRoleArns, func(r AWSRole) bool { return r.Name == name })
	if idx < 0 {
		return role, fmt.Errorf("the role %s not found", name)
	}
	return a.AWSRoleArns[idx], nil
}

// GetName account Name getter
func (a *Account) GetName() string {
	return a.Name
}

// GetAwsCliProfile AwsCliProfile getter
func (a *Account) GetAwsCliProfile() string {
	return a.AwsCliProfile
}

// GetEmail account Email getter
func (a *Account) GetEmail() string {
	return a.Email
}

// GetPassword account Password getter
func (a *Account) GetPassword() string {
	return a.Password
}

// GetMFATokenSecret account MFATokenSecret getter
func (a *Account) GetMFATokenSecret() string {
	return a.MFASecret
}

// GetAWSPrincipalArn account AWSPrincipalArn getter
func (a *Account) GetAWSPrincipalArn() string {
	return a.AWSPrincipalArn
}

// GetAWSRoleArns account AWSRoleArns getter
func (a *Account) GetAWSRoleArns() []AWSRole {
	return a.AWSRoleArns
}

// GetAWSRegions account AWSRoleArns getter
func (a *Account) GetAWSRegions() []string {
	return a.AWSRegions
}

// GetJcIdpURL account JcIdpURL getter
func (a *Account) GetJcIdpURL() string {
	return a.IdpURL
}

// GetSessionTimeout account SessionTimeout getter
func (a *Account) GetSessionTimeout() int {
	return a.Duration
}
