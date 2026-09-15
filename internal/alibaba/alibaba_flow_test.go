package alibaba

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

const (
	testRoleARN     = "acs:ram::1250000000000000:role/admin"
	testProviderARN = "acs:ram::1250000000000000:saml-provider/jumpcloud"
	testRegion      = "eu-central-1"
)

// newTestProvider returns a Provider pointed at an httptest server running h.
func newTestProvider(t *testing.T, h http.HandlerFunc) *Provider {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return New(WithEndpoint(srv.URL))
}

// testInput returns a valid SAMLInput, so tests fail on the server response
// rather than on local validation.
func testInput() cloud.SAMLInput {
	return cloud.SAMLInput{
		ProviderARN:     testProviderARN,
		RoleARN:         testRoleARN,
		SAMLAssertion:   "YmFzZTY0LXNhbWw=",
		Region:          testRegion,
		DurationSeconds: 3600,
	}
}

const successBody = `{
  "RequestId": "6894B13B-6D71-4EF5-88FA-F32781734A7E",
  "Credentials": {
    "AccessKeyId": "STS.NUgYrLnoC37mZZCNnAbez",
    "AccessKeySecret": "CVwjCkNzTMupZ8NbTCxCBRq3K16jtcWFTJAyBEv2",
    "SecurityToken": "CAESrAIIARKAAShl40jpa8T1mjSCpS",
    "Expiration": "2015-04-09T11:52:19Z"
  },
  "AssumedRoleUser": {
    "AssumedRoleId": "33157794895460****",
    "Arn": "acs:ram::1250000000000000:role/admin/alice"
  }
}`

func TestAssumeRoleWithSAMLSuccess(t *testing.T) {
	var got struct {
		action, version, roleARN, providerARN, assertion, duration string
		signature, accessKeyID                                     string
	}

	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}
		// Parameters may arrive in the query string or the form body.
		param := func(name string) string {
			if v := r.URL.Query().Get(name); v != "" {
				return v
			}
			return r.PostFormValue(name)
		}
		got.action = param("Action")
		got.version = param("Version")
		got.roleARN = param("RoleArn")
		got.providerARN = param("SAMLProviderArn")
		got.assertion = param("SAMLAssertion")
		got.duration = param("DurationSeconds")
		got.signature = param("Signature")
		got.accessKeyID = param("AccessKeyId")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(successBody))
	})

	cred, err := p.AssumeRoleWithSAML(context.Background(), testInput())
	if err != nil {
		t.Fatalf("AssumeRoleWithSAML() error = %v", err)
	}

	if got.action != "AssumeRoleWithSAML" {
		t.Errorf("Action = %q, want %q", got.action, "AssumeRoleWithSAML")
	}
	if got.version != "2015-04-01" {
		t.Errorf("Version = %q, want %q", got.version, "2015-04-01")
	}
	if got.roleARN != testRoleARN {
		t.Errorf("RoleArn = %q, want %q", got.roleARN, testRoleARN)
	}
	if got.providerARN != testProviderARN {
		t.Errorf("SAMLProviderArn = %q, want %q", got.providerARN, testProviderARN)
	}
	if got.assertion != "YmFzZTY0LXNhbWw=" {
		t.Errorf("SAMLAssertion = %q", got.assertion)
	}
	if got.duration != "3600" {
		t.Errorf("DurationSeconds = %q, want %q", got.duration, "3600")
	}

	// The whole design rests on this API being anonymous: if the SDK ever
	// starts signing the request we would need real credentials to call it.
	if got.signature != "" {
		t.Errorf("request carried Signature=%q; AssumeRoleWithSAML must be anonymous", got.signature)
	}
	if got.accessKeyID != "" {
		t.Errorf("request carried AccessKeyId=%q; AssumeRoleWithSAML must be anonymous", got.accessKeyID)
	}

	if cred.Provider != cloud.NameAlibaba {
		t.Errorf("Provider = %q, want %q", cred.Provider, cloud.NameAlibaba)
	}
	if cred.AccessKeyID != "STS.NUgYrLnoC37mZZCNnAbez" {
		t.Errorf("AccessKeyID = %q", cred.AccessKeyID)
	}
	if cred.SecretAccessKey != "CVwjCkNzTMupZ8NbTCxCBRq3K16jtcWFTJAyBEv2" {
		t.Errorf("SecretAccessKey = %q", cred.SecretAccessKey)
	}
	if cred.SessionToken != "CAESrAIIARKAAShl40jpa8T1mjSCpS" {
		t.Errorf("SessionToken = %q", cred.SessionToken)
	}
	if cred.Region != testRegion {
		t.Errorf("Region = %q, want %q", cred.Region, testRegion)
	}
	if cred.Expiration == nil {
		t.Fatal("Expiration is nil, want the parsed response value")
	}
	if got, want := cred.Expiration.UTC().Format(expirationLayout), "2015-04-09T11:52:19Z"; got != want {
		t.Errorf("Expiration = %q, want %q", got, want)
	}
}

func TestAssumeRoleWithSAMLServerErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantInText []string
	}{
		{
			name:   "client error",
			status: http.StatusBadRequest,
			body: `{"RequestId":"REQ-1","HostId":"sts.aliyuncs.com",` +
				`"Code":"InvalidParameter.SAMLAssertion","Message":"SAMLAssertion is invalid."}`,
			wantInText: []string{"InvalidParameter.SAMLAssertion", "REQ-1"},
		},
		{
			name:       "server error",
			status:     http.StatusInternalServerError,
			body:       `{"RequestId":"REQ-2","Code":"InternalError","Message":"boom"}`,
			wantInText: []string{"InternalError"},
		},
		{
			name:       "empty credentials in a 200",
			status:     http.StatusOK,
			body:       `{"RequestId":"REQ-3"}`,
			wantInText: []string{"no credentials"},
		},
		{
			name:       "malformed expiration",
			status:     http.StatusOK,
			body:       `{"Credentials":{"AccessKeyId":"STS.X","Expiration":"nonsense"}}`,
			wantInText: []string{"sts expiration"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			_, err := p.AssumeRoleWithSAML(context.Background(), testInput())
			if err == nil {
				t.Fatal("AssumeRoleWithSAML() succeeded, want an error")
			}
			for _, want := range tt.wantInText {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			// The SDK default is a single attempt; a retry storm here would
			// turn a transient STS failure into several MFA-bearing requests.
			if n := hits.Load(); n != 1 {
				t.Errorf("server was hit %d times, want exactly 1", n)
			}
		})
	}
}

func TestAssumeRoleWithSAMLHonoursContextCancellation(t *testing.T) {
	// Proves the context-aware SDK overload is used: the context-free one would
	// block here until the handler returns.
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := p.AssumeRoleWithSAML(ctx, testInput()); err == nil {
		t.Fatal("AssumeRoleWithSAML() ignored a cancelled context")
	}
}

func TestAssumeRoleWithSAMLValidatesBeforeDialing(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cloud.SAMLInput)
		want   string
	}{
		{name: "empty region", mutate: func(in *cloud.SAMLInput) { in.Region = "" }, want: "region"},
		{name: "bad region", mutate: func(in *cloud.SAMLInput) { in.Region = "Not A Region" }, want: "region"},
		{name: "aws role arn", mutate: func(in *cloud.SAMLInput) { in.RoleARN = "arn:aws:iam::1:role/x" }, want: "role arn"},
		{name: "aws provider arn", mutate: func(in *cloud.SAMLInput) { in.ProviderARN = "arn:aws:iam::1:saml-provider/x" }, want: "saml provider arn"},
		{name: "duration too low", mutate: func(in *cloud.SAMLInput) { in.DurationSeconds = 60 }, want: "session duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("provider dialed the server despite invalid input")
			})

			in := testInput()
			tt.mutate(&in)

			_, err := p.AssumeRoleWithSAML(context.Background(), in)
			if err == nil {
				t.Fatal("AssumeRoleWithSAML() accepted invalid input")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}
