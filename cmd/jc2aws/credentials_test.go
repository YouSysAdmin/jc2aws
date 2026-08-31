package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/jc2aws/internal/aws"
)

func testCred() aws.AwsSamlOutput {
	exp := time.Now().Add(time.Hour).UTC()
	return aws.AwsSamlOutput{
		AccessKeyID:     "TEST_ACCESS_KEY_ID",
		SecretAccessKey: "TEST_SECRET_ACCESS_KEY",
		SessionToken:    "TEST_SESSION_TOKEN",
		Region:          "eu-west-1",
		Expiration:      &exp,
	}
}

// captureStdout runs fn while capturing everything written to os.Stdout.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fnErr := fn()
	w.Close()

	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return string(out), fnErr
}

// ---------------------------------------------------------------------------
// isOTPCode
// ---------------------------------------------------------------------------

func TestIsOTPCode(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"123456", true},
		{"000000", true},
		{"12345", false},
		{"1234567", false},
		{"12345a", false},
		{"123456 ", false},
		{"", false},
		{"JBSWY3", false},
	}
	for _, tt := range tests {
		if got := isOTPCode(tt.input); got != tt.want {
			t.Errorf("isOTPCode(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// getCredentials input validation (fails before any network access)
// ---------------------------------------------------------------------------

func TestGetCredentialsDurationOutOfRange(t *testing.T) {
	for _, duration := range []int{0, 899, 43201, -1} {
		_, err := getCredentials(context.Background(),
			"user@example.com", "password", "https://sso.example.com", "",
			"arn:aws:iam::1:saml-provider/x", "arn:aws:iam::1:role/x", "us-east-1", duration)
		if err == nil || !strings.Contains(err.Error(), "out of the allowed range") {
			t.Errorf("duration %d: expected out-of-range error, got %v", duration, err)
		}
	}
}

func TestGetCredentialsInvalidMFASecret(t *testing.T) {
	// 7+ chars that are not a valid base32 secret must fail fast.
	_, err := getCredentials(context.Background(),
		"user@example.com", "password", "https://sso.example.com", "not!a@secret",
		"arn:aws:iam::1:saml-provider/x", "arn:aws:iam::1:role/x", "us-east-1", 3600)
	if err == nil || !strings.Contains(err.Error(), "MFA secret") {
		t.Errorf("expected MFA secret error, got %v", err)
	}
}

func TestGetCredentialsTrimsPastedOTP(t *testing.T) {
	// A pasted 6-digit code with a trailing newline must be treated as a code
	// (not as a base32 secret). The call then proceeds to JumpCloud and fails
	// on the network layer — but NOT with a base32 error.
	_, err := getCredentials(context.Background(),
		"user@example.com", "password", "https://127.0.0.1:1/idp", "123456\n",
		"arn:aws:iam::1:saml-provider/x", "arn:aws:iam::1:role/x", "us-east-1", 3600)
	if err != nil && strings.Contains(err.Error(), "base32") {
		t.Errorf("pasted OTP was misinterpreted as a secret: %v", err)
	}
}

// ---------------------------------------------------------------------------
// outputCredentials
// ---------------------------------------------------------------------------

func TestOutputCredentialsCli(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := outputCredentials(testCred(), "cli", "myprofile"); err != nil {
		t.Fatalf("outputCredentials(cli) failed: %v", err)
	}

	credPath := filepath.Join(home, ".aws", "credentials")
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("credentials file not written: %v", err)
	}
	for _, want := range []string{"[myprofile]", "TEST_ACCESS_KEY_ID", "TEST_SECRET_ACCESS_KEY", "TEST_SESSION_TOKEN"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("credentials file missing %q", want)
		}
	}

	fi, err := os.Stat(credPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("credentials file mode = %o, want 0600", perm)
	}

	confData, err := os.ReadFile(filepath.Join(home, ".aws", "config"))
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	if !strings.Contains(string(confData), "[profile myprofile]") {
		t.Errorf("config file missing profile section, got:\n%s", confData)
	}
}

func TestOutputCredentialsCliPreservesOtherProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0700); err != nil {
		t.Fatal(err)
	}
	existing := "[other]\naws_access_key_id = OTHER_KEY\n"
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := outputCredentials(testCred(), "cli", "myprofile"); err != nil {
		t.Fatalf("outputCredentials(cli) failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(awsDir, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[other]") || !strings.Contains(string(data), "OTHER_KEY") {
		t.Errorf("existing profile was lost:\n%s", data)
	}

	// A pre-existing world-readable file must be tightened to 0600.
	fi, err := os.Stat(filepath.Join(awsDir, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("rewritten credentials file mode = %o, want 0600", perm)
	}
}

func TestOutputCredentialsEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := outputCredentials(testCred(), "env", ""); err != nil {
		t.Fatalf("outputCredentials(env) failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".jc2aws.env"))
	if err != nil {
		t.Fatalf("env file not written: %v", err)
	}
	if !strings.Contains(string(data), "AWS_ACCESS_KEY_ID=TEST_ACCESS_KEY_ID") {
		t.Errorf("env file missing access key:\n%s", data)
	}
}

func TestOutputCredentialsCliStdout(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return outputCredentials(testCred(), "cli-stdout", "myprofile")
	})
	if err != nil {
		t.Fatalf("outputCredentials(cli-stdout) failed: %v", err)
	}
	if !strings.Contains(out, "[myprofile]") || !strings.Contains(out, "TEST_ACCESS_KEY_ID") {
		t.Errorf("cli-stdout output incomplete:\n%s", out)
	}
}

func TestOutputCredentialsEnvStdout(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return outputCredentials(testCred(), "env-stdout", "")
	})
	if err != nil {
		t.Fatalf("outputCredentials(env-stdout) failed: %v", err)
	}
	if !strings.Contains(out, "AWS_SECRET_ACCESS_KEY=TEST_SECRET_ACCESS_KEY") {
		t.Errorf("env-stdout output incomplete:\n%s", out)
	}
}

func TestOutputCredentialsUnsupportedFormat(t *testing.T) {
	err := outputCredentials(testCred(), "bogus", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Errorf("expected unsupported-format error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeSecretFile
// ---------------------------------------------------------------------------

func TestWriteSecretFileAtomicAndTight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")

	// Existing world-readable file gets replaced with 0600.
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeSecretFile(path, []byte("new-content")); err != nil {
		t.Fatalf("writeSecretFile failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-content" {
		t.Errorf("content = %q, want %q", data, "new-content")
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("mode = %o, want 0600", perm)
	}

	// No leftover temp files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the target file in dir, found %d entries", len(entries))
	}
}
