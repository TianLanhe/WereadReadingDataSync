package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPStatusError struct{ StatusCode int }

func (e *HTTPStatusError) Error() string { return fmt.Sprintf("HTTP 状态码 %d", e.StatusCode) }

func isHTTPStatus(err error, status int) bool {
	var responseErr *HTTPStatusError
	return errors.As(err, &responseErr) && responseErr.StatusCode == status
}

// logExecutionTime 打印函数执行耗时日志
func logExecutionTime(name string) func() {
	now := time.Now()
	return func() {
		elapsed := time.Since(now)
		fmt.Printf("%s执行耗时: %s\n", name, elapsed)
	}
}

// GenerateCurlCommand 构建curl命令
func GenerateCurlCommand(method, path string, body []byte, headers map[string]string) string {
	var curlCmd []string
	curlCmd = append(curlCmd, "curl", "-X", method)

	for key, value := range headers {
		// 为header添加双引号并转义内部引号
		escapedValue := strings.ReplaceAll(value, "\"", "\\\"")
		curlCmd = append(curlCmd, "-H", fmt.Sprintf("\"%s: %s\"", key, escapedValue))
	}

	if body != nil {
		// 为body添加双引号并转义内部引号
		escapedBody := strings.ReplaceAll(string(body), "\"", "\\\"")
		curlCmd = append(curlCmd, "-d", fmt.Sprintf("\"%s\"", escapedBody))
	}

	curlCmd = append(curlCmd, path)
	return strings.Join(curlCmd, " ")
}

// SendHTTPRequest 通用的执行HTTP请求的函数
func SendHTTPRequest(method, path string, body []byte, headers map[string]string, debugs ...bool) ([]byte, error) {
	var debug bool
	if len(debugs) > 0 {
		debug = debugs[0]
	}

	// 打印curl命令
	if debug {
		curlCmd := GenerateCurlCommand(method, path, body, headers)
		fmt.Printf("Generated curl command: %s\n", curlCmd)
	}

	// 构建HTTP请求
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewBuffer(body)
	}
	req, err := http.NewRequest(method, path, reqBody)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// 执行HTTP请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取返回体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 判断返回状态码是否正常
	if resp.StatusCode != 200 {
		return respBody, &HTTPStatusError{StatusCode: resp.StatusCode}
	}

	// 打印返回内容
	if debug {
		fmt.Printf("Response body: %s\n", string(respBody))
	}

	return respBody, nil
}
