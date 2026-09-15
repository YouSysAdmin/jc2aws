package aws

import (
	"path/filepath"
	"testing"

	"gopkg.in/ini.v1"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

func TestProviderName(t *testing.T) {
	if got := New().Name(); got != cloud.NameAWS {
		t.Errorf("Name() = %q, want %q", got, cloud.NameAWS)
	}
}

func TestProviderInfo(t *testing.T) {
	info := New().Info()
	if info.DisplayName == "" || info.ProviderARNLabel == "" ||
		info.RoleARNLabel == "" || info.CLIDescription == "" {
		t.Errorf("Info() has empty fields: %+v", info)
	}
}

func TestProviderSessionDurationRange(t *testing.T) {
	gotMin, gotMax := New().SessionDurationRange()
	if gotMin != 900 || gotMax != 43200 {
		t.Errorf("SessionDurationRange() = (%d, %d), want (900, 43200)", gotMin, gotMax)
	}
}

func TestProviderRegionsIsACopy(t *testing.T) {
	p := New()

	got := p.Regions()
	if len(got) != len(RegionsList) {
		t.Fatalf("Regions() returned %d regions, want %d", len(got), len(RegionsList))
	}

	got[0] = "mutated"
	if RegionsList[0] == "mutated" {
		t.Error("Regions() handed out the package-level slice; a caller can corrupt it")
	}
}

func TestProviderValidateRoleARN(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid role arn", input: "arn:aws:iam::000000000000:role/jumpcloud-admin", wantErr: false},
		{name: "valid saml provider arn is also a valid arn", input: "arn:aws:iam::000000000000:saml-provider/jc", wantErr: false},
		{name: "not an arn", input: "jumpcloud-admin", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "alibaba arn is rejected", input: "acs:ram::1250000000000000:role/admin", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New().ValidateRoleARN(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRoleARN(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestProviderValidateProviderARN(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "arn:aws:iam::000000000000:saml-provider/jumpcloud", wantErr: false},
		{name: "not an arn", input: "jumpcloud", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New().ValidateProviderARN(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProviderARN(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestProviderValidateRegion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "known region", input: "us-east-1", wantErr: false},
		{name: "another known region", input: "eu-central-1", wantErr: false},
		{name: "unknown region", input: "us-east-99", wantErr: true},
		{name: "garbage", input: "invalid-region", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New().ValidateRegion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRegion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestProviderEnv(t *testing.T) {
	cred := cloud.Credentials{
		AccessKeyID:     "AKIA_TEST",
		SecretAccessKey: "SECRET_TEST",
		SessionToken:    "TOKEN_TEST",
		Region:          "eu-central-1",
	}

	want := []string{
		"AWS_ACCESS_KEY_ID=AKIA_TEST",
		"AWS_SECRET_ACCESS_KEY=SECRET_TEST",
		"AWS_SESSION_TOKEN=TOKEN_TEST",
		"AWS_REGION=eu-central-1",
		"AWS_DEFAULT_REGION=eu-central-1",
	}

	got := New().Env(cred)
	if len(got) != len(want) {
		t.Fatalf("Env() returned %d entries, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Env()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCLIFilesPathsAndOrder(t *testing.T) {
	home := t.TempDir()
	cred := cloud.Credentials{
		AccessKeyID:     "AKIA_TEST",
		SecretAccessKey: "SECRET_TEST",
		SessionToken:    "TOKEN_TEST",
		Region:          "eu-central-1",
	}

	files, err := New().CLIFiles(home, "my-prod", cred)
	if err != nil {
		t.Fatalf("CLIFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("CLIFiles() returned %d files, want 2", len(files))
	}

	wantPaths := []string{
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".aws", "config"),
	}
	for i, want := range wantPaths {
		if files[i].Path != want {
			t.Errorf("CLIFiles()[%d].Path = %q, want %q", i, files[i].Path, want)
		}
	}

	// CLIFiles must not touch the filesystem: the caller writes.
	for _, f := range files {
		if _, err := ini.Load(f.Data); err != nil {
			t.Errorf("CLIFiles() produced unparsable INI for %s: %v", f.Path, err)
		}
	}

	creds, err := ini.Load(files[0].Data)
	if err != nil {
		t.Fatalf("failed to parse rendered credentials: %v", err)
	}
	if got := creds.Section("my-prod").Key("aws_access_key_id").String(); got != "AKIA_TEST" {
		t.Errorf("credentials aws_access_key_id = %q, want %q", got, "AKIA_TEST")
	}

	conf, err := ini.Load(files[1].Data)
	if err != nil {
		t.Fatalf("failed to parse rendered config: %v", err)
	}
	if got := conf.Section("profile my-prod").Key("region").String(); got != "eu-central-1" {
		t.Errorf("config region = %q, want %q", got, "eu-central-1")
	}
}

func TestCLIFilesDoesNotWriteToDisk(t *testing.T) {
	home := t.TempDir()

	if _, err := New().CLIFiles(home, "my-prod", cloud.Credentials{Region: "us-east-1"}); err != nil {
		t.Fatalf("CLIFiles() error = %v", err)
	}

	if entries, err := filepath.Glob(filepath.Join(home, ".aws", "*")); err != nil {
		t.Fatalf("glob failed: %v", err)
	} else if len(entries) != 0 {
		t.Errorf("CLIFiles() wrote %v to disk; rendering must be side-effect free", entries)
	}
}

func TestStdoutProfileHasNoOnDiskMerge(t *testing.T) {
	cred := cloud.Credentials{AccessKeyID: "AKIA_TEST", Region: "us-east-1"}

	data, err := New().StdoutProfile("my-prod", cred)
	if err != nil {
		t.Fatalf("StdoutProfile() error = %v", err)
	}

	loaded, err := ini.Load(data)
	if err != nil {
		t.Fatalf("failed to parse StdoutProfile output: %v", err)
	}

	// Only the requested profile (plus go-ini's implicit DEFAULT) may appear.
	for _, s := range loaded.Sections() {
		if s.Name() != ini.DefaultSection && s.Name() != "my-prod" {
			t.Errorf("StdoutProfile() leaked section %q", s.Name())
		}
	}
}

func TestStdoutProfileDefaultsProfileName(t *testing.T) {
	data, err := New().StdoutProfile("", cloud.Credentials{AccessKeyID: "AKIA_TEST"})
	if err != nil {
		t.Fatalf("StdoutProfile() error = %v", err)
	}

	loaded, err := ini.Load(data)
	if err != nil {
		t.Fatalf("failed to parse StdoutProfile output: %v", err)
	}
	if !loaded.HasSection(DefaultAwsProfileName) {
		t.Errorf("StdoutProfile(\"\") did not fall back to %q; sections: %v",
			DefaultAwsProfileName, loaded.SectionStrings())
	}
}
