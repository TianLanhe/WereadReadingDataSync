package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type HTTPStatusError struct {
	StatusCode int
	Code       int
	RetryAfter time.Duration
}

func (e *HTTPStatusError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("接口错误码 %d（HTTP %d）", e.Code, e.StatusCode)
	}
	return fmt.Sprintf("HTTP 状态码 %d", e.StatusCode)
}
func isHTTPStatus(err error, status int) bool {
	var e *HTTPStatusError
	return errors.As(err, &e) && e.StatusCode == status
}

var syncHTTPClient = newSyncHTTPClient()

func newSyncHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 10
	transport.MaxConnsPerHost = 10
	return &http.Client{Timeout: 45 * time.Second, Transport: transport}
}

// SendHTTPRequest returns structured errors; presentation belongs to the reporter.
// The variadic parameter is retained for callers built against the old helper.
func SendHTTPRequest(method, path string, body []byte, headers map[string]string, _ ...bool) ([]byte, error) {
	return sendHTTPRequest(context.Background(), method, path, body, headers, nil)
}

func sendHTTPRequest(ctx context.Context, method, path string, body []byte, headers map[string]string, beforeSend func() error) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if beforeSend != nil {
		if err := beforeSend(); err != nil {
			return nil, err
		}
	}
	resp, err := syncHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var api struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(data, &api)
	if resp.StatusCode != http.StatusOK || api.Code == 99991400 {
		reset, _ := strconv.ParseFloat(resp.Header.Get("x-ogw-ratelimit-reset"), 64)
		if reset <= 0 {
			reset, _ = strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
		}
		return data, &HTTPStatusError{StatusCode: resp.StatusCode, Code: api.Code, RetryAfter: time.Duration(reset * float64(time.Second))}
	}
	return data, nil
}
