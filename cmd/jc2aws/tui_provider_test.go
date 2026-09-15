package main

import (
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/yousysadmin/jc2aws/internal/cloud"
	"github.com/yousysadmin/jc2aws/internal/config"
)

// stepTitle returns the sidebar title currently shown for a step.
func stepTitle(m tuiModel, id stepID) string {
	for _, s := range m.steps {
		if s.id == id {
			return s.title
		}
	}
	return ""
}

func TestNewTuiModelAlwaysHasAProvider(t *testing.T) {
	resetViper()

	m := newTuiModel(newTestConfig(nil))
	if m.provider == nil {
		t.Fatal("newTuiModel left provider nil; the fetch goroutine would panic")
	}
	if m.provider.Name() != cloud.DefaultName {
		t.Errorf("provider = %q, want the default %q", m.provider.Name(), cloud.DefaultName)
	}
}

func TestRefreshProviderFollowsTheAccount(t *testing.T) {
	tests := []struct {
		name     string
		account  *config.Account
		wantName string
	}{
		{name: "nil account", account: nil, wantName: cloud.NameAWS},
		{name: "aws account", account: &config.Account{Provider: cloud.NameAWS}, wantName: cloud.NameAWS},
		{name: "alibaba account", account: &config.Account{Provider: cloud.NameAlibaba}, wantName: cloud.NameAlibaba},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()

			m := tuiModel{appCfg: newTestConfig(nil), steps: allStepMeta(), account: tt.account}
			m.refreshProvider()

			if m.provider == nil {
				t.Fatal("refreshProvider left provider nil")
			}
			if m.provider.Name() != tt.wantName {
				t.Errorf("provider = %q, want %q", m.provider.Name(), tt.wantName)
			}
		})
	}
}

func TestRefreshProviderFallsBackAndNotifies(t *testing.T) {
	resetViper()
	viper.Set(keyProvider, "gcp")

	m := tuiModel{appCfg: newTestConfig(nil), steps: allStepMeta()}
	m.refreshProvider()

	if m.provider == nil {
		t.Fatal("refreshProvider left provider nil on an unknown provider")
	}
	if m.provider.Name() != cloud.DefaultName {
		t.Errorf("provider = %q, want the default %q as a fallback", m.provider.Name(), cloud.DefaultName)
	}
	if m.notice == "" {
		t.Error("an unknown provider must surface a notice")
	}
}

func TestRetitleForProviderUsesVendorVocabulary(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		wantTitle string
	}{
		{name: "aws", provider: cloud.NameAWS, wantTitle: "Principal ARN"},
		{name: "alibaba", provider: cloud.NameAlibaba, wantTitle: "SAML Provider ARN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViper()

			m := tuiModel{
				appCfg:  newTestConfig(nil),
				steps:   allStepMeta(),
				account: &config.Account{Provider: tt.provider},
			}
			m.refreshProvider()

			if got := stepTitle(m, stepPrincipalARN); got != tt.wantTitle {
				t.Errorf("stepPrincipalARN title = %q, want %q", got, tt.wantTitle)
			}
		})
	}
}

func TestRegionsFollowTheProvider(t *testing.T) {
	resetViper()

	awsModel := tuiModel{appCfg: newTestConfig(nil), steps: allStepMeta(),
		account: &config.Account{Provider: cloud.NameAWS}}
	awsModel.refreshProvider()

	aliModel := tuiModel{appCfg: newTestConfig(nil), steps: allStepMeta(),
		account: &config.Account{Provider: cloud.NameAlibaba}}
	aliModel.refreshProvider()

	awsRegions := regionListForAccount(awsModel.account, awsModel.provider)
	aliRegions := regionListForAccount(aliModel.account, aliModel.provider)

	// The two vendors share several region IDs that name different places, so
	// the lists must never be interchangeable.
	if contains(awsRegions, "cn-hangzhou") {
		t.Error("AWS region list contains an Alibaba-only region")
	}
	if !contains(aliRegions, "cn-hangzhou") {
		t.Error("Alibaba region list is missing cn-hangzhou")
	}
	if contains(aliRegions, "us-gov-west-1") {
		t.Error("Alibaba region list contains an AWS-only region")
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestAccountSelectShowsProvider(t *testing.T) {
	resetViper()

	accounts := []config.Account{
		{Name: "prod", Provider: cloud.NameAWS},
		{Name: "prod-ali", Provider: cloud.NameAlibaba},
	}

	m := buildAccountSelect(accounts)
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.items))
	}

	want := []string{"AWS", "Alibaba Cloud"}
	for i, item := range m.items {
		if len(item.details) == 0 || item.details[0].key != "Provider" {
			t.Fatalf("item %d does not lead with a Provider detail: %+v", i, item.details)
		}
		if item.details[0].value != want[i] {
			t.Errorf("item %d Provider = %q, want %q", i, item.details[0].value, want[i])
		}
	}
}

func TestOutputFormatDescriptionsFollowTheProvider(t *testing.T) {
	resetViper()

	p, err := resolveProvider(&config.Account{Provider: cloud.NameAlibaba})
	if err != nil {
		t.Fatalf("resolveProvider() error = %v", err)
	}

	m := buildOutputFormatSelect(p)
	for _, item := range m.items {
		if item.name != "cli" {
			continue
		}
		if !strings.Contains(item.description, ".aliyun") {
			t.Errorf("cli description = %q, want it to name the Alibaba files", item.description)
		}
		return
	}
	t.Error("no cli entry in the output format selector")
}
