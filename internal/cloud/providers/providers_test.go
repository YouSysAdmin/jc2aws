package providers

import (
	"errors"
	"testing"

	"github.com/yousysadmin/jc2aws/internal/cloud"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantErr  bool
	}{
		{name: "aws", input: "aws", wantName: cloud.NameAWS},
		{name: "alibaba", input: "alibaba", wantName: cloud.NameAlibaba},
		{name: "alibaba mixed case", input: " Alibaba ", wantName: cloud.NameAlibaba},
		{name: "empty falls back to the default", input: "", wantName: cloud.DefaultName},
		{name: "mixed case is normalized", input: "  AWS ", wantName: cloud.NameAWS},
		{name: "unknown provider", input: "gcp", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Get(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Name() != tt.wantName {
				t.Errorf("Get(%q).Name() = %q, want %q", tt.input, got.Name(), tt.wantName)
			}
		})
	}
}

func TestGetUnknownWrapsSentinel(t *testing.T) {
	_, err := Get("gcp")
	if !errors.Is(err, cloud.ErrUnknownProvider) {
		t.Errorf("Get(\"gcp\") error = %v, want it to wrap cloud.ErrUnknownProvider", err)
	}
}

// TestGetCoversEveryKnownName guards against cloud.Names() and the factory
// drifting apart: every advertised provider must be constructible.
func TestGetCoversEveryKnownName(t *testing.T) {
	for _, name := range cloud.Names() {
		t.Run(name, func(t *testing.T) {
			p, err := Get(name)
			if err != nil {
				t.Fatalf("Get(%q) error = %v", name, err)
			}
			if p.Name() != name {
				t.Errorf("Get(%q).Name() = %q", name, p.Name())
			}
		})
	}
}
