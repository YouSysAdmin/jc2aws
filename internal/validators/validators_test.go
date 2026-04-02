package validators

import (
	"testing"
)

func TestMapContainsAllKeys(t *testing.T) {
	expectedKeys := []string{
		"skip", "email", "password", "idp-url",
		"role-arn", "principal-arn", "region", "mfa", "output-format",
	}
	for _, key := range expectedKeys {
		if _, ok := Map[key]; !ok {
			t.Errorf("Map missing expected key %q", key)
		}
	}
}

func TestGetReturnsNilForUnknown(t *testing.T) {
	fn := Get("nonexistent-key")
	if fn != nil {
		t.Error("expected nil for unknown key, got a function")
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
		"http://localhost:8080/path",
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
	}
	for _, v := range invalid {
		if err := fn(v); err == nil {
			t.Errorf("idp-url validator accepted invalid URL %q", v)
		}
	}
}

func TestRoleArnValidator(t *testing.T) {
	fn := Get("role-arn")

	if err := fn("arn:aws:iam::123456789012:role/admin"); err != nil {
		t.Errorf("role-arn validator rejected valid ARN: %v", err)
	}
	if err := fn("not-an-arn"); err == nil {
		t.Error("role-arn validator accepted invalid ARN")
	}
	if err := fn(""); err == nil {
		t.Error("role-arn validator accepted empty string")
	}
}

func TestPrincipalArnValidator(t *testing.T) {
	fn := Get("principal-arn")

	if err := fn("arn:aws:iam::123456789012:saml-provider/jumpcloud"); err != nil {
		t.Errorf("principal-arn validator rejected valid ARN: %v", err)
	}
	if err := fn("garbage"); err == nil {
		t.Error("principal-arn validator accepted invalid ARN")
	}
}

func TestRegionValidator(t *testing.T) {
	fn := Get("region")

	validRegions := []string{"us-east-1", "eu-west-1", "ap-northeast-1"}
	for _, r := range validRegions {
		if err := fn(r); err != nil {
			t.Errorf("region validator rejected valid region %q: %v", r, err)
		}
	}

	invalidRegions := []string{"", "us-east-99", "invalid-region", "US-EAST-1"}
	for _, r := range invalidRegions {
		if err := fn(r); err == nil {
			t.Errorf("region validator accepted invalid region %q", r)
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
