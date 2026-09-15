package main

import (
	"strconv"
	"strings"

	"github.com/yousysadmin/jc2aws/internal/cloud"
	"github.com/yousysadmin/jc2aws/internal/cloud/providers"
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
	stepCLIProfile
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
		{id: stepPrincipalARN, title: "Identity Provider ARN"},
		{id: stepOutputFormat, title: "Output Format"},
		{id: stepCLIProfile, title: "CLI Profile"},
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
		roles := make([]string, 0, len(a.Roles))
		for _, r := range a.Roles {
			roles = append(roles, r.Name)
		}

		details := []detailPair{{"Provider", providerDisplayName(a.Provider)}}
		if len(roles) > 0 {
			details = append(details, detailPair{"Roles", strings.Join(roles, ", ")})
		}
		if len(a.Regions) > 0 {
			details = append(details, detailPair{"Regions", strings.Join(a.Regions, ", ")})
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
			details = append(details, detailPair{"Duration", strconv.Itoa(a.Duration)})
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
	for _, r := range account.Roles {
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

// buildRegionSelect creates a selectModel for cloud region selection.
func buildRegionSelect(regions []string) selectModel {
	var items []selectItem
	for _, r := range regions {
		items = append(items, selectItem{name: r})
	}
	return newSelectModel("Select region:", items)
}

// buildOutputFormatSelect creates a selectModel for output format selection,
// describing the vendor-specific formats in the provider's own terms.
func buildOutputFormatSelect(p cloud.Provider) selectModel {
	cliDescription := "Write the vendor CLI credential files"
	vendor := "cloud"
	if p != nil {
		cliDescription = p.Info().CLIDescription
		vendor = p.Info().DisplayName
	}

	items := []selectItem{
		{name: "cli", description: cliDescription},
		{name: "env", description: "Write to ~/.jc2aws.env"},
		{name: "cli-stdout", description: "Print " + vendor + " CLI credentials to stdout"},
		{name: "env-stdout", description: "Print environment variables to stdout"},
		{name: "shell", description: "Launch a shell with " + vendor + " credentials as env vars"},
	}
	return newSelectModel("Select output format:", items)
}

// Input builders use shared validators.

func buildEmailInput() inputModel {
	return newInputModel("Email", false, validators.Get(validators.KeyEmail))
}

func buildPasswordInput() inputModel {
	return newInputModel("Password", true, validators.Get(validators.KeyPassword))
}

func buildIdpURLInput() inputModel {
	return newInputModel("IDP URL", false, validators.Get(validators.KeyIdpURL))
}

func buildPrincipalARNInput(p cloud.Provider) inputModel {
	label := "Identity Provider ARN"
	if p != nil {
		label = p.Info().ProviderARNLabel
	}
	return newInputModel(label, false, validators.ProviderAware(validators.KeyProviderARN, p))
}

func buildRoleARNInput(p cloud.Provider) inputModel {
	label := "Role ARN"
	if p != nil {
		label = p.Info().RoleARNLabel
	}
	return newInputModel(label, false, validators.ProviderAware(validators.KeyRoleARN, p))
}

func buildCLIProfileInput() inputModel {
	return newInputModel("CLI Profile Name", false, validators.Get(validators.KeySkip))
}

func buildMFAInput() inputModel {
	return newInputModel("MFA Token or MFA Secret", false, validators.Get(validators.KeySkip))
}

// regionListForAccount returns account-specific regions if configured,
// otherwise the provider's built-in list. A nil provider yields no regions,
// which leaves the picker empty rather than offering another vendor's regions.
func regionListForAccount(account *config.Account, p cloud.Provider) []string {
	if account != nil && len(account.Regions) > 0 {
		return account.Regions
	}
	if p == nil {
		return nil
	}
	return p.Regions()
}

// providerDisplayName renders a provider name for the account picker, falling
// back to the raw value so an unconstructible provider never breaks rendering.
func providerDisplayName(name string) string {
	p, err := providers.Get(name)
	if err != nil {
		return cloud.Normalize(name)
	}
	return p.Info().DisplayName
}
