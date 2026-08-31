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

	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/jumpcloud"
	"github.com/yousysadmin/jc2aws/internal/totp"
)

const (
	minSessionDuration = 900
	maxSessionDuration = 43200
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

// getCredentials authenticates via JumpCloud and retrieves temporary AWS credentials.
func getCredentials(ctx context.Context, email, password, idpURL, mfa, principalARN, roleARN, region string, duration int) (aws.AwsSamlOutput, error) {
	email = strings.TrimSpace(email)
	mfa = strings.TrimSpace(mfa)
	principalARN = strings.TrimSpace(principalARN)
	roleARN = strings.TrimSpace(roleARN)
	region = strings.TrimSpace(region)

	if duration < minSessionDuration || duration > maxSessionDuration {
		return aws.AwsSamlOutput{}, fmt.Errorf("session duration %d is out of the allowed range %d-%d seconds",
			duration, minSessionDuration, maxSessionDuration)
	}

	// A value of exactly 6 digits is a one-time code; anything else is a TOTP secret.
	if mfa != "" && !isOTPCode(mfa) {
		token, err := totp.GetToken(mfa)
		if err != nil {
			return aws.AwsSamlOutput{}, fmt.Errorf("failed to derive TOTP code from MFA secret: %w", err)
		}
		mfa = token
	}

	jc, err := jumpcloud.New(email, password, idpURL, mfa)
	if err != nil {
		return aws.AwsSamlOutput{}, err
	}
	saml, err := jc.GetSaml(ctx)
	if err != nil {
		return aws.AwsSamlOutput{}, err
	}

	cred, err := aws.GetCredentials(ctx, aws.AwsSamlInput{
		PrincipalArn:    principalARN,
		RoleArn:         roleARN,
		SAMLAssertion:   saml,
		DurationSeconds: int32(duration),
		Region:          region,
	})
	if err != nil {
		return aws.AwsSamlOutput{}, err
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

// outputCredentials writes credentials in the selected format.
func outputCredentials(cred aws.AwsSamlOutput, format, profileName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to determine home directory: %w", err)
	}

	switch format {
	case "cli":
		awsDir := filepath.Join(homeDir, ".aws")
		if err := os.MkdirAll(awsDir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", awsDir, err)
		}

		filePathCreds := filepath.Join(awsDir, "credentials")
		creds, err := cred.ToAwsCredentials(profileName, filePathCreds)
		if err != nil {
			return fmt.Errorf("failed to prepare AWS credentials: %w", err)
		}
		if err := writeSecretFile(filePathCreds, creds); err != nil {
			return fmt.Errorf("failed to write AWS credentials: %w", err)
		}

		filePathConf := filepath.Join(awsDir, "config")
		conf, err := cred.ToAwsConfig(profileName, filePathConf)
		if err != nil {
			return fmt.Errorf("failed to prepare AWS config: %w", err)
		}
		if err := writeSecretFile(filePathConf, conf); err != nil {
			return fmt.Errorf("failed to write AWS config: %w", err)
		}

	case "env":
		filePath := filepath.Join(homeDir, ".jc2aws.env")
		if err := writeSecretFile(filePath, []byte(cred.PrintEnv())); err != nil {
			return fmt.Errorf("failed to write env file: %w", err)
		}

	case "cli-stdout":
		c, err := cred.ToAwsCredentials(profileName, "")
		if err != nil {
			return fmt.Errorf("failed to prepare AWS credentials: %w", err)
		}
		if _, err := os.Stdout.Write(c); err != nil {
			return err
		}

	case "env-stdout":
		if _, err := io.WriteString(os.Stdout, cred.PrintEnv()); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}

	return nil
}

// launchShell starts an interactive shell with AWS credential env vars injected.
func launchShell(cred aws.AwsSamlOutput, scriptName string) error {
	env := cred.ToEnv()
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
