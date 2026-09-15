package cloud

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

// fakeProvider is a minimal Provider used to exercise the package-level
// helpers without importing a vendor implementation.
type fakeProvider struct {
	name    string
	regions []string
	env     []string
}

func (f fakeProvider) Name() string { return f.name }

func (f fakeProvider) Info() Info {
	return Info{
		DisplayName:      "Fake Cloud",
		ProviderARNLabel: "Provider ARN",
		RoleARNLabel:     "Role ARN",
		CLIDescription:   "Write nothing",
	}
}

func (f fakeProvider) Regions() []string { return slices.Clone(f.regions) }

func (f fakeProvider) SessionDurationRange() (int, int) { return 900, 43200 }

func (f fakeProvider) ValidateRoleARN(string) error     { return nil }
func (f fakeProvider) ValidateProviderARN(string) error { return nil }

func (f fakeProvider) ValidateRegion(s string) error { return ValidateRegionInList(f, s) }

func (f fakeProvider) AssumeRoleWithSAML(context.Context, SAMLInput) (Credentials, error) {
	return Credentials{}, errors.New("not implemented")
}

func (f fakeProvider) Env(Credentials) []string { return slices.Clone(f.env) }

func (f fakeProvider) CLIFiles(string, string, Credentials) ([]File, error) { return nil, nil }

func (f fakeProvider) StdoutProfile(string, Credentials) ([]byte, error) { return nil, nil }

var _ Provider = fakeProvider{}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty falls back to default", input: "", want: DefaultName},
		{name: "blank falls back to default", input: "   ", want: DefaultName},
		{name: "already canonical", input: "aws", want: NameAWS},
		{name: "uppercase is lowered", input: "ALIBABA", want: NameAlibaba},
		{name: "surrounding space is trimmed", input: "  Alibaba \t", want: NameAlibaba},
		{name: "unknown value passes through", input: "gcp", want: "gcp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.input); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsKnown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "aws", input: "aws", want: true},
		{name: "alibaba", input: "alibaba", want: true},
		{name: "empty defaults to aws", input: "", want: true},
		{name: "mixed case", input: "AwS", want: true},
		{name: "unknown", input: "gcp", want: false},
		{name: "near miss", input: "aws ", want: true},
		{name: "typo", input: "alibabacloud", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnown(tt.input); got != tt.want {
				t.Errorf("IsKnown(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNames(t *testing.T) {
	want := []string{"aws", "alibaba"}
	if got := Names(); !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}

	// Names must hand out a fresh slice: callers sort and filter it.
	got := Names()
	got[0] = "mutated"
	if Names()[0] != "aws" {
		t.Error("Names() returned a shared backing array; callers can corrupt it")
	}
}

func TestDefaultNameIsKnown(t *testing.T) {
	if !IsKnown(DefaultName) {
		t.Errorf("DefaultName %q is not in Names() %v", DefaultName, Names())
	}
}

func TestEnvString(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{
			name: "multiple variables",
			env:  []string{"A=1", "B=2"},
			want: "A=1\nB=2\n",
		},
		{
			name: "single variable",
			env:  []string{"A=1"},
			want: "A=1\n",
		},
		{
			name: "empty env still ends with a newline",
			env:  nil,
			want: "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := fakeProvider{env: tt.env}
			if got := EnvString(p, Credentials{}); got != tt.want {
				t.Errorf("EnvString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateRegionInList(t *testing.T) {
	p := fakeProvider{regions: []string{"eu-central-1", "us-east-1"}}

	tests := []struct {
		name    string
		region  string
		wantErr bool
	}{
		{name: "known region", region: "eu-central-1", wantErr: false},
		{name: "other known region", region: "us-east-1", wantErr: false},
		{name: "unknown region", region: "us-east-99", wantErr: true},
		{name: "empty region", region: "", wantErr: true},
		{name: "case sensitive", region: "EU-CENTRAL-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegionInList(p, tt.region)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRegionInList(%q) error = %v, wantErr %v", tt.region, err, tt.wantErr)
			}
		})
	}
}

func TestProviderRegionsIsACopy(t *testing.T) {
	p := fakeProvider{regions: []string{"eu-central-1", "us-east-1"}}

	got := p.Regions()
	got[0] = "mutated"

	if p.Regions()[0] != "eu-central-1" {
		t.Error("Regions() returned a shared backing array; a caller can corrupt the provider's list")
	}
}

func TestCredentialsZeroValue(t *testing.T) {
	// Expiration is a pointer precisely so "no expiry reported" is
	// distinguishable from the zero time.
	var c Credentials
	if c.Expiration != nil {
		t.Error("zero Credentials should report a nil Expiration")
	}

	now := time.Now()
	c.Expiration = &now
	if c.Expiration.IsZero() {
		t.Error("Expiration should round-trip a real time")
	}
}
