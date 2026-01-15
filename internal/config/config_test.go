package config

import (
	"os"
	"testing"
)

func TestConfig(t *testing.T) {
	configData := `
default_email: default@example.com
default_password: defaultpass123
default_mfa_token_secret: JBSWY3DPEHPK3PXP
accounts:
  - name: "dev-account"
    email: "dev@example.com"
    password: "devpass123"
    idp_url: "https://dev.jumpcloud.com/saml"
    mfa_secret: "JBSWY3DPEHPK3PXP"
    roles:
      - name: "DeveloperRole"
        arn: "arn:aws:iam::123456789012:role/DeveloperRole"
      - name: "AdminRole"
        arn: "arn:aws:iam::123456789012:role/AdminRole"
  - name: "prod-account"
    # Uses default email/password
    idp_url: "https://prod.jumpcloud.com/saml"
    roles:
      - name: "ReadOnlyRole"
        arn: "arn:aws:iam::123456789013:role/ReadOnlyRole"
`

	// Create temporary config file
	tmpFile, err := os.CreateTemp("", "jc2aws_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("Failed to write config data: %v", err)
	}
	tmpFile.Close()

	// Test loading config
	config, err := NewConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test default values
	if config.GetDefaultEmail() != "default@example.com" {
		t.Errorf("Expected default email 'default@example.com', got '%s'", config.GetDefaultEmail())
	}

	if config.GetDefaultPassword() != "defaultpass123" {
		t.Errorf("Expected default password 'defaultpass123', got '%s'", config.GetDefaultPassword())
	}

	if config.GetDefaultMFATokenSecret() != "JBSWY3DPEHPK3PXP" {
		t.Errorf("Expected default MFA secret 'JBSWY3DPEHPK3PXP', got '%s'", config.GetDefaultMFATokenSecret())
	}

	// Test GetAccountsNameList
	accountNames, err := config.GetAccountsNameList()
	if err != nil {
		t.Fatalf("Failed to get account names: %v", err)
	}

	expectedNames := []string{"dev-account", "prod-account"}
	if len(accountNames) != len(expectedNames) {
		t.Errorf("Expected %d account names, got %d", len(expectedNames), len(accountNames))
	}

	for i, name := range expectedNames {
		if i >= len(accountNames) || accountNames[i] != name {
			t.Errorf("Expected account name %s at index %d, got %s", name, i, accountNames[i])
		}
	}

	// Test GetAccounts
	accounts := config.GetAccounts()
	if len(accounts) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(accounts))
	}

	// Check dev-account
	devAccount := accounts[0]
	if devAccount.Name != "dev-account" {
		t.Errorf("Expected account name 'dev-account', got '%s'", devAccount.Name)
	}
	if devAccount.Email != "dev@example.com" {
		t.Errorf("Expected dev account email 'dev@example.com', got '%s'", devAccount.Email)
	}
	if devAccount.Password != "devpass123" {
		t.Errorf("Expected dev account password 'devpass123', got '%s'", devAccount.Password)
	}

	// Check prod-account (should use defaults)
	prodAccount := accounts[1]
	if prodAccount.Name != "prod-account" {
		t.Errorf("Expected account name 'prod-account', got '%s'", prodAccount.Name)
	}
	if prodAccount.Email != "default@example.com" {
		t.Errorf("Expected prod account email to use default, got '%s'", prodAccount.Email)
	}
	if prodAccount.Password != "defaultpass123" {
		t.Errorf("Expected prod account password to use default, got '%s'", prodAccount.Password)
	}

	// Test FindAccountByName
	foundAccount, err := config.FindAccountByName("dev-account")
	if err != nil {
		t.Fatalf("Failed to find account: %v", err)
	}
	if foundAccount.Name != "dev-account" {
		t.Errorf("Expected found account name 'dev-account', got '%s'", foundAccount.Name)
	}

	// Test FindAccountByName not found
	_, err = config.FindAccountByName("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent account, got nil")
	}
}

func TestConfigFileNotFound(t *testing.T) {
	_, err := NewConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for non-existent config file, got nil")
	}
}

func TestConfigEmptyAccounts(t *testing.T) {
	configData := `
default_email: default@example.com
default_password: defaultpass123
accounts: []
`

	tmpFile, err := os.CreateTemp("", "jc2aws_empty_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("Failed to write config data: %v", err)
	}
	tmpFile.Close()

	config, err := NewConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	_, err = config.GetAccountsNameList()
	if err == nil {
		t.Error("Expected error for empty accounts list, got nil")
	}
}

func TestConfigInvalidYAML(t *testing.T) {
	configData := `
default_email: default@example.com
default_password: defaultpass123
accounts:
  - name: "test-account"
    email: "test@example.com"
    invalid_yaml: [unclosed array
`

	tmpFile, err := os.CreateTemp("", "jc2aws_invalid_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configData); err != nil {
		t.Fatalf("Failed to write config data: %v", err)
	}
	tmpFile.Close()

	_, err = NewConfig(tmpFile.Name())
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestConfigGetDefaultPasswordBug(t *testing.T) {
	// This test demonstrates the bug in GetDefaultPassword method
	config := &Config{
		DefaultEmail:    "test@example.com",
		DefaultPassword: "secret123",
	}

	// Bug: GetDefaultPassword returns c.DefaultEmail instead of c.DefaultPassword
	if config.GetDefaultPassword() != "secret123" {
		t.Errorf("Bug detected: GetDefaultPassword() should return 'secret123', got '%s'", config.GetDefaultPassword())
	}
}

func TestDefaultConfigFileName(t *testing.T) {
	if DefaultConfigFileName != ".jc2aws.yaml" {
		t.Errorf("Expected DefaultConfigFileName to be '.jc2aws.yaml', got '%s'", DefaultConfigFileName)
	}
}
