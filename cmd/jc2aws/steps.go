package main

import (
	"fmt"
	"strings"

	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/config"
	"github.com/yousysadmin/jc2aws/internal/validators"
)

// stepID identifies each wizard step.
type stepID int

const (
	stepAccount stepID = iota
	stepRole
	stepRegion
	stepEmail
	stepPassword
	stepIdpURL
	stepPrincipalARN
	stepOutputFormat
	stepAwsCliProfile
	stepMFA
	stepConfirm
	stepFetching // credential fetching in progress
	stepDone     // all done
)

// Step value source constants.
const (
	sourceNone        = ""            // not yet set
	sourceInteractive = "interactive" // picked it in the TUI
	sourcePreset      = "preset"      // came from config, flag, or env var
)

// stepMeta holds display metadata for a step.
type stepMeta struct {
	id     stepID
	title  string
	value  string // filled in once the step completes
	source string // how the value was set: sourceNone, sourceInteractive, sourcePreset
}

func allStepMeta() []stepMeta {
	return []stepMeta{
		{id: stepAccount, title: "Account"},
		{id: stepRole, title: "Role"},
		{id: stepRegion, title: "Region"},
		{id: stepEmail, title: "Email"},
		{id: stepPassword, title: "Password"},
		{id: stepIdpURL, title: "IDP URL"},
		{id: stepPrincipalARN, title: "Principal ARN"},
		{id: stepOutputFormat, title: "Output Format"},
		{id: stepAwsCliProfile, title: "AWS CLI Profile"},
		{id: stepMFA, title: "MFA"},
		{id: stepConfirm, title: "Confirm"},
	}
}

// ---------------------------------------------------------------------------
// Factory functions: build the component model for a given step
// ---------------------------------------------------------------------------

// buildAccountSelect creates a selectModel for account selection.
func buildAccountSelect(accounts []config.Account) selectModel {
	var items []selectItem
	for _, a := range accounts {
		roles := make([]string, 0, len(a.AWSRoleArns))
		for _, r := range a.AWSRoleArns {
			roles = append(roles, r.Name)
		}

		var details []detailPair
		if len(roles) > 0 {
			details = append(details, detailPair{"Roles", strings.Join(roles, ", ")})
		}
		if len(a.AWSRegions) > 0 {
			details = append(details, detailPair{"Regions", strings.Join(a.AWSRegions, ", ")})
		}
		if a.Email != "" {
			details = append(details, detailPair{"Email", "Present"})
		} else {
			details = append(details, detailPair{"Email", "Not present"})
		}
		if a.Password != "" {
			details = append(details, detailPair{"Password", "Present"})
		} else {
			details = append(details, detailPair{"Password", "Not present"})
		}
		if a.MFASecret != "" {
			details = append(details, detailPair{"MFA", "Present"})
		} else {
			details = append(details, detailPair{"MFA", "Not present"})
		}
		if a.Duration > 0 {
			details = append(details, detailPair{"Duration", fmt.Sprintf("%d", a.Duration)})
		}

		items = append(items, selectItem{
			name:        a.Name,
			description: a.Description,
			details:     details,
		})
	}
	return newSelectModel("Select account:", items)
}

// buildRoleSelect creates a selectModel for role ARN selection.
func buildRoleSelect(account config.Account) selectModel {
	var items []selectItem
	for _, r := range account.AWSRoleArns {
		details := []detailPair{
			{"ARN", r.Arn},
		}
		items = append(items, selectItem{
			name:        r.Name,
			description: r.Description,
			details:     details,
		})
	}
	return newSelectModel("Select role:", items)
}

// buildRegionSelect creates a selectModel for AWS region selection.
func buildRegionSelect(regions []string) selectModel {
	var items []selectItem
	for _, r := range regions {
		items = append(items, selectItem{name: r})
	}
	return newSelectModel("Select region:", items)
}

// buildOutputFormatSelect creates a selectModel for output format selection.
func buildOutputFormatSelect() selectModel {
	items := []selectItem{
		{name: "cli", description: "Write to ~/.aws/credentials and ~/.aws/config"},
		{name: "env", description: "Write to ~/.jc2aws.env"},
		{name: "cli-stdout", description: "Print AWS CLI credentials to stdout"},
		{name: "env-stdout", description: "Print environment variables to stdout"},
		{name: "shell", description: "Launch a shell with AWS credentials as env vars"},
	}
	return newSelectModel("Select output format:", items)
}

// Input builders use shared validators.

func buildEmailInput() inputModel {
	return newInputModel("Email", false, validators.Get("email"))
}

func buildPasswordInput() inputModel {
	return newInputModel("Password", true, validators.Get("password"))
}

func buildIdpURLInput() inputModel {
	return newInputModel("IDP URL", false, validators.Get("idp-url"))
}

func buildPrincipalARNInput() inputModel {
	return newInputModel("Principal ARN", false, validators.Get("principal-arn"))
}

func buildRoleARNInput() inputModel {
	return newInputModel("Role ARN", false, validators.Get("role-arn"))
}

func buildAwsCliProfileInput() inputModel {
	return newInputModel("AWS CLI Profile Name", false, validators.Get("skip"))
}

func buildMFAInput() inputModel {
	return newInputModel("MFA Token or MFA Secret", false, validators.Get("skip"))
}

// regionListForAccount returns account-specific regions if available, else full list.
func regionListForAccount(account *config.Account) []string {
	if account != nil && len(account.AWSRegions) > 0 {
		return account.AWSRegions
	}
	return aws.RegionsList
}
