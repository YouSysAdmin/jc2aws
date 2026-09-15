package aws

import (
	"bytes"
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/ini.v1"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

// renderCredentialsINI renders an ~/.aws/credentials file with profileName set
// to the given credentials. When inputIniFile exists its other profiles are
// loaded and preserved; the named profile is replaced in place.
func renderCredentialsINI(profileName, inputIniFile string, cred cloud.Credentials) ([]byte, error) {
	if profileName == "" {
		profileName = DefaultAwsProfileName
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
	section.Key("aws_access_key_id").SetValue(cred.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(cred.SecretAccessKey)
	section.Key("aws_session_token").SetValue(cred.SessionToken)
	if cred.Expiration != nil {
		section.Key("expiration").SetValue(cred.Expiration.Format(time.RFC3339))
	}

	var buf bytes.Buffer
	if _, err := profile.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to render credentials file: %w", err)
	}

	return buf.Bytes(), nil
}

// renderConfigINI renders an ~/.aws/config file with profileName set to the
// credential region. When inputIniFile exists its other profiles are loaded and
// preserved; the named profile is replaced in place.
func renderConfigINI(profileName, inputIniFile string, cred cloud.Credentials) ([]byte, error) {
	if profileName == "" {
		profileName = DefaultAwsProfileName
	}
	if profileName != DefaultAwsProfileName {
		profileName = "profile " + profileName
	}

	profile, err := ini.LooseLoad(inputIniFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", inputIniFile, err)
	}

	section, err := profile.NewSection(profileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile section %q: %w", profileName, err)
	}
	section.Key("region").SetValue(cred.Region)

	var buf bytes.Buffer
	if _, err := profile.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("failed to render config file: %w", err)
	}

	return buf.Bytes(), nil
}

// CLIFiles renders ~/.aws/credentials and ~/.aws/config, in that order, merging
// into whatever already exists at those paths. Nothing is written to disk.
func (*Provider) CLIFiles(homeDir, profileName string, cred cloud.Credentials) ([]cloud.File, error) {
	credPath := filepath.Join(homeDir, ".aws", "credentials")
	confPath := filepath.Join(homeDir, ".aws", "config")

	credData, err := renderCredentialsINI(profileName, credPath, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare AWS credentials: %w", err)
	}
	confData, err := renderConfigINI(profileName, confPath, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare AWS config: %w", err)
	}

	return []cloud.File{
		{Path: credPath, Data: credData},
		{Path: confPath, Data: confData},
	}, nil
}

// StdoutProfile renders a self-contained AWS credentials profile, with no
// on-disk merge, for the "cli-stdout" output format.
func (*Provider) StdoutProfile(profileName string, cred cloud.Credentials) ([]byte, error) {
	return renderCredentialsINI(profileName, "", cred)
}
