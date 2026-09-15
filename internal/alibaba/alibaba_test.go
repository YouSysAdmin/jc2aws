package alibaba

import (
	"reflect"
	"testing"
	"time"

	stsclient "github.com/alibabacloud-go/sts-20150401/v2/client"
	"github.com/alibabacloud-go/tea/dara"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

func TestProviderName(t *testing.T) {
	if got := New().Name(); got != cloud.NameAlibaba {
		t.Errorf("Name() = %q, want %q", got, cloud.NameAlibaba)
	}
}

func TestProviderInfo(t *testing.T) {
	info := New().Info()
	if info.DisplayName == "" || info.ProviderARNLabel == "" ||
		info.RoleARNLabel == "" || info.CLIDescription == "" {
		t.Errorf("Info() has empty fields: %+v", info)
	}
}

func TestProviderRegionsIsACopy(t *testing.T) {
	got := New().Regions()
	if len(got) != len(RegionsList) {
		t.Fatalf("Regions() returned %d regions, want %d", len(got), len(RegionsList))
	}

	got[0] = "mutated"
	if RegionsList[0] == "mutated" {
		t.Error("Regions() handed out the package-level slice; a caller can corrupt it")
	}
}

func TestRegionsListIsNotTheAWSList(t *testing.T) {
	// Alibaba region IDs overlap with AWS ones but name different places, so a
	// few Alibaba-only IDs must be present to prove the lists are independent.
	want := []string{"cn-hangzhou", "cn-hongkong", "na-south-1"}
	for _, region := range want {
		found := false
		for _, r := range RegionsList {
			if r == region {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RegionsList is missing Alibaba-specific region %q", region)
		}
	}
}

func TestProviderValidateRegion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "listed region", input: "eu-central-1"},
		{name: "china region", input: "cn-hangzhou"},
		{name: "unlisted but well-formed region", input: "ap-southeast-99"},
		{name: "empty", input: "", wantErr: true},
		{name: "single segment", input: "hangzhou", wantErr: true},
		{name: "uppercase", input: "CN-HANGZHOU", wantErr: true},
		{name: "trailing dash", input: "cn-hangzhou-", wantErr: true},
		{name: "spaces", input: "cn hangzhou", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New().ValidateRegion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRegion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestProviderValidateDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   int32
		wantErr bool
	}{
		{name: "minimum", input: 900},
		{name: "typical", input: 3600},
		{name: "maximum", input: 43200},
		{name: "below minimum", input: 899, wantErr: true},
		{name: "above maximum", input: 43201, wantErr: true},
		{name: "zero", input: 0, wantErr: true},
		{name: "negative", input: -1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New().ValidateDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDuration(%d) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseExpiration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			name:  "documented utc form",
			input: "2015-04-09T11:52:19Z",
			want:  time.Date(2015, 4, 9, 11, 52, 19, 0, time.UTC),
		},
		// The service documents UTC only; accepting offsets would hide a
		// contract change instead of surfacing it.
		{name: "offset form", input: "2015-04-09T11:52:19+08:00", wantErr: true},
		{name: "no timezone", input: "2015-04-09T11:52:19", wantErr: true},
		{name: "date only", input: "2015-04-09", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "garbage", input: "not a time", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExpiration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseExpiration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseExpiration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestProviderEnv(t *testing.T) {
	cred := cloud.Credentials{
		AccessKeyID:     "STS.TEST",
		SecretAccessKey: "SECRET_TEST",
		SessionToken:    "TOKEN_TEST",
		Region:          "eu-central-1",
	}

	want := []string{
		"ALIBABA_CLOUD_ACCESS_KEY_ID=STS.TEST",
		"ALIBABA_CLOUD_ACCESS_KEY_SECRET=SECRET_TEST",
		"ALIBABA_CLOUD_SECURITY_TOKEN=TOKEN_TEST",
		"ALIBABA_CLOUD_REGION_ID=eu-central-1",
		"ALIBABA_CLOUD_REGION=eu-central-1",
	}

	if got := New().Env(cred); !reflect.DeepEqual(got, want) {
		t.Errorf("Env() = %v, want %v", got, want)
	}
}

func TestEnvStringEndsWithNewline(t *testing.T) {
	got := cloud.EnvString(New(), cloud.Credentials{})
	if got == "" || got[len(got)-1] != '\n' {
		t.Errorf("EnvString() = %q, want a trailing newline", got)
	}
}

func TestToCredentials(t *testing.T) {
	tests := []struct {
		name    string
		body    *stsclient.AssumeRoleWithSAMLResponseBody
		want    cloud.Credentials
		wantErr bool
	}{
		{name: "nil body", body: nil, wantErr: true},
		{
			name:    "body without credentials",
			body:    &stsclient.AssumeRoleWithSAMLResponseBody{},
			wantErr: true,
		},
		{
			name: "full mapping",
			body: &stsclient.AssumeRoleWithSAMLResponseBody{
				Credentials: &stsclient.AssumeRoleWithSAMLResponseBodyCredentials{
					AccessKeyId:     dara.String("STS.TEST"),
					AccessKeySecret: dara.String("SECRET_TEST"),
					SecurityToken:   dara.String("TOKEN_TEST"),
					Expiration:      dara.String("2015-04-09T11:52:19Z"),
				},
			},
			want: cloud.Credentials{
				Provider:        cloud.NameAlibaba,
				AccessKeyID:     "STS.TEST",
				SecretAccessKey: "SECRET_TEST",
				SessionToken:    "TOKEN_TEST",
				Region:          "eu-central-1",
			},
		},
		{
			name: "missing expiration is not an error",
			body: &stsclient.AssumeRoleWithSAMLResponseBody{
				Credentials: &stsclient.AssumeRoleWithSAMLResponseBodyCredentials{
					AccessKeyId: dara.String("STS.TEST"),
				},
			},
			want: cloud.Credentials{
				Provider:    cloud.NameAlibaba,
				AccessKeyID: "STS.TEST",
				Region:      "eu-central-1",
			},
		},
		{
			name: "malformed expiration",
			body: &stsclient.AssumeRoleWithSAMLResponseBody{
				Credentials: &stsclient.AssumeRoleWithSAMLResponseBodyCredentials{
					AccessKeyId: dara.String("STS.TEST"),
					Expiration:  dara.String("nonsense"),
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toCredentials(tt.body, "eu-central-1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("toCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			gotExp := got.Expiration
			got.Expiration = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toCredentials() = %+v, want %+v", got, tt.want)
			}

			wantExpiry := tt.body.Credentials.Expiration != nil
			if wantExpiry != (gotExp != nil) {
				t.Errorf("Expiration presence = %v, want %v", gotExp != nil, wantExpiry)
			}
		})
	}
}

func TestWithEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantEndpoint string
		wantProtocol string
	}{
		{name: "httptest url", input: "http://127.0.0.1:54321", wantEndpoint: "127.0.0.1:54321", wantProtocol: "HTTP"},
		{name: "https url", input: "https://sts.aliyuncs.com", wantEndpoint: "sts.aliyuncs.com"},
		{name: "bare host", input: "sts.eu-central-1.aliyuncs.com", wantEndpoint: "sts.eu-central-1.aliyuncs.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(WithEndpoint(tt.input))
			if p.endpoint != tt.wantEndpoint {
				t.Errorf("endpoint = %q, want %q", p.endpoint, tt.wantEndpoint)
			}
			if p.protocol != tt.wantProtocol {
				t.Errorf("protocol = %q, want %q", p.protocol, tt.wantProtocol)
			}
		})
	}
}

func TestNewLeavesEndpointUnsetByDefault(t *testing.T) {
	// Production must let the SDK derive sts.<region>.aliyuncs.com rather than
	// pinning the legacy global alias.
	p := New()
	if p.endpoint != "" || p.protocol != "" {
		t.Errorf("New() pinned endpoint %q protocol %q; both must be empty", p.endpoint, p.protocol)
	}
}
