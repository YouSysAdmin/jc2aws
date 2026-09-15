package alibaba

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/ini.v1"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

func testCredentials() cloud.Credentials {
	exp := time.Date(2026, 9, 15, 18, 4, 5, 0, time.UTC)
	return cloud.Credentials{
		Provider:        cloud.NameAlibaba,
		AccessKeyID:     "STS.TEST",
		SecretAccessKey: "SECRET_TEST",
		SessionToken:    "TOKEN_TEST",
		Region:          "eu-central-1",
		Expiration:      &exp,
	}
}

// writeFile writes body into a temp dir and returns the path.
func writeFile(t *testing.T, name, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	return path
}

// decodeAliyun parses rendered config.json into its top level plus profiles
// keyed by name.
func decodeAliyun(t *testing.T, data []byte) (map[string]json.RawMessage, map[string]map[string]any) {
	t.Helper()

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("rendered config is not valid JSON: %v\n%s", err, data)
	}

	var raw []map[string]any
	if p, ok := doc["profiles"]; ok {
		if err := json.Unmarshal(p, &raw); err != nil {
			t.Fatalf("profiles is not an array: %v", err)
		}
	}

	byName := make(map[string]map[string]any, len(raw))
	for _, entry := range raw {
		name, _ := entry["name"].(string)
		byName[name] = entry
	}
	return doc, byName
}

func TestToAliyunConfigCreatesNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	data, err := ToAliyunConfig("prod-ali", path, testCredentials())
	if err != nil {
		t.Fatalf("ToAliyunConfig() error = %v", err)
	}

	doc, profiles := decodeAliyun(t, data)

	var current string
	if err := json.Unmarshal(doc["current"], &current); err != nil {
		t.Fatalf("current is not a string: %v", err)
	}
	if current != "prod-ali" {
		t.Errorf("current = %q, want %q", current, "prod-ali")
	}

	p, ok := profiles["prod-ali"]
	if !ok {
		t.Fatalf("profile prod-ali missing; got %v", profiles)
	}
	want := map[string]any{
		"name":              "prod-ali",
		"mode":              aliyunStsMode,
		"access_key_id":     "STS.TEST",
		"access_key_secret": "SECRET_TEST",
		"sts_token":         "TOKEN_TEST",
		"region_id":         "eu-central-1",
		"output_format":     "json",
		"language":          "en",
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("profile[%q] = %v, want %v", k, p[k], v)
		}
	}
	if p["sts_expiration"] != float64(testCredentials().Expiration.Unix()) {
		t.Errorf("sts_expiration = %v, want %d", p["sts_expiration"], testCredentials().Expiration.Unix())
	}

	// The aliyun CLI writes tab-indented JSON; matching it avoids churning the
	// whole document on every login.
	if !strings.Contains(string(data), "\n\t") {
		t.Error("rendered config is not tab-indented")
	}

	// ToAliyunConfig renders; it must not write.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("ToAliyunConfig() created %s; rendering must be side-effect free", path)
	}
}

func TestToAliyunConfigPreservesOtherProfilesByteForByte(t *testing.T) {
	const existing = `{
	"current": "work",
	"meta_path": "/custom/meta",
	"profiles": [
		{"name": "work", "mode": "AK", "access_key_id": "AK_WORK", "access_key_secret": "SECRET_WORK", "region_id": "cn-hangzhou", "some_future_field": {"nested": true}},
		{"name": "personal", "mode": "RamRoleArn", "ram_role_arn": "acs:ram::1:role/x", "expired_seconds": 900}
	]
}`
	path := writeFile(t, "config.json", existing)

	data, err := ToAliyunConfig("prod-ali", path, testCredentials())
	if err != nil {
		t.Fatalf("ToAliyunConfig() error = %v", err)
	}

	_, before := decodeAliyun(t, []byte(existing))
	doc, after := decodeAliyun(t, data)

	for _, name := range []string{"work", "personal"} {
		if !reflect.DeepEqual(before[name], after[name]) {
			t.Errorf("profile %q was modified:\n before: %v\n  after: %v", name, before[name], after[name])
		}
	}
	if _, ok := after["prod-ali"]; !ok {
		t.Error("prod-ali was not appended")
	}

	// An existing selection must not be hijacked.
	var current string
	if err := json.Unmarshal(doc["current"], &current); err != nil {
		t.Fatalf("current is not a string: %v", err)
	}
	if current != "work" {
		t.Errorf("current = %q, want it left as %q", current, "work")
	}

	// Unknown top-level keys must survive.
	var metaPath string
	if raw, ok := doc["meta_path"]; !ok {
		t.Error("meta_path was dropped")
	} else if err := json.Unmarshal(raw, &metaPath); err != nil || metaPath != "/custom/meta" {
		t.Errorf("meta_path = %q, want %q", metaPath, "/custom/meta")
	}
}

func TestToAliyunConfigUpdatesInPlaceKeepingUnknownKeys(t *testing.T) {
	const existing = `{
	"current": "prod-ali",
	"profiles": [
		{"name": "prod-ali", "mode": "AK", "access_key_id": "OLD", "access_key_secret": "OLD", "region_id": "cn-beijing", "output_format": "table", "cloud_sso_sign_in_url": "https://example.com"}
	]
}`
	path := writeFile(t, "config.json", existing)

	data, err := ToAliyunConfig("prod-ali", path, testCredentials())
	if err != nil {
		t.Fatalf("ToAliyunConfig() error = %v", err)
	}

	_, profiles := decodeAliyun(t, data)
	if len(profiles) != 1 {
		t.Errorf("expected the profile to be replaced in place, got %d profiles", len(profiles))
	}

	p := profiles["prod-ali"]
	if p["mode"] != aliyunStsMode {
		t.Errorf("mode = %v, want %v", p["mode"], aliyunStsMode)
	}
	if p["access_key_id"] != "STS.TEST" || p["region_id"] != "eu-central-1" {
		t.Errorf("credentials were not refreshed: %v", p)
	}
	// A user's own display choice must not be reset.
	if p["output_format"] != "table" {
		t.Errorf("output_format = %v, want the user's %q to survive", p["output_format"], "table")
	}
	// Fields this tool knows nothing about must survive.
	if p["cloud_sso_sign_in_url"] != "https://example.com" {
		t.Errorf("unknown profile key was dropped: %v", p)
	}
}

func TestToAliyunConfigEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		profileName string
		wantErr     bool
		wantCurrent string
	}{
		{name: "empty file", body: "", profileName: "prod-ali", wantCurrent: "prod-ali"},
		{name: "whitespace only", body: "  \n\t ", profileName: "prod-ali", wantCurrent: "prod-ali"},
		{name: "empty document", body: "{}", profileName: "prod-ali", wantCurrent: "prod-ali"},
		{name: "blank current is seeded", body: `{"current":"","profiles":[]}`, profileName: "prod-ali", wantCurrent: "prod-ali"},
		{name: "empty profile name defaults", body: "{}", profileName: "", wantCurrent: DefaultProfileName},
		{name: "corrupt json", body: "{not json", profileName: "prod-ali", wantErr: true},
		{name: "profiles is not an array", body: `{"profiles":"nope"}`, profileName: "prod-ali", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, "config.json", tt.body)

			data, err := ToAliyunConfig(tt.profileName, path, testCredentials())
			if (err != nil) != tt.wantErr {
				t.Fatalf("ToAliyunConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				// A corrupt file must be left untouched, not replaced.
				onDisk, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatalf("failed to re-read fixture: %v", readErr)
				}
				if string(onDisk) != tt.body {
					t.Error("the original file was modified despite the error")
				}
				return
			}

			doc, profiles := decodeAliyun(t, data)
			var current string
			if err := json.Unmarshal(doc["current"], &current); err != nil {
				t.Fatalf("current is not a string: %v", err)
			}
			if current != tt.wantCurrent {
				t.Errorf("current = %q, want %q", current, tt.wantCurrent)
			}
			if _, ok := profiles[tt.wantCurrent]; !ok {
				t.Errorf("profile %q missing; got %v", tt.wantCurrent, profiles)
			}
		})
	}
}

func TestToAliyunConfigWithoutExpiration(t *testing.T) {
	cred := testCredentials()
	cred.Expiration = nil

	data, err := ToAliyunConfig("prod-ali", filepath.Join(t.TempDir(), "config.json"), cred)
	if err != nil {
		t.Fatalf("ToAliyunConfig() error = %v", err)
	}

	_, profiles := decodeAliyun(t, data)
	if _, ok := profiles["prod-ali"]["sts_expiration"]; ok {
		t.Error("sts_expiration should be absent when no expiry was reported")
	}
}

func TestToAliyunConfigClearsStaleExpiration(t *testing.T) {
	path := writeFile(t, "config.json",
		`{"profiles":[{"name":"prod-ali","mode":"StsToken","sts_expiration":1}]}`)

	cred := testCredentials()
	cred.Expiration = nil

	data, err := ToAliyunConfig("prod-ali", path, cred)
	if err != nil {
		t.Fatalf("ToAliyunConfig() error = %v", err)
	}

	_, profiles := decodeAliyun(t, data)
	if _, ok := profiles["prod-ali"]["sts_expiration"]; ok {
		t.Error("a stale sts_expiration must be removed, not left pointing at the past")
	}
}

func TestToCredentialsFile(t *testing.T) {
	data, err := ToCredentialsFile("prod-ali", "", testCredentials())
	if err != nil {
		t.Fatalf("ToCredentialsFile() error = %v", err)
	}

	cfg, err := ini.Load(data)
	if err != nil {
		t.Fatalf("rendered credentials are not valid INI: %v", err)
	}

	section := cfg.Section("prod-ali")
	want := map[string]string{
		"type":              "sts",
		"access_key_id":     "STS.TEST",
		"access_key_secret": "SECRET_TEST",
		"security_token":    "TOKEN_TEST",
		"expiration":        "2026-09-15T18:04:05Z",
	}
	for key, value := range want {
		if got := section.Key(key).String(); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
}

func TestToCredentialsFilePreservesOtherSections(t *testing.T) {
	path := writeFile(t, "credentials", `[other]
type = access_key
access_key_id = AK_OTHER
access_key_secret = SECRET_OTHER

[prod-ali]
type = sts
access_key_id = OLD
`)

	data, err := ToCredentialsFile("prod-ali", path, testCredentials())
	if err != nil {
		t.Fatalf("ToCredentialsFile() error = %v", err)
	}

	cfg, err := ini.Load(data)
	if err != nil {
		t.Fatalf("rendered credentials are not valid INI: %v", err)
	}

	if got := cfg.Section("other").Key("access_key_id").String(); got != "AK_OTHER" {
		t.Errorf("other section was clobbered: access_key_id = %q", got)
	}
	if got := cfg.Section("prod-ali").Key("access_key_id").String(); got != "STS.TEST" {
		t.Errorf("prod-ali was not refreshed: access_key_id = %q", got)
	}

	// Re-rendering must overwrite rather than duplicate.
	if n := strings.Count(string(data), "[prod-ali]"); n != 1 {
		t.Errorf("section [prod-ali] appears %d times, want 1", n)
	}
}

func TestToCredentialsFileDefaultsProfileName(t *testing.T) {
	data, err := ToCredentialsFile("", "", testCredentials())
	if err != nil {
		t.Fatalf("ToCredentialsFile() error = %v", err)
	}

	cfg, err := ini.Load(data)
	if err != nil {
		t.Fatalf("rendered credentials are not valid INI: %v", err)
	}
	if !cfg.HasSection(DefaultProfileName) {
		t.Errorf("expected section %q, got %v", DefaultProfileName, cfg.SectionStrings())
	}
}

func TestCLIFilesPathsAndOrder(t *testing.T) {
	home := t.TempDir()

	files, err := New().CLIFiles(home, "prod-ali", testCredentials())
	if err != nil {
		t.Fatalf("CLIFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("CLIFiles() returned %d files, want 2", len(files))
	}

	wantPaths := []string{
		filepath.Join(home, ".aliyun", "config.json"),
		filepath.Join(home, ".alibabacloud", "credentials"),
	}
	for i, want := range wantPaths {
		if files[i].Path != want {
			t.Errorf("CLIFiles()[%d].Path = %q, want %q", i, files[i].Path, want)
		}
	}

	// Two separate parent directories, unlike the AWS provider's single ~/.aws.
	if filepath.Dir(files[0].Path) == filepath.Dir(files[1].Path) {
		t.Error("expected the two files to live in different directories")
	}

	if _, err := os.Stat(filepath.Join(home, ".aliyun")); !os.IsNotExist(err) {
		t.Error("CLIFiles() created directories; rendering must be side-effect free")
	}
}
