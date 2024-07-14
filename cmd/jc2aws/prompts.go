package main

import (
	"errors"
	"github.com/manifoldco/promptui"
	"github.com/yousysadmin/jc2aws/internal/config"
	"os"
	"strings"
)

// PromptAccount prompt account from account list
func PromptAccount(accounts []config.Account) (account config.Account, err error) {
	selectTemplate := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "> {{ .Name }}",
		Inactive: "  {{ .Name }} ",
		Selected: "> {{ .Name | cyan }}",
		Details: `
--------- Account Properties ----------
{{ "Description:" | faint }}	{{ .Description }}
{{ "Roles:" | faint }}	{{ range $i, $r := .AWSRoleArns }}{{ if $i }}, {{ end }}{{ $r.Name }}{{ end }}
{{ "Regions:" | faint }}	{{ range $i, $r := .AWSRegions }}{{ if $i }}, {{ end }}{{ . }}{{ end }}
{{ "E-mail" | faint }}	{{ if eq (len .Email) 0 }}Not present{{else}}Present{{ end }}
{{ "Password" | faint }}	{{ if eq (len .Password) 0 }}Not present{{ else }}Present{{ end }}
{{ "MFA" | faint }}	{{ if eq (len .MFASecret) 0 }}Not present{{ else }}Present{{ end }}
{{ "Duration:" | faint }}	{{ .Duration }}`,
	}

	searcher := func(input string, index int) bool {
		account := accounts[index]
		name := strings.Replace(strings.ToLower(account.Name), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)

		return strings.Contains(name, input)
	}

	prompt := promptui.Select{
		Label:     "Select account:",
		Items:     accounts,
		Templates: selectTemplate,
		Size:      len(accounts),
		Searcher:  searcher,
	}

	idx, _, err := prompt.Run()

	if err != nil {
		return account, err
	}

	return accounts[idx], err
}

// PromptRegion prompt AWS region from a list
func PromptRegion(regionList []string) (region string, err error) {
	return PromptSimpleSelect("Select region:", regionList)
}

// PromptRoleArn prompt AWS role
func PromptRoleArn(account config.Account) (arn string, err error) {
	selectTemplate := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "> {{ .Name }}",
		Inactive: "  {{ .Name }} ",
		Selected: "> {{ .Name | cyan }}",
		Details: `
--------- Role Properties ----------
{{ "Description:" | faint }}	{{ .Description }}
{{ "ARN:" | faint }}	{{ .Arn }}`,
	}

	searcher := func(input string, index int) bool {
		role := account.AWSRoleArns[index]
		name := strings.Replace(strings.ToLower(role.Name), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)

		return strings.Contains(name, input)
	}

	prompt := promptui.Select{
		Label:     "Select Role:",
		Items:     account.AWSRoleArns,
		Templates: selectTemplate,
		Size:      len(account.AWSRoleArns),
		Searcher:  searcher,
	}

	idx, _, err := prompt.Run()

	if err != nil {
		return "", err
	}

	return account.AWSRoleArns[idx].Arn, err
}

// PromptSimple simple prompt manual user input
func PromptSimple(label, validator string, promptType string) (answer string, err error) {

	promtTypes := map[string]promptui.Prompt{
		"simple": {
			Label:    label,
			Validate: validators[validator],
		},
		"masked": {
			Label:    label,
			Validate: validators[validator],
			Mask:     '*',
		},
	}

	prompt, ok := promtTypes[promptType]
	if !ok {
		return "", errors.New("invalid prompt type")
	}

	answer, err = prompt.Run()

	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			os.Exit(-1)
		}
		return "", err
	}
	return answer, nil
}

// PromptSimpleSelect simple select from a slice
func PromptSimpleSelect(label string, list []string) (result string, err error) {
	prompt := promptui.Select{
		Label: label,
		Items: list,
	}

	_, result, err = prompt.Run()

	if err != nil {
		return result, err
	}

	return result, err
}
