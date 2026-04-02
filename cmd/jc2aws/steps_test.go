package main

import (
	"testing"

	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/config"
)

// ---------------------------------------------------------------------------
// allStepMeta tests
// ---------------------------------------------------------------------------

func TestAllStepMetaOrder(t *testing.T) {
	steps := allStepMeta()

	expectedIDs := []stepID{
		stepAccount, stepRole, stepRegion, stepEmail,
		stepPassword, stepIdpURL, stepPrincipalARN,
		stepOutputFormat, stepAwsCliProfile, stepMFA, stepConfirm,
	}

	if len(steps) != len(expectedIDs) {
		t.Fatalf("expected %d steps, got %d", len(expectedIDs), len(steps))
	}

	for i, expected := range expectedIDs {
		if steps[i].id != expected {
			t.Errorf("step[%d]: expected id %d, got %d", i, expected, steps[i].id)
		}
	}
}

func TestAllStepMetaInitialValues(t *testing.T) {
	steps := allStepMeta()
	for _, s := range steps {
		if s.value != "" {
			t.Errorf("step %q should have empty initial value, got %q", s.title, s.value)
		}
		if s.source != sourceNone {
			t.Errorf("step %q should have source=sourceNone, got %q", s.title, s.source)
		}
	}
}

func TestAllStepMetaTitlesNonEmpty(t *testing.T) {
	steps := allStepMeta()
	for _, s := range steps {
		if s.title == "" {
			t.Errorf("step with id %d has empty title", s.id)
		}
	}
}

// ---------------------------------------------------------------------------
// Factory function tests
// ---------------------------------------------------------------------------

// detailValue looks up a detail pair by key from a []detailPair slice.
func detailValue(details []detailPair, key string) string {
	for _, d := range details {
		if d.key == key {
			return d.value
		}
	}
	return ""
}

func TestBuildAccountSelect(t *testing.T) {
	accounts := []config.Account{
		{
			Name:        "prod",
			Description: "Production account",
			Email:       "user@example.com",
			Password:    "secret",
			MFASecret:   "TOTP123",
			AWSRoleArns: []config.AWSRole{
				{Name: "admin", Arn: "arn:aws:iam::111:role/admin"},
			},
			AWSRegions: []string{"us-east-1", "eu-west-1"},
			Duration:   7200,
		},
		{
			Name: "staging",
		},
	}

	m := buildAccountSelect(accounts)

	if m.label != "Select account:" {
		t.Errorf("expected label 'Select account:', got %q", m.label)
	}
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.items))
	}

	// First item should have rich details
	item0 := m.items[0]
	if item0.name != "prod" {
		t.Errorf("expected name 'prod', got %q", item0.name)
	}
	if item0.description != "Production account" {
		t.Errorf("expected description 'Production account', got %q", item0.description)
	}
	if v := detailValue(item0.details, "Email"); v != "Present" {
		t.Errorf("expected Email=Present, got %q", v)
	}
	if v := detailValue(item0.details, "Password"); v != "Present" {
		t.Errorf("expected Password=Present, got %q", v)
	}
	if v := detailValue(item0.details, "MFA"); v != "Present" {
		t.Errorf("expected MFA=Present, got %q", v)
	}

	// Second item should have "Not present" for missing fields
	item1 := m.items[1]
	if v := detailValue(item1.details, "Email"); v != "Not present" {
		t.Errorf("expected Email='Not present', got %q", v)
	}
}

func TestBuildRoleSelect(t *testing.T) {
	account := config.Account{
		AWSRoleArns: []config.AWSRole{
			{Name: "admin", Description: "Admin role", Arn: "arn:aws:iam::111:role/admin"},
			{Name: "readonly", Arn: "arn:aws:iam::111:role/readonly"},
		},
	}

	m := buildRoleSelect(account)

	if m.label != "Select role:" {
		t.Errorf("expected label 'Select role:', got %q", m.label)
	}
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.items))
	}
	if m.items[0].name != "admin" {
		t.Errorf("expected first item 'admin', got %q", m.items[0].name)
	}
	if detailValue(m.items[0].details, "ARN") != "arn:aws:iam::111:role/admin" {
		t.Errorf("expected ARN in details")
	}
}

func TestBuildRegionSelect(t *testing.T) {
	regions := []string{"us-east-1", "eu-west-1"}
	m := buildRegionSelect(regions)

	if m.label != "Select region:" {
		t.Errorf("expected label 'Select region:', got %q", m.label)
	}
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.items))
	}
	if m.items[0].name != "us-east-1" {
		t.Errorf("expected 'us-east-1', got %q", m.items[0].name)
	}
}

func TestBuildOutputFormatSelect(t *testing.T) {
	m := buildOutputFormatSelect()

	if m.label != "Select output format:" {
		t.Errorf("expected label 'Select output format:', got %q", m.label)
	}
	if len(m.items) != 5 {
		t.Fatalf("expected 5 output format items, got %d", len(m.items))
	}

	names := make(map[string]bool)
	for _, item := range m.items {
		names[item.name] = true
	}
	for _, expected := range []string{"cli", "env", "cli-stdout", "env-stdout", "shell"} {
		if !names[expected] {
			t.Errorf("expected output format %q in items", expected)
		}
	}
}

func TestBuildInputFactories(t *testing.T) {
	tests := []struct {
		name    string
		builder func() inputModel
		label   string
		masked  bool
	}{
		{"email", buildEmailInput, "Email", false},
		{"password", buildPasswordInput, "Password", true},
		{"idp-url", buildIdpURLInput, "IDP URL", false},
		{"principal-arn", buildPrincipalARNInput, "Principal ARN", false},
		{"role-arn", buildRoleARNInput, "Role ARN", false},
		{"aws-cli-profile", buildAwsCliProfileInput, "AWS CLI Profile Name", false},
		{"mfa", buildMFAInput, "MFA Token or MFA Secret", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.builder()
			if m.label != tt.label {
				t.Errorf("expected label %q, got %q", tt.label, m.label)
			}
			if m.isMasked != tt.masked {
				t.Errorf("expected masked=%v, got %v", tt.masked, m.isMasked)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// regionListForAccount tests
// ---------------------------------------------------------------------------

func TestRegionListForAccountWithRegions(t *testing.T) {
	acc := &config.Account{
		AWSRegions: []string{"us-east-1", "eu-west-1"},
	}
	result := regionListForAccount(acc)
	if len(result) != 2 {
		t.Errorf("expected 2 regions, got %d", len(result))
	}
}

func TestRegionListForAccountWithoutRegions(t *testing.T) {
	acc := &config.Account{}
	result := regionListForAccount(acc)
	if len(result) != len(aws.RegionsList) {
		t.Errorf("expected full regions list (%d), got %d", len(aws.RegionsList), len(result))
	}
}

func TestRegionListForAccountNilAccount(t *testing.T) {
	result := regionListForAccount(nil)
	if len(result) != len(aws.RegionsList) {
		t.Errorf("expected full regions list (%d), got %d", len(aws.RegionsList), len(result))
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		expect string
	}{
		{"all empty", []string{"", "", ""}, ""},
		{"first wins", []string{"a", "b", "c"}, "a"},
		{"skip empty", []string{"", "b", "c"}, "b"},
		{"last one", []string{"", "", "c"}, "c"},
		{"single", []string{"x"}, "x"},
		{"no args", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstNonEmpty(tt.args...)
			if result != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, result)
			}
		})
	}
}

func TestTruncateARN(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"short", "short-arn", "short-arn"},
		{"exactly 30", "123456789012345678901234567890", "123456789012345678901234567890"},
		{"long", "arn:aws:iam::123456789012:role/very-long-role-name-that-exceeds", "...le/very-long-role-name-that-exceeds"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateARN(tt.input)
			if len(tt.input) > 30 {
				// Should start with "..."
				if result[:3] != "..." {
					t.Errorf("expected truncated ARN to start with '...', got %q", result)
				}
				// Length should be 30
				if len(result) != 30 {
					t.Errorf("expected truncated length 30, got %d", len(result))
				}
			} else {
				if result != tt.input {
					t.Errorf("expected %q, got %q", tt.input, result)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Step constants sanity checks
// ---------------------------------------------------------------------------

func TestStepIDValues(t *testing.T) {
	// Ensure step IDs are sequential from 0
	if stepAccount != 0 {
		t.Error("stepAccount should be 0")
	}
	if stepDone != 12 {
		t.Errorf("stepDone should be 12, got %d", int(stepDone))
	}
	// Verify ordering: Confirm < Fetching < Done
	if stepConfirm >= stepFetching {
		t.Error("stepConfirm should be before stepFetching")
	}
	if stepFetching >= stepDone {
		t.Error("stepFetching should be before stepDone")
	}
}

func TestSourceConstants(t *testing.T) {
	if sourceNone != "" {
		t.Error("sourceNone should be empty string")
	}
	if sourceInteractive != "interactive" {
		t.Errorf("sourceInteractive should be 'interactive', got %q", sourceInteractive)
	}
	if sourcePreset != "preset" {
		t.Errorf("sourcePreset should be 'preset', got %q", sourcePreset)
	}
}
