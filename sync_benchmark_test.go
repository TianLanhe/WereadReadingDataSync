package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// benchmarkFixture intercepts the HTTP boundary, while running the real readers,
// diff calculation, cover manager, and writers. No live endpoints are contacted.
func benchmarkFixture(tb testing.TB) (*atomic.Int64, *atomic.Int64) {
	tb.Helper()
	dir := tb.TempDir()
	configFile := filepath.Join(dir, "config.json")
	authFile := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(configFile, []byte(`{"feishu":{"appId":"fixture-app","appSecret":"fixture-secret","baseAppId":"fixture-base","bookListTableId":"books","readTimeTableId":"days"}}`), 0600); err != nil {
		tb.Fatal(err)
	}
	if err := saveWeReadCredentials(authFile, weReadCredentials{VID: "fixture-user", DeviceID: "fixture-device", RefreshToken: "fixture-refresh"}); err != nil {
		tb.Fatal(err)
	}
	tb.Setenv("WEREAD_SYNC_CONFIG", configFile)
	tb.Setenv("WEREAD_AUTH_FILE", authFile)
	oldTransport := http.DefaultTransport
	oldSyncClient := syncHTTPClient
	oldToken, oldExpiry := cachedToken, tokenExpiration
	oldAccess, oldAccessExpiry := cachedAccessToken, tokenExpiryTime
	cachedToken, tokenExpiration = "Bearer fixture-token", time.Now().Add(time.Hour)
	cachedAccessToken, tokenExpiryTime = "fixture-access", time.Now().Add(time.Hour)
	requests, uploads := &atomic.Int64{}, &atomic.Int64{}
	shelf := &BookShelfInfoResponse{}
	var records []*SheetRecord
	for i := 0; i < 31; i++ {
		id := strconv.Itoa(i)
		shelf.Book = append(shelf.Book, &BookInResponse{BookId: id, Title: fmt.Sprintf("测试书%02d", i), Author: "作者", Paid: 1, Cover: "https://covers.example/" + id + ".jpg", Categories: []Category{{Title: "教育"}}})
		seconds := int64(600)
		if i == 0 {
			seconds = 1200
		}
		shelf.BookProgress = append(shelf.BookProgress, &BookProgress{BookId: id, ReadingTime: seconds, Progress: 20})
		if i < 30 {
			fields := convertBookToMap(&Book{BookId: id, Title: fmt.Sprintf("测试书%02d", i), Author: "作者", CanRead: true, Categories: []string{"教育"}, ReadTime: 600, Progress: 0.2, Score: 80, Words: 1, Intro: "简介"}, "existing-cover")
			records = append(records, &SheetRecord{RecordID: "record-" + id, Fields: fields})
		}
	}
	http.DefaultTransport = testRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		time.Sleep(40 * time.Millisecond)
		var payload any
		switch {
		case r.URL.Host == "covers.example":
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("fixture image"))}, nil
		case r.URL.Path == "/book/info":
			payload = &BookDetailResponse{Intro: "简介", TotalWords: 10000, NewRating: 801, Categories: []Category{{Title: "教育"}}}
		case r.URL.Path == "/shelf/sync":
			payload = shelf
		case r.URL.Path == "/mine/readbook":
			payload = &MineReadBookResponse{}
		case r.URL.Path == "/readdata/detail":
			year := time.Now().Year()
			if raw := r.URL.Query().Get("baseTime"); raw != "0" {
				n, _ := strconv.ParseInt(raw, 10, 64)
				year = time.Unix(n, 0).Year()
			}
			stamp := time.Date(year, 1, 2, 0, 0, 0, 0, time.Local).Unix()
			payload = map[string]any{"dailyReadTimes": map[string]int64{strconv.FormatInt(stamp, 10): 600}}
		case strings.HasSuffix(r.URL.Path, "/records/search"):
			items := records
			if strings.Contains(r.URL.Path, "/tables/days/") {
				items = nil
			}
			payload = map[string]any{"code": 0, "data": map[string]any{"items": items, "has_more": false}}
		case r.URL.Path == "/open-apis/drive/v1/medias/upload_all":
			uploads.Add(1)
			payload = map[string]any{"code": 0, "data": map[string]any{"file_token": "new-cover"}}
		case strings.Contains(r.URL.Path, "/records/batch_"):
			payload = map[string]any{"code": 0, "data": map[string]any{}}
		default:
			return nil, fmt.Errorf("unexpected fixture request %s %s", r.Method, r.URL.Path)
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})
	client := *syncHTTPClient
	client.Transport = http.DefaultTransport
	syncHTTPClient = &client
	tb.Cleanup(func() {
		syncHTTPClient = oldSyncClient
		http.DefaultTransport = oldTransport
		cachedToken, tokenExpiration = oldToken, oldExpiry
		cachedAccessToken, tokenExpiryTime = oldAccess, oldAccessExpiry
	})
	return requests, uploads
}

func BenchmarkSyncScenario(b *testing.B) {
	requests, uploads := benchmarkFixture(b)
	config, err := loadAppConfig()
	if err != nil {
		b.Fatal(err)
	}
	// Both versions format their console output and discard device I/O equally.
	out, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = oldStdout; out.Close() }()
	reporter := NewConsoleReporter(out, false, 80)
	defer reporter.Close()
	runner := &syncRunner{config: config.Feishu, reporter: reporter}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := runner.readTime(); err != nil {
			b.Fatal(err)
		}
		if _, err := runner.books(); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(requests.Load())/float64(b.N), "requests/op")
	b.ReportMetric(float64(uploads.Load())/float64(b.N), "uploads/op")
}
