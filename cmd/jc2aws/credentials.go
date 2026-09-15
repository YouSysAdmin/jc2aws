package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yousysadmin/jc2aws/internal/cloud"
	"github.com/yousysadmin/jc2aws/internal/jumpcloud"
	"github.com/yousysadmin/jc2aws/internal/totp"
)

// isOTPCode reports whether the value looks like a one-time code (exactly 6 ASCII digits).
func isOTPCode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// credentialRequest holds everything getCredentials needs. A struct keeps the
// call sites readable and removes the risk of transposing the several
// same-typed string parameters.
type credentialRequest struct {
	Provider     cloud.Provider
	Email        string
	Password     string
	IdpURL       string
	MFA          string
	PrincipalARN string
	RoleARN      string
	Region       string
	Duration     int
}

// getCredentials authenticates via JumpCloud and exchanges the resulting SAML
// assertion for temporary credentials from the selected cloud provider.
func getCredentials(ctx context.Context, req credentialRequest) (cloud.Credentials, error) {
	// getCredentials runs inside a bubbletea command goroutine, where a nil
	// dereference would kill the program with the alt-screen still active.
	if req.Provider == nil {
		return cloud.Credentials{}, errors.New("no cloud provider selected")
	}

	email := strings.TrimSpace(req.Email)
	mfa := strings.TrimSpace(req.MFA)
	principalARN := strings.TrimSpace(req.PrincipalARN)
	roleARN := strings.TrimSpace(req.RoleARN)
	region := strings.TrimSpace(req.Region)

	minDuration, maxDuration := req.Provider.SessionDurationRange()
	if req.Duration < minDuration || req.Duration > maxDuration {
		return cloud.Credentials{}, fmt.Errorf("session duration %d is out of the allowed range %d-%d seconds for %s",
			req.Duration, minDuration, maxDuration, req.Provider.Info().DisplayName)
	}

	// Validate the ARNs before authenticating: the provider rejects a malformed
	// ARN anyway, and finding out after a full JumpCloud round-trip (which may
	// have consumed a one-time MFA code) is needlessly expensive.
	if err := req.Provider.ValidateProviderARN(principalARN); err != nil {
		return cloud.Credentials{}, err
	}
	if err := req.Provider.ValidateRoleARN(roleARN); err != nil {
		return cloud.Credentials{}, err
	}

	// A value of exactly 6 digits is a one-time code; anything else is a TOTP secret.
	if mfa != "" && !isOTPCode(mfa) {
		token, err := totp.GetToken(mfa)
		if err != nil {
			return cloud.Credentials{}, fmt.Errorf("failed to derive TOTP code from MFA secret: %w", err)
		}
		mfa = token
	}

	jc, err := jumpcloud.New(email, req.Password, req.IdpURL, mfa)
	if err != nil {
		return cloud.Credentials{}, err
	}
	saml, err := jc.GetSaml(ctx)
	if err != nil {
		return cloud.Credentials{}, err
	}

	cred, err := req.Provider.AssumeRoleWithSAML(ctx, cloud.SAMLInput{
		ProviderARN:     principalARN,
		RoleARN:         roleARN,
		SAMLAssertion:   saml,
		DurationSeconds: int32(req.Duration),
		Region:          region,
	})
	if err != nil {
		return cloud.Credentials{}, err
	}
	return cred, nil
}

// writeSecretFile atomically writes data to path with 0600 permissions,
// replacing any existing file (and its mode) via rename.
func writeSecretFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to set permissions on %s: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	return nil
}

// outputCredentials writes credentials in the selected format, using the
// provider's own rendering for the vendor CLI formats.
func outputCredentials(p cloud.Provider, cred cloud.Credentials, format, profileName string) error {
	if p == nil {
		return errors.New("no cloud provider selected")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	switch format {
	case "cli":
		// Every file is rendered before any is written, so a rendering failure
		// cannot leave a half-updated set of profiles behind.
		files, err := p.CLIFiles(homeDir, profileName, cred)
		if err != nil {
			return fmt.Errorf("failed to prepare %s CLI files: %w", p.Info().DisplayName, err)
		}
		for _, f := range files {
			dir := filepath.Dir(f.Path)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
			if err := writeSecretFile(f.Path, f.Data); err != nil {
				return fmt.Errorf("failed to write %s: %w", f.Path, err)
			}
		}

	case "env":
		filePath := filepath.Join(homeDir, ".jc2aws.env")
		if err := writeSecretFile(filePath, []byte(cloud.EnvString(p, cred))); err != nil {
			return fmt.Errorf("failed to write env file: %w", err)
		}

	case "cli-stdout":
		c, err := p.StdoutProfile(profileName, cred)
		if err != nil {
			return fmt.Errorf("failed to prepare %s credentials: %w", p.Info().DisplayName, err)
		}
		if _, err := os.Stdout.Write(c); err != nil {
			return err
		}

	case "env-stdout":
		if _, err := io.WriteString(os.Stdout, cloud.EnvString(p, cred)); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}

	return nil
}

// launchShell starts an interactive shell with the provider's credential
// environment variables injected.
func launchShell(p cloud.Provider, cred cloud.Credentials, scriptName string) error {
	if p == nil {
		return errors.New("no cloud provider selected")
	}

	env := p.Env(cred)
	sysEnv := os.Environ()
	curShell := cmp.Or(os.Getenv("SHELL"), "/bin/sh")

	var cmd *exec.Cmd
	if scriptName != "" {
		cmd = exec.Command(curShell, scriptName)
	} else {
		cmd = exec.Command(curShell, "-i")
	}
	cmd.Env = append(sysEnv, env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return fmt.Errorf("shell exited with status %d", exitErr.ExitCode())
		}
		return fmt.Errorf("failed to launch shell %s: %w", curShell, err)
	}
	return nil
}
