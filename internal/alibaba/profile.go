package alibaba

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/ini.v1"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// aliyunStsMode is the aliyun CLI authentication mode for STS tokens.
const aliyunStsMode = "StsToken"

// AliyunConfigPath returns the aliyun CLI config path under homeDir.
func AliyunConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".aliyun", "config.json")
}

// CredentialsPath returns the shared Alibaba Cloud credentials path under
// homeDir, as read by the credentials-go profile provider.
func CredentialsPath(homeDir string) string {
	return filepath.Join(homeDir, ".alibabacloud", "credentials")
}

// CLIFiles renders ~/.aliyun/config.json and ~/.alibabacloud/credentials, in
// that order, merging into whatever already exists at those paths. Nothing is
// written to disk.
func (*Provider) CLIFiles(homeDir, profileName string, cred cloud.Credentials) ([]cloud.File, error) {
	cfgPath := AliyunConfigPath(homeDir)
	cfgData, err := ToAliyunConfig(profileName, cfgPath, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare aliyun config: %w", err)
	}

	credPath := CredentialsPath(homeDir)
	credData, err := ToCredentialsFile(profileName, credPath, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare alibaba cloud credentials: %w", err)
	}

	return []cloud.File{
		{Path: cfgPath, Data: cfgData},
		{Path: credPath, Data: credData},
	}, nil
}

// StdoutProfile renders a self-contained credentials section, with no on-disk
// merge, for the "cli-stdout" output format.
func (*Provider) StdoutProfile(profileName string, cred cloud.Credentials) ([]byte, error) {
	return ToCredentialsFile(profileName, "", cred)
}

// ToAliyunConfig renders ~/.aliyun/config.json with profileName set to mode
// StsToken, preserving every other profile, the current selection and any keys
// this tool does not recognise.
//
// The document is manipulated as raw JSON rather than through a typed struct on
// purpose: the aliyun CLI's profile has dozens of fields and gains more each
// release, and a decode/re-encode round trip would silently strip unknown
// fields from other people's profiles.
func ToAliyunConfig(profileName, inputFile string, cred cloud.Credentials) ([]byte, error) {
	if profileName == "" {
		profileName = DefaultProfileName
	}

	doc, err := readAliyunDoc(inputFile)
	if err != nil {
		return nil, err
	}

	var profiles []json.RawMessage
	if raw, ok := doc["profiles"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &profiles); err != nil {
			return nil, fmt.Errorf("failed to parse profiles in %s: %w", inputFile, err)
		}
	}

	idx, entry, err := findProfile(profiles, profileName, inputFile)
	if err != nil {
		return nil, err
	}

	if err := fillStsProfile(entry, profileName, cred); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to encode profile %q: %w", profileName, err)
	}
	if idx >= 0 {
		profiles[idx] = encoded
	} else {
		profiles = append(profiles, encoded)
	}

	if doc["profiles"], err = json.Marshal(profiles); err != nil {
		return nil, fmt.Errorf("failed to encode profiles: %w", err)
	}

	// Preserve whichever profile the user has selected; only seed it when the
	// file does not name one. Writing ~/.aws/credentials does not switch
	// AWS_PROFILE either.
	var current string
	if raw, ok := doc["current"]; ok {
		_ = json.Unmarshal(raw, &current)
	}
	if current == "" {
		if doc["current"], err = json.Marshal(profileName); err != nil {
			return nil, fmt.Errorf("failed to encode current profile: %w", err)
		}
	}

	// The aliyun CLI writes this file with tab indentation; match it so we do
	// not churn the whole document on every login.
	out, err := json.MarshalIndent(doc, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("failed to render %s: %w", inputFile, err)
	}

	return append(out, '\n'), nil
}

// readAliyunDoc loads the top level of an aliyun CLI config document. A missing
// or empty file yields an empty document; a corrupt one is an error rather than
// something to overwrite, because replacing it would destroy every profile in it.
func readAliyunDoc(inputFile string) (map[string]json.RawMessage, error) {
	doc := map[string]json.RawMessage{}
	if inputFile == "" {
		return doc, nil
	}

	raw, err := os.ReadFile(inputFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return doc, nil
	case err != nil:
		return nil, fmt.Errorf("failed to read %s: %w", inputFile, err)
	case len(bytes.TrimSpace(raw)) == 0:
		return doc, nil
	}

	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse %s (move it aside and retry): %w", inputFile, err)
	}

	return doc, nil
}

// findProfile locates profileName among profiles, returning its index and
// decoded fields. Entries that cannot be read are passed over rather than
// rejected, so one damaged profile does not block the others.
func findProfile(profiles []json.RawMessage, profileName, inputFile string) (int, map[string]json.RawMessage, error) {
	for i, p := range profiles {
		var probe struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(p, &probe); err != nil {
			continue
		}
		if probe.Name != profileName {
			continue
		}

		entry := map[string]json.RawMessage{}
		if err := json.Unmarshal(p, &entry); err != nil {
			return 0, nil, fmt.Errorf("failed to parse profile %q in %s: %w", profileName, inputFile, err)
		}
		return i, entry, nil
	}

	return -1, map[string]json.RawMessage{}, nil
}

// fillStsProfile writes the StsToken fields into entry, leaving every key it
// does not own untouched.
func fillStsProfile(entry map[string]json.RawMessage, profileName string, cred cloud.Credentials) error {
	set := func(key string, value any) error {
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to encode %s: %w", key, err)
		}
		entry[key] = b
		return nil
	}

	owned := []struct {
		key   string
		value string
	}{
		{"name", profileName},
		{"mode", aliyunStsMode},
		{"access_key_id", cred.AccessKeyID},
		{"access_key_secret", cred.SecretAccessKey},
		{"sts_token", cred.SessionToken},
		{"region_id", cred.Region},
	}
	for _, f := range owned {
		if err := set(f.key, f.value); err != nil {
			return err
		}
	}

	// Defaults only on creation; never clobber a user's own choices.
	for key, value := range map[string]string{"output_format": "json", "language": "en"} {
		if _, ok := entry[key]; !ok {
			if err := set(key, value); err != nil {
				return err
			}
		}
	}

	// sts_expiration is inert for StsToken mode, but it is the only place a
	// user or a status-line script can see when the profile goes stale.
	if cred.Expiration != nil {
		return set("sts_expiration", cred.Expiration.Unix())
	}
	delete(entry, "sts_expiration")

	return nil
}

// ToCredentialsFile renders ~/.alibabacloud/credentials with profileName as a
// type=sts section, preserving any other sections found in inputIniFile.
func ToCredentialsFile(profileName, inputIniFile string, cred cloud.Credentials) ([]byte, error) {
	if profileName == "" {
		profileName = DefaultProfileName
	}

	profile, err := ini.LooseLoad(inputIniFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", inputIniFile, err)
	}

	// NewSection returns the existing section when one already carries this
	// name, which is what overwrites our profile while leaving others intact.
	section, err := profile.NewSection(profileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile section %q: %w", profileName, err)
	}
	section.Key("type").SetValue("sts")
	section.Key("access_key_id").SetValue(cred.AccessKeyID)
	section.Key("access_key_secret").SetValue(cred.SecretAccessKey)
	section.Key("security_token").SetValue(cred.SessionToken)
	if cred.Expiration != nil {
		section.Key("expiration").SetValue(cred.Expiration.Format(time.RFC3339))
	}

	var buf bytes.Buffer
	if _, err := profile.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to render credentials file: %w", err)
	}

	return buf.Bytes(), nil
}
