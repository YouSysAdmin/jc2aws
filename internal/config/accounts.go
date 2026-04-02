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
	Duration        int       `yaml:"session_duration"`
	// Deprecated: use session_duration instead. Will be removed in a future release.
	SessionTimeout int `yaml:"session_timeout"`
}

// AWSRole store information about aws roles
type AWSRole struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Arn         string `yaml:"arn"`
}

// FindAWSRoleArnByName return account by account name from accounts list
func (a *Account) FindAWSRoleArnByName(name string) (role AWSRole, err error) {
	idx := slices.IndexFunc(a.AWSRoleArns, func(r AWSRole) bool { return r.Name == name })
	if idx < 0 {
		return role, fmt.Errorf("the role %s not found", name)
	}
	return a.AWSRoleArns[idx], nil
}
