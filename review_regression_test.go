package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestActualUploadPacingSurvivesSlowTokenRefresh(t *testing.T) {
	benchmarkFixture(t)
	cachedToken, tokenExpiration = "", time.Time{}
	previous := syncHTTPClient
	client := *previous
	var mu sync.Mutex
	var starts []time.Time
	client.Transport = testRoundTrip(func(r *http.Request) (*http.Response, error) {
		body := `{"code":0,"data":{"file_token":"cover-token"}}`
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			time.Sleep(90 * time.Millisecond)
			body = `{"code":0,"tenant_access_token":"new-token"}`
		} else if r.URL.Path == "/open-apis/drive/v1/medias/upload_all" {
			mu.Lock()
			starts = append(starts, time.Now())
			mu.Unlock()
		} else {
			return nil, errors.New("unexpected endpoint")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	syncHTTPClient = &client
	t.Cleanup(func() { syncHTTPClient = previous })
	m := newCoverUploadManager(coverUploadOptions{Workers: 3, Interval: 15 * time.Millisecond, Wait: time.Second, Download: func(context.Context, string) ([]byte, error) { return []byte("image"), nil }})
	for _, cover := range []string{"a", "b", "c"} {
		m.StartUpload(cover)
	}
	for _, cover := range []string{"a", "b", "c"} {
		if _, err := m.GetResult(cover); err != nil {
			m.Close()
			t.Fatal(err)
		}
	}
	m.Close()
	if len(starts) != 3 {
		t.Fatalf("uploads=%d", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if starts[i].Sub(starts[i-1]) < 13*time.Millisecond {
			t.Fatalf("actual HTTP uploads bunched after token refresh: %v", starts)
		}
	}
}

func TestReadBookSecondPageBusinessErrorPreventsAllWrites(t *testing.T) {
	benchmarkFixture(t)
	previous := syncHTTPClient
	client := *previous
	transport := http.DefaultTransport
	var pages, writes atomic.Int64
	client.Transport = testRoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/records/batch_") {
			writes.Add(1)
		}
		if r.URL.Path == "/mine/readbook" {
			body := `{"errcode":12345,"errmsg":"backend unavailable"}`
			if pages.Add(1) == 1 {
				body = `{"readBooks":[{"bookId":"only-finished","title":"历史书","markStatus":4,"finishTime":1000}],"hasMore":1}`
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		return transport.RoundTrip(r)
	})
	syncHTTPClient = &client
	t.Cleanup(func() { syncHTTPClient = previous })
	cfg, err := loadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&syncRunner{config: cfg.Feishu}).books()
	if err == nil || writes.Load() != 0 {
		t.Fatalf("failed page accepted: err=%v writes=%d", err, writes.Load())
	}
}

func TestPermanentDNSFailureDoesNotRetry(t *testing.T) {
	var calls atomic.Int64
	m := newCoverUploadManager(coverUploadOptions{Workers: 1, Interval: time.Millisecond, Wait: time.Second, Download: func(context.Context, string) ([]byte, error) {
		calls.Add(1)
		return nil, &net.DNSError{Name: "invalid.example", Err: "no such host", IsNotFound: true}
	}, Backoff: func(int) time.Duration { return 0 }})
	defer m.Close()
	_, err := m.GetResult("cover")
	if err == nil || calls.Load() != 1 {
		t.Fatalf("permanent DNS retried %d times: %v", calls.Load(), err)
	}
}

func TestDynamicProgressFitsVeryNarrowTerminal(t *testing.T) {
	var out bytes.Buffer
	r := NewConsoleReporter(&out, true, 8)
	r.Report(Event{Kind: EventProgress, Stage: "book", Task: "details", Done: 1, Total: 20})
	r.Close()
	for _, segment := range strings.Split(out.String(), "\r\x1b[2K") {
		width := 0
		for _, c := range segment {
			if c >= 0x1100 {
				width += 2
			} else {
				width++
			}
		}
		if width > 7 {
			t.Fatalf("progress wrapped in 8-column terminal: %q", segment)
		}
	}
}

func TestConcurrentExpiredRequestsRecoverLoginOnce(t *testing.T) {
	benchmarkFixture(t)
	previous := syncHTTPClient
	client := *previous
	var oldRequests, logins atomic.Int64
	barrier := make(chan struct{})
	client.Transport = testRoundTrip(func(r *http.Request) (*http.Response, error) {
		status, body := 200, `{"ok":true}`
		if r.Header.Get("accessToken") == "fixture-access" {
			if oldRequests.Add(1) == 5 {
				close(barrier)
			}
			<-barrier
			status, body = 401, `{"errcode":-2012,"errmsg":"登录超时"}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	syncHTTPClient = &client
	t.Cleanup(func() { syncHTTPClient = previous })
	login := func() error {
		logins.Add(1)
		weReadAuthMu.Lock()
		cachedAccessToken, tokenExpiryTime = "new-access", time.Now().Add(time.Hour)
		weReadAuthMu.Unlock()
		return nil
	}
	var wg sync.WaitGroup
	failures := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, err := sendAuthenticatedWeReadGETWithLogin("https://i.weread.qq.com/test", login)
			if err != nil {
				failures <- err
			} else if string(body) != `{"ok":true}` {
				failures <- errors.New("retry returned wrong body")
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if logins.Load() != 1 {
		t.Fatalf("concurrent recovery performed %d logins", logins.Load())
	}
}

func TestHTTPPoolReusesConcurrentConnections(t *testing.T) {
	var connections, requests atomic.Int64
	gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		round := (n - 1) / 10
		if n%10 == 0 {
			close(gates[round])
		}
		<-gates[round]
		w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	for round := 0; round < 2; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, err := syncHTTPClient.Get(server.URL)
				if err != nil {
					t.Error(err)
					return
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}()
		}
		wg.Wait()
	}
	if connections.Load() > 10 {
		t.Fatalf("second request wave recreated connections: %d", connections.Load())
	}
}
