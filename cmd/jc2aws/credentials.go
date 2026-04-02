package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yousysadmin/jc2aws/internal/aws"
	"github.com/yousysadmin/jc2aws/internal/jumpcloud"
	"github.com/yousysadmin/jc2aws/internal/totp"
)

// getCredentials authenticates via JumpCloud and retrieves temporary AWS credentials.
func getCredentials(email, password, idpURL, mfa, principalARN, roleARN, region string, duration int) (aws.AwsSamlOutput, error) {
	var err error

	// If MFA value length > 6, treat as secret and derive TOTP
	if len(mfa) > 6 {
		mfa, err = totp.GetToken(mfa)
		if err != nil {
			return aws.AwsSamlOutput{}, err
		}
	}

	jc, err := jumpcloud.New(email, password, idpURL, mfa)
	if err != nil {
		return aws.AwsSamlOutput{}, err
	}
	saml, err := jc.GetSaml()
	if err != nil {
		return aws.AwsSamlOutput{}, err
	}

	cred, err := aws.GetCredentials(aws.AwsSamlInput{
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
		if err := os.WriteFile(filePathCreds, creds, 0600); err != nil {
			return err
		}

		filePathConf := filepath.Join(awsDir, "config")
		conf, err := cred.ToAwsConfig(profileName, filePathConf)
		if err != nil {
			return fmt.Errorf("failed to prepare AWS config: %w", err)
		}
		if err := os.WriteFile(filePathConf, conf, 0600); err != nil {
			return err
		}

	case "env":
		filePath := filepath.Join(homeDir, ".jc2aws.env")
		if err := os.WriteFile(filePath, []byte(cred.PrintEnv()), 0600); err != nil {
			return err
		}

	case "cli-stdout":
		c, _ := cred.ToAwsCredentials(profileName, "")
		if _, err := io.Writer(os.Stdout).Write(c); err != nil {
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
	curShell := os.Getenv("SHELL")
	if curShell == "" {
		curShell = "/bin/sh"
	}

	var cmd *exec.Cmd
	if scriptName != "" {
		cmd = exec.Command(curShell, "-i", scriptName)
	} else {
		cmd = exec.Command(curShell, "-i")
	}
	cmd.Env = append(sysEnv, env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
