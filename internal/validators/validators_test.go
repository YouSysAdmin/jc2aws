package validators

import (
	"errors"
	"slices"
	"testing"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

func TestNamesContainsAllKeys(t *testing.T) {
	expectedKeys := []string{
		"skip", "email", "password", "idp-url", "mfa", "output-format",
	}
	names := Names()
	for _, key := range expectedKeys {
		if !slices.Contains(names, key) {
			t.Errorf("Names() missing expected key %q", key)
		}
	}

	// Provider-dependent keys are served by ProviderAware, not by the registry.
	for _, key := range ProviderKeys() {
		if slices.Contains(names, key) {
			t.Errorf("Names() should not list provider-dependent key %q", key)
		}
	}
}

func TestGetReturnsSkipForUnknown(t *testing.T) {
	fn := Get("nonexistent-key")
	if fn == nil {
		t.Fatal("expected a safe validator for unknown key, got nil")
	}
	if err := fn("anything"); err != nil {
		t.Errorf("unknown-key validator should accept any input, got: %v", err)
	}
}

func TestGetReturnsNonNilForKnownKey(t *testing.T) {
	for _, key := range []string{"skip", "email", "password"} {
		fn := Get(key)
		if fn == nil {
			t.Errorf("expected non-nil function for key %q", key)
		}
	}
}

func TestSkipValidator(t *testing.T) {
	fn := Get("skip")
	if err := fn(""); err != nil {
		t.Errorf("skip validator should accept empty string, got: %v", err)
	}
	if err := fn("anything"); err != nil {
		t.Errorf("skip validator should accept any string, got: %v", err)
	}
}

func TestEmailValidator(t *testing.T) {
	fn := Get("email")

	valid := []string{
		"user@example.com",
		"first.last@domain.co",
		"test+tag@example.org",
	}
	for _, v := range valid {
		if err := fn(v); err != nil {
			t.Errorf("email validator rejected valid email %q: %v", v, err)
		}
	}

	invalid := []string{
		"",
		"notanemail",
		"@nodomain",
		"missing@",
		"spaces in@email.com",
	}
	for _, v := range invalid {
		if err := fn(v); err == nil {
			t.Errorf("email validator accepted invalid email %q", v)
		}
	}
}

func TestPasswordValidator(t *testing.T) {
	fn := Get("password")

	if err := fn("12345678"); err != nil {
		t.Errorf("password validator rejected 8-char string: %v", err)
	}
	if err := fn("longpasswordhere"); err != nil {
		t.Errorf("password validator rejected long password: %v", err)
	}
	if err := fn("short"); err == nil {
		t.Error("password validator accepted <8 char string")
	}
	if err := fn(""); err == nil {
		t.Error("password validator accepted empty string")
	}
}

func TestIdpURLValidator(t *testing.T) {
	fn := Get("idp-url")

	valid := []string{
		"https://sso.jumpcloud.com/saml2/my-aws-prod",
	}
	for _, v := range valid {
		if err := fn(v); err != nil {
			t.Errorf("idp-url validator rejected valid URL %q: %v", v, err)
		}
	}

	invalid := []string{
		"",
		"not a url",
		"://missing-scheme",
		"http://insecure.example.com/saml", // plain http is not allowed
		"/relative/path",
	}
	for _, v := range invalid {
		if err := fn(v); err == nil {
			t.Errorf("idp-url validator accepted invalid URL %q", v)
		}
	}
}

func TestMFAValidator(t *testing.T) {
	fn := Get("mfa")

	if err := fn("123456"); err != nil {
		t.Errorf("mfa validator rejected 6-digit code: %v", err)
	}
	if err := fn("JBSWY3DPEHPK3PXP"); err != nil {
		t.Errorf("mfa validator rejected TOTP secret: %v", err)
	}
	if err := fn("12345"); err == nil {
		t.Error("mfa validator accepted <6 char string")
	}
	if err := fn(""); err == nil {
		t.Error("mfa validator accepted empty string")
	}
}

func TestOutputFormatValidator(t *testing.T) {
	fn := Get("output-format")

	valid := []string{"cli", "env", "cli-stdout", "env-stdout", "shell"}
	for _, v := range valid {
		if err := fn(v); err != nil {
			t.Errorf("output-format validator rejected valid format %q: %v", v, err)
		}
	}

	invalid := []string{"", "json", "yaml", "CLI"}
	for _, v := range invalid {
		if err := fn(v); err == nil {
			t.Errorf("output-format validator accepted invalid format %q", v)
		}
	}
}

// fakeProvider records which validator was asked for and always fails, so a
// test can prove ProviderAware routed to the provider rather than the registry.
type fakeProvider struct {
	cloud.Provider // embedded: only the validators below are exercised
	called         string
}

func (f *fakeProvider) ValidateRoleARN(string) error {
	f.called = KeyRoleARN
	return errors.New("role arn rejected by provider")
}

func (f *fakeProvider) ValidateProviderARN(string) error {
	f.called = KeyProviderARN
	return errors.New("provider arn rejected by provider")
}

func (f *fakeProvider) ValidateRegion(string) error {
	f.called = KeyRegion
	return errors.New("region rejected by provider")
}

func TestProviderAwareRoutesProviderKeysToProvider(t *testing.T) {
	for _, key := range ProviderKeys() {
		t.Run(key, func(t *testing.T) {
			p := &fakeProvider{}

			err := ProviderAware(key, p)("anything")
			if err == nil {
				t.Fatalf("ProviderAware(%q) did not reach the provider", key)
			}
			if p.called != key {
				t.Errorf("ProviderAware(%q) called the provider's %q validator", key, p.called)
			}
		})
	}
}

func TestProviderAwareFallsThroughToRegistry(t *testing.T) {
	p := &fakeProvider{}

	// A provider-independent key must behave exactly like Get, and must not
	// touch the provider.
	if err := ProviderAware(KeyEmail, p)("not-an-email"); err == nil {
		t.Error("ProviderAware(email) accepted an invalid address")
	}
	if err := ProviderAware(KeyEmail, p)("user@example.com"); err != nil {
		t.Errorf("ProviderAware(email) rejected a valid address: %v", err)
	}
	if p.called != "" {
		t.Errorf("ProviderAware(email) unexpectedly called the provider's %q validator", p.called)
	}
}

func TestProviderAwareNilProviderSkipsProviderKeys(t *testing.T) {
	// Before an account is chosen the TUI has no provider; input must still be
	// accepted rather than rejected outright.
	for _, key := range ProviderKeys() {
		t.Run(key, func(t *testing.T) {
			if err := ProviderAware(key, nil)("whatever"); err != nil {
				t.Errorf("ProviderAware(%q, nil) = %v, want nil", key, err)
			}
		})
	}
}

func TestProviderAwareNilProviderStillValidatesOtherKeys(t *testing.T) {
	if err := ProviderAware(KeyEmail, nil)("not-an-email"); err == nil {
		t.Error("ProviderAware(email, nil) accepted an invalid address")
	}
}

func TestProviderAwareUnknownKeyIsPermissive(t *testing.T) {
	if err := ProviderAware("no-such-key", &fakeProvider{})("anything"); err != nil {
		t.Errorf("ProviderAware(unknown) = %v, want nil", err)
	}
}

func TestGetIsPermissiveForProviderKeys(t *testing.T) {
	// Get must stay total: provider-dependent keys are no longer registered,
	// so they fall back to skip rather than panicking.
	for _, key := range ProviderKeys() {
		if err := Get(key)("anything at all"); err != nil {
			t.Errorf("Get(%q) = %v, want nil (skip)", key, err)
		}
	}
}
