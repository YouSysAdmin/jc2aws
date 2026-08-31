package jumpcloud

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yousysadmin/jc2aws/internal/utils"
)

const (
	// DefaultXsrfURL is the JumpCloud endpoint that issues XSRF tokens.
	DefaultXsrfURL = "https://console.jumpcloud.com/userconsole/xsrf"
	// DefaultAuthURL is the JumpCloud authentication endpoint.
	DefaultAuthURL = "https://console.jumpcloud.com/userconsole/auth"
	// MaxRequestTimeout is the default per-request timeout in seconds.
	MaxRequestTimeout = 10
	// MaxConnectionTimeout is the default overall timeout in seconds for the
	// whole xsrf -> auth -> SAML flow.
	MaxConnectionTimeout = 30
)

// ErrMFARequired is returned when JumpCloud requires an MFA code that was
// not provided or was rejected.
var ErrMFARequired = errors.New("jumpcloud requires an MFA code (missing or invalid OTP)")

// xsfrResponse Jumpcloud XSRF respose structure
type xsfrResponse struct {
	Token string `json:"xsrf"`
}

// authRequest Jumpcloud Auth request structure
type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Otp      string `json:"otp"`
}

// authResponse Jumpcloud Auth response
// MFA require response '{"factors":[{"type":"totp","status":"available"}],"message":"MFA required."}'
// Auth failed response: '{"message":"Authentication failed."}'
type authResponse struct {
	Message string `json:"message"`
	Factors []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	} `json:"factors"`
}

type JumpCloud struct {
	// Jumpcloud user email
	Email string
	// Jumpcloud user password
	Password string
	// Jumpcloud SSO application IDP URL
	IdpURL string
	// Jumpcloud user MFA token (optional)
	MFAToken string

	// XsrfURL overrides the XSRF token endpoint (defaults to DefaultXsrfURL)
	XsrfURL string
	// AuthURL overrides the authentication endpoint (defaults to DefaultAuthURL)
	AuthURL string

	// Maximal overall timeout in seconds for the whole SAML flow
	MaxConnectionTimeout int
	// Maximal request timeout for all request
	MaxRequestTimeout int

	// Coockies store
	cookies []*http.Cookie
	// XSRF token
	xsrf string
}

// New Init new jc client
func New(email, password, idpURL, mfaToken string) (JumpCloud, error) {
	config := JumpCloud{
		Email:    email,
		Password: password,
		IdpURL:   idpURL,
		MFAToken: mfaToken,

		MaxRequestTimeout:    MaxRequestTimeout,
		MaxConnectionTimeout: MaxConnectionTimeout,
	}

	return NewWithConfig(config)
}

// NewWithConfig Init new jc client with config
func NewWithConfig(config JumpCloud) (JumpCloud, error) {
	// Validate config and set default values
	if config.Email == "" || config.Password == "" || config.IdpURL == "" {
		return config, errors.New("email, password, idpurl can't be blank")
	}

	config.MaxRequestTimeout = cmp.Or(config.MaxRequestTimeout, MaxRequestTimeout)
	config.MaxConnectionTimeout = cmp.Or(config.MaxConnectionTimeout, MaxConnectionTimeout)
	config.XsrfURL = cmp.Or(config.XsrfURL, DefaultXsrfURL)
	config.AuthURL = cmp.Or(config.AuthURL, DefaultAuthURL)

	return config, nil
}

// requestCtx derives a per-request context from the flow context.
func (jc *JumpCloud) requestCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(jc.MaxRequestTimeout)*time.Second)
}

// GetSaml get SAML data
func (jc *JumpCloud) GetSaml(ctx context.Context) (samlResponse string, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(jc.MaxConnectionTimeout)*time.Second)
	defer cancel()

	if err = jc.getXSRFToken(ctx); err != nil {
		return "", fmt.Errorf("failed to get XSRF token: %w", err)
	}

	if err = jc.auth(ctx); err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	reqCtx, reqCancel := jc.requestCtx(ctx)
	defer reqCancel()

	resp, err := utils.Request(reqCtx, http.MethodGet, jc.IdpURL, nil, nil, jc.cookies)
	if err != nil {
		return "", fmt.Errorf("failed to request IDP URL: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return "", fmt.Errorf("IDP URL %s returned status %d", jc.IdpURL, resp.StatusCode)
	}

	samlResponse, err = utils.GetHTMLInputValue(resp, "SAMLResponse")
	if err != nil {
		return "", fmt.Errorf("fail to get saml response: %w", err)
	}

	return samlResponse, nil
}

// auth authenticate in the Jumpcloud
func (jc *JumpCloud) auth(ctx context.Context) error {
	authRequestData, err := json.Marshal(authRequest{
		Email:    jc.Email,
		Password: jc.Password,
		Otp:      jc.MFAToken,
	})
	if err != nil {
		return fmt.Errorf("cannot encode auth request: %w", err)
	}

	headers := http.Header{}
	headers.Add("Accept", "application/json")
	headers.Add("Content-Type", "application/json")
	headers.Add("X-Xsrftoken", jc.xsrf)

	reqCtx, cancel := jc.requestCtx(ctx)
	defer cancel()

	resp, err := utils.Request(reqCtx, http.MethodPost, jc.AuthURL, authRequestData, headers, jc.cookies)
	if err != nil {
		return err
	}

	respBody, err := utils.ReadHTTPResponseBody(resp)
	if err != nil {
		return err
	}

	var responseData authResponse
	if resp.StatusCode != http.StatusOK {
		// The body may not be JSON at all (proxy or WAF error page); best-effort
		// extract the message and always report the status code.
		_ = json.Unmarshal(respBody, &responseData)
		if isMFARequired(responseData) {
			return fmt.Errorf("%w (HTTP %d)", ErrMFARequired, resp.StatusCode)
		}
		if responseData.Message != "" {
			return fmt.Errorf("jumpcloud auth returned status %d: %s", resp.StatusCode, responseData.Message)
		}
		return fmt.Errorf("jumpcloud auth returned status %d", resp.StatusCode)
	}

	if err := json.Unmarshal(respBody, &responseData); err != nil {
		return fmt.Errorf("cannot decode auth response: %w", err)
	}

	// JumpCloud can answer 2xx while still requiring a second factor.
	if isMFARequired(responseData) {
		return ErrMFARequired
	}

	// Keep any session cookies issued or rotated by the auth step.
	jc.cookies = append(jc.cookies, resp.Cookies()...)

	return nil
}

// isMFARequired reports whether the auth response indicates a pending MFA challenge.
func isMFARequired(r authResponse) bool {
	return len(r.Factors) > 0 || strings.Contains(strings.ToLower(r.Message), "mfa required")
}

// getXSRFToken get XSRF token from Jumpcloud
func (jc *JumpCloud) getXSRFToken(ctx context.Context) error {
	reqCtx, cancel := jc.requestCtx(ctx)
	defer cancel()

	resp, err := utils.Request(reqCtx, http.MethodGet, jc.XsrfURL, nil, nil, nil)
	if err != nil {
		return err
	}

	respBody, err := utils.ReadHTTPResponseBody(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("xsrf endpoint returned status %d", resp.StatusCode)
	}

	var xsrf xsfrResponse
	if err := json.Unmarshal(respBody, &xsrf); err != nil {
		return fmt.Errorf("cannot decode xsrf response: %w", err)
	}

	if xsrf.Token == "" {
		return errors.New("fail to get xsrf token")
	}

	jc.xsrf = xsrf.Token
	jc.cookies = resp.Cookies()

	return nil
}
