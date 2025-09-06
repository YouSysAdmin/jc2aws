package config

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v2"
)

const DefaultConfigFileName = ".jc2aws.yaml"

type Config struct {
	DefaultEmail          string    `yaml:"default_email"`
	DefaultPassword       string    `yaml:"default_password"`
	DefaultMFATokenSecret string    `yaml:"default_mfa_token_secret"`
	Accounts              []Account `yaml:"accounts"`
}

func NewConfig(path string) (conf *Config, err error) {

	conf = &Config{}

	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return conf, fmt.Errorf("config file %s not found", path)
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return conf, err
	}

	err = yaml.Unmarshal(file, &conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}

// GetAccounts return list of accounts
func (c *Config) GetAccounts() (accounts []Account) {
	// sets default email, password and mfa if it is not set for an account separately
	for _, a := range c.Accounts {
		if a.Email == "" {
			a.Email = c.DefaultEmail
		}
		if a.Password == "" {
			a.Password = c.DefaultPassword
		}
		if a.MFASecret == "" {
			a.MFASecret = c.DefaultMFATokenSecret
		}
		accounts = append(accounts, a)
	}
	return accounts
}

func (c *Config) GetAccountsNameList() ([]string, error) {
	var accountsList []string
	for _, a := range c.Accounts {
		accountsList = append(accountsList, a.Name)
	}

	if len(accountsList) <= 0 {
		return nil, errors.New("accounts list is empty")
	}

	return accountsList, nil
}

// GetDefaultEmail return list of accounts
func (c *Config) GetDefaultEmail() string { return c.DefaultEmail }

// GetDefaultPassword return list of accounts
func (c *Config) GetDefaultPassword() string { return c.DefaultEmail }

func (c *Config) GetDefaultMFATokenSecret() string { return c.DefaultMFATokenSecret }

// FindAccountByName return account by account name from accounts list
func (c *Config) FindAccountByName(name string) (account Account, err error) {
	idx := slices.IndexFunc(c.GetAccounts(), func(a Account) bool { return a.Name == name })
	if idx < 0 {
		return account, fmt.Errorf("the account %s not found", name)
	}

	account = c.Accounts[idx]

	if account.Email == "" {
		account.Email = c.DefaultEmail
	}
	if account.Password == "" {
		account.Password = c.DefaultPassword
	}
	if account.MFASecret == "" {
		account.MFASecret = c.DefaultMFATokenSecret
	}

	return account, nil
}
