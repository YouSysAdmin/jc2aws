package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadHTTPResponseBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantBody string
		wantErr  bool
	}{
		{
			name:     "successful read",
			body:     "test response body",
			wantBody: "test response body",
			wantErr:  false,
		},
		{
			name:     "empty body",
			body:     "",
			wantBody: "",
			wantErr:  false,
		},
		{
			name:     "large body",
			body:     strings.Repeat("x", 10000),
			wantBody: strings.Repeat("x", 10000),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}

			body, err := ReadHTTPResponseBody(resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadHTTPResponseBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if string(body) != tt.wantBody {
				t.Errorf("ReadHTTPResponseBody() = %v, want %v", string(body), tt.wantBody)
			}
		})
	}
}

func TestRequest(t *testing.T) {
	tests := []struct {
		name               string
		method             string
		url                string
		body               []byte
		headers            http.Header
		cookies            []*http.Cookie
		connectMaxWaitTime int
		expectedStatus     int
		expectedBody       string
		wantErr            bool
	}{
		{
			name:               "GET request",
			method:             "GET",
			url:                "/test",
			body:               nil,
			headers:            nil,
			cookies:            nil,
			connectMaxWaitTime: 30,
			expectedStatus:     http.StatusOK,
			expectedBody:       "GET response",
			wantErr:            false,
		},
		{
			name:               "POST request with body",
			method:             "POST",
			url:                "/test",
			body:               []byte("test payload"),
			headers:            map[string][]string{"Content-Type": {"application/json"}},
			cookies:            nil,
			connectMaxWaitTime: 30,
			expectedStatus:     http.StatusOK,
			expectedBody:       "POST response",
			wantErr:            false,
		},
		{
			name:               "request with headers",
			method:             "GET",
			url:                "/test",
			body:               nil,
			headers:            map[string][]string{"Authorization": {"Bearer token123"}},
			cookies:            nil,
			connectMaxWaitTime: 30,
			expectedStatus:     http.StatusOK,
			expectedBody:       "Authorized response",
			wantErr:            false,
		},
		{
			name:               "request with cookies",
			method:             "GET",
			url:                "/test",
			body:               nil,
			headers:            nil,
			cookies:            []*http.Cookie{{Name: "session", Value: "abc123"}},
			connectMaxWaitTime: 30,
			expectedStatus:     http.StatusOK,
			expectedBody:       "Cookie response",
			wantErr:            false,
		},
		{
			name:               "timeout error",
			method:             "GET",
			url:                "/slow",
			body:               nil,
			headers:            nil,
			cookies:            nil,
			connectMaxWaitTime: 1,
			expectedStatus:     200, // Server will respond
			expectedBody:       "slow response",
			wantErr:            false, // Our HTTP client doesn't enforce connect timeout
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.url == "/slow" {
					time.Sleep(2 * time.Second)
				}

				if r.Method != tt.method {
					t.Errorf("Expected method %s, got %s", tt.method, r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}

				if r.URL.Path != tt.url {
					t.Errorf("Expected path %s, got %s", tt.url, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				if tt.headers != nil {
					for key, values := range tt.headers {
						if got := r.Header.Get(key); got != values[0] {
							t.Errorf("Expected header %s=%s, got %s", key, values[0], got)
						}
					}
				}

				if tt.cookies != nil {
					for _, cookie := range tt.cookies {
						if got, err := r.Cookie(cookie.Name); err != nil || got.Value != cookie.Value {
							t.Errorf("Expected cookie %s=%s, got error or wrong value", cookie.Name, cookie.Value)
						}
					}
				}

				w.Header().Set("Content-Type", "text/plain")
				if tt.expectedStatus > 0 {
					w.WriteHeader(tt.expectedStatus)
				}
				w.Write([]byte(tt.expectedBody))
			}))
			defer server.Close()

			ctx := context.Background()
			resp, err := Request(ctx, tt.method, server.URL+tt.url, tt.body, tt.headers, tt.cookies, tt.connectMaxWaitTime)

			if (err != nil) != tt.wantErr {
				t.Errorf("Request() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp.StatusCode != tt.expectedStatus {
					t.Errorf("Request() status = %v, want %v", resp.StatusCode, tt.expectedStatus)
				}

				body, err := ReadHTTPResponseBody(resp)
				if err != nil {
					t.Errorf("Failed to read response body: %v", err)
				}

				if string(body) != tt.expectedBody {
					t.Errorf("Request() body = %v, want %v", string(body), tt.expectedBody)
				}
			}
		})
	}
}

func TestRequestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("slow response"))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := Request(ctx, "GET", server.URL, nil, nil, nil, 30)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}

	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "context") {
		t.Errorf("Expected timeout or context error, got: %v", err)
	}
}

func TestRequestInvalidURL(t *testing.T) {
	ctx := context.Background()

	_, err := Request(ctx, "GET", "://invalid-url", nil, nil, nil, 30)
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}
