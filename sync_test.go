package main

import (
	"errors"
	"fmt"
	"golang.org/x/term"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDetailWorkersBoundConcurrencyAndReturnAllResults(t *testing.T) {
	var active, peak atomic.Int64
	ids := make([]string, 35)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	result, err := batchGetBookDetails(ids, func(string) (*BookDetailResponse, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		time.Sleep(time.Millisecond)
		return &BookDetailResponse{Intro: "ok"}, nil
	}, nil)
	if err != nil || len(result) != 35 {
		t.Fatalf("results=%d err=%v", len(result), err)
	}
	if peak.Load() > 10 || peak.Load() < 2 {
		t.Fatalf("peak concurrency=%d", peak.Load())
	}
}

func TestDetailFailureNeverReturnsPartialSuccess(t *testing.T) {
	result, err := batchGetBookDetails([]string{"good", "bad", "later"}, func(id string) (*BookDetailResponse, error) {
		if id == "bad" {
			return nil, errors.New("unavailable")
		}
		time.Sleep(time.Millisecond)
		return &BookDetailResponse{}, nil
	}, nil)
	if err == nil || result != nil {
		t.Fatalf("partial result accepted: %v %v", result, err)
	}
}

func TestReadTimePlanPreservesAddUpdateDelete(t *testing.T) {
	d1 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	d2 := d1.AddDate(0, 0, 1)
	d3 := d1.AddDate(0, 0, 2)
	records := []*SheetRecord{{RecordID: "update", Fields: convertReadTimeToMap(d1, 60)}, {RecordID: "delete", Fields: convertReadTimeToMap(d3, 10)}}
	add, update, remove, err := CalculateReadTime(map[time.Time]int64{d1: 120, d2: 60}, records)
	if err != nil || len(add) != 1 || len(update) != 1 || len(remove) != 1 {
		t.Fatalf("bad counts: %d %d %d %v", len(add), len(update), len(remove), err)
	}
	if update[0].RecordID != "update" || remove[0] != "delete" || getInt64Value(add[0], "日期") != d2.UnixMilli() {
		t.Fatal("wrong records")
	}
}

func TestSyncSourceFailureDoesNotWriteRecords(t *testing.T) {
	requests, _ := benchmarkFixture(t)
	_ = requests
	// Replacing only one external endpoint keeps real orchestration and writers.
	original := syncHTTPClient
	client := *syncHTTPClient
	transport := http.DefaultTransport
	var writes atomic.Int64
	client.Transport = testRoundTrip(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/records/batch_") {
			writes.Add(1)
		}
		if r.URL.Path == "/book/info" {
			return nil, errors.New("fixture failed")
		}
		return transport.RoundTrip(r)
	})
	syncHTTPClient = &client
	t.Cleanup(func() { syncHTTPClient = original })
	config, err := loadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&syncRunner{config: config.Feishu}).books()
	if writes.Load() != 0 {
		t.Fatal("source failure caused a remote write")
	}
	if err == nil || !strings.Contains(err.Error(), "fixture failed") {
		t.Fatalf("want source failure, got %v", err)
	}
}

func TestCompleteSyncRetainsRatingWritesWithoutReuploadingExistingCovers(t *testing.T) {
	_, uploads := benchmarkFixture(t)
	config, err := loadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	s := &syncRunner{config: config.Feishu}
	if os.Getenv("WEREAD_CONSOLE_PREVIEW") == "1" {
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			width = 80
		}
		r := NewConsoleReporter(os.Stdout, term.IsTerminal(int(os.Stdout.Fd())), width)
		defer r.Close()
		s.reporter = r
		r.Report(Event{Kind: EventRun, Task: "模拟演示"})
	}
	days, err := s.readTime()
	if err != nil {
		t.Fatal(err)
	}
	books, err := s.books()
	if err != nil {
		t.Fatal(err)
	}
	if days.Add != 5 || books.Add != 1 || books.Update != 30 || uploads.Load() != 1 {
		t.Fatalf("days=%+v books=%+v uploads=%d", days, books, uploads.Load())
	}
}

func TestSyncCountsOnlyConfirmedBatchesOnFailure(t *testing.T) {
	benchmarkFixture(t)
	old := syncHTTPClient
	client := *old
	var calls int
	client.Transport = testRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 2 {
			return &http.Response{StatusCode: 500, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":1}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0}`))}, nil
	})
	syncHTTPClient = &client
	t.Cleanup(func() { syncHTTPClient = old })
	records := make([]map[string]interface{}, 201)
	for i := range records {
		records[i] = map[string]interface{}{"书名": "测试"}
	}
	var stats SyncStats
	err := (&syncRunner{config: feishuConfig{BaseAppID: "base"}}).write("book", "table", records, nil, nil, &stats)
	if err == nil || stats.Add != 200 {
		t.Fatalf("confirmed=%d err=%v", stats.Add, err)
	}
}

func TestBookPlanRejectsMalformedRecordBeforeWriting(t *testing.T) {
	_, err := makeBookPlan(&BookShelfInfoResponse{}, nil, nil, []*SheetRecord{nil})
	if err == nil {
		t.Fatal("nil record accepted")
	}
}
