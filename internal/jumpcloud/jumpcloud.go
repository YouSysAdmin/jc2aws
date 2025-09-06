package jumpcloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/yousysadmin/jc2aws/internal/utils"
	"golang.org/x/net/context"
)

const (
	xsrfURL              = "https://console.jumpcloud.com/userconsole/xsrf"
	authURL              = "https://console.jumpcloud.com/userconsole/auth"
	MaxRequestTimeout    = 10
	MaxConnectionTimeout = 5
)

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

	// Maximal connection timeout for all reqest
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

	if config.MaxConnectionTimeout == 0 {
		config.MaxConnectionTimeout = MaxConnectionTimeout
	}

	if config.MaxRequestTimeout == 0 {
		config.MaxRequestTimeout = MaxRequestTimeout
	}

	return config, nil
}

// GetSaml get SAML data
func (jc *JumpCloud) GetSaml() (samlResponse string, err error) {

	if err = jc.getXSRFToken(); err != nil {
		return "", err
	}

	if err = jc.auth(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(jc.MaxRequestTimeout)*time.Second)
	defer cancel()

	resp, err := utils.Request(ctx, http.MethodGet, jc.IdpURL, nil, nil, jc.cookies, jc.MaxConnectionTimeout)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	samlResponse, err = utils.GetHTMLInputValue(resp, "SAMLResponse")
	if err != nil {
		return "", fmt.Errorf("fail to get saml response: %s", err)
	}

	return samlResponse, nil
}

// auth authenticate in the Jumpcloud
func (jc *JumpCloud) auth() error {
	authRequestData, _ := json.Marshal(authRequest{
		Email:    jc.Email,
		Password: jc.Password,
		Otp:      jc.MFAToken},
	)

	headers := http.Header{}
	headers.Add("Accept", "application/json")
	headers.Add("Content-Type", "application/json")
	headers.Add("X-Xsrftoken", jc.xsrf)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(jc.MaxRequestTimeout)*time.Second)
	defer cancel()

	resp, err := utils.Request(ctx, http.MethodPost, authURL, authRequestData, headers, jc.cookies, jc.MaxRequestTimeout)
	if err != nil {
		return err
	}

	// Unmarshal response message
	var responseData authResponse

	respBody, err := utils.ReadHTTPResponseBody(resp)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(respBody, &responseData); err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.New(responseData.Message)
	}

	return nil
}

// getXSRFToken get XSRF token from Jumpcloud
func (jc *JumpCloud) getXSRFToken() error {

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(jc.MaxRequestTimeout)*time.Second)
	defer cancel()

	resp, err := utils.Request(ctx, http.MethodGet, xsrfURL, nil, nil, nil, jc.MaxConnectionTimeout)
	if err != nil {
		return err
	}

	var xsrf xsfrResponse
	respBody, err := utils.ReadHTTPResponseBody(resp)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(respBody, &xsrf); err != nil {
		return err
	}

	if xsrf.Token == "" {
		return errors.New("fail to get xsrf token")
	}

	jc.xsrf = xsrf.Token
	jc.cookies = resp.Cookies()

	return nil
}
