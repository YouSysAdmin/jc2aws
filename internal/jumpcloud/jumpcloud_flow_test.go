package jumpcloud

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newFlowServer builds a JumpCloud client wired to a local httptest server.
// The mux must define /xsrf and /auth handlers; idpPath is appended to the
// server URL as the IdP endpoint.
func newFlowClient(t *testing.T, mux *http.ServeMux, idpPath string) (JumpCloud, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	jc, err := NewWithConfig(JumpCloud{
		Email:    "user@example.com",
		Password: "password123",
		IdpURL:   srv.URL + idpPath,
		MFAToken: "123456",
		XsrfURL:  srv.URL + "/xsrf",
		AuthURL:  srv.URL + "/auth",
	})
	if err != nil {
		t.Fatalf("NewWithConfig failed: %v", err)
	}
	return jc, srv
}

func xsrfOK(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "xsrf-session", Value: "s1"})
	fmt.Fprint(w, `{"xsrf":"test-xsrf-token"}`)
}

func TestGetSamlHappyPathPropagatesCookies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xsrf", xsrfOK)
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Xsrftoken") != "test-xsrf-token" {
			t.Errorf("auth did not receive the xsrf token header")
		}
		if c, err := r.Cookie("xsrf-session"); err != nil || c.Value != "s1" {
			t.Errorf("auth did not receive the xsrf cookie")
		}
		http.SetCookie(w, &http.Cookie{Name: "auth-session", Value: "s2"})
		fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("/idp", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("xsrf-session"); err != nil || c.Value != "s1" {
			t.Errorf("idp did not receive the xsrf cookie")
		}
		if c, err := r.Cookie("auth-session"); err != nil || c.Value != "s2" {
			t.Errorf("idp did not receive the auth session cookie")
		}
		fmt.Fprint(w, `<html><body><form><input type="hidden" name="SAMLResponse" value="c2FtbA=="></form></body></html>`)
	})

	jc, _ := newFlowClient(t, mux, "/idp")
	saml, err := jc.GetSaml(t.Context())
	if err != nil {
		t.Fatalf("GetSaml failed: %v", err)
	}
	if saml != "c2FtbA==" {
		t.Errorf("saml = %q, want %q", saml, "c2FtbA==")
	}
}

func TestGetSamlXsrfServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xsrf", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "<html>proxy error</html>")
	})

	jc, _ := newFlowClient(t, mux, "/idp")
	_, err := jc.GetSaml(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "XSRF") || !strings.Contains(err.Error(), "502") {
		t.Errorf("error should name the xsrf step and the status code, got: %v", err)
	}
}

func TestGetSamlAuthFailedWithMessage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xsrf", xsrfOK)
	mux.HandleFunc("/auth", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"Authentication failed."}`)
	})

	jc, _ := newFlowClient(t, mux, "/idp")
	_, err := jc.GetSaml(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Authentication failed.") || !strings.Contains(err.Error(), "401") {
		t.Errorf("error should carry the message and status, got: %v", err)
	}
}

func TestGetSamlAuthNonJSONErrorBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xsrf", xsrfOK)
	mux.HandleFunc("/auth", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "<html>rate limited</html>")
	})

	jc, _ := newFlowClient(t, mux, "/idp")
	_, err := jc.GetSaml(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should carry the HTTP status even for a non-JSON body, got: %v", err)
	}
}

func TestGetSamlMFARequired(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xsrf", xsrfOK)
	mux.HandleFunc("/auth", func(w http.ResponseWriter, _ *http.Request) {
		// JumpCloud can answer 2xx while still requiring MFA.
		fmt.Fprint(w, `{"factors":[{"type":"totp","status":"available"}],"message":"MFA required."}`)
	})

	jc, _ := newFlowClient(t, mux, "/idp")
	_, err := jc.GetSaml(t.Context())
	if !errors.Is(err, ErrMFARequired) {
		t.Errorf("expected ErrMFARequired, got: %v", err)
	}
}

func TestGetSamlIdpBadStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xsrf", xsrfOK)
	mux.HandleFunc("/auth", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{}`) })
	mux.HandleFunc("/idp", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	jc, _ := newFlowClient(t, mux, "/idp")
	_, err := jc.GetSaml(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should carry the IDP status code, got: %v", err)
	}
}

func TestGetSamlMissingSAMLInput(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xsrf", xsrfOK)
	mux.HandleFunc("/auth", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{}`) })
	mux.HandleFunc("/idp", func(w http.ResponseWriter, _ *http.Request) {
		// An element named SAMLResponse that is NOT an input must not match.
		fmt.Fprint(w, `<html><body><form name="SAMLResponse"><input name="other" value="x"></form></body></html>`)
	})

	jc, _ := newFlowClient(t, mux, "/idp")
	_, err := jc.GetSaml(t.Context())
	if err == nil || !strings.Contains(err.Error(), "SAMLResponse") {
		t.Errorf("expected SAMLResponse-not-found error, got: %v", err)
	}
}
