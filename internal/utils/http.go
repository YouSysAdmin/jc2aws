package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ReadHTTPResponseBody read HTTP Response content to byte array
func ReadHTTPResponseBody(resp *http.Response) (body []byte, err error) {
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	return body, nil
}

// Request make HTTP request
func Request(ctx context.Context, method string, url string, body []byte, headers http.Header, cookies []*http.Cookie, connectMaxWaitTime int) (resp *http.Response, err error) {
	client := http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: time.Duration(connectMaxWaitTime) * time.Second,
			}).DialContext,
		},
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %s", err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	for name, values := range headers {
		req.Header.Add(name, values[0])
	}

	resp, err = client.Do(req)

	if e, ok := err.(net.Error); ok && e.Timeout() {
		return nil, fmt.Errorf("do request timeout: %s", err)
	} else if err != nil {
		return nil, fmt.Errorf("cannot do request: %s", err)
	}

	return resp, nil
}
