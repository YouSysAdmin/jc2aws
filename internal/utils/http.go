package utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// maxResponseBodySize caps how much of a response body is read into memory;
// the JumpCloud JSON/HTML responses are far below this limit.
const maxResponseBodySize = 10 << 20 // 10 MiB

var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
	},
}

// ReadHTTPResponseBody read HTTP Response content to byte array
func ReadHTTPResponseBody(resp *http.Response) (body []byte, err error) {
	if resp == nil || resp.Body == nil {
		return nil, errors.New("nil HTTP response")
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("cannot read response body: %w", err)
	}

	return body, nil
}

// Request make HTTP request
func Request(ctx context.Context, method string, url string, body []byte, headers http.Header, cookies []*http.Cookie) (resp *http.Response, err error) {
	var reqBody io.Reader = http.NoBody
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %w", err)
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	for name, values := range headers {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}

	resp, err = httpClient.Do(req)
	if err != nil {
		if e, ok := errors.AsType[net.Error](err); ok && e.Timeout() {
			return nil, fmt.Errorf("do request timeout: %w", err)
		}
		return nil, fmt.Errorf("cannot do request: %w", err)
	}

	return resp, nil
}
