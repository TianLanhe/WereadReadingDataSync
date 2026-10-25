package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBookPlanKeepsExistingCoversOnRatingChanges(t *testing.T) {
	shelf := &BookShelfInfoResponse{Book: []*BookInResponse{{BookId: "1", Title: "书", Paid: 1, Cover: "https://example.com/1.jpg"}}}
	details := map[string]*BookDetailResponse{"1": {NewRating: 801}}
	existing := []*SheetRecord{{RecordID: "r1", Fields: convertBookToMap(&Book{BookId: "1", Title: "书", CanRead: true, Score: 80}, "existing")}}
	plan, err := makeBookPlan(shelf, details, nil, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Updates) != 1 || len(plan.Covers) != 0 {
		t.Fatalf("updates=%d covers=%d", len(plan.Updates), len(plan.Covers))
	}
	delete(existing[0].Fields, "封面")
	plan, err = makeBookPlan(shelf, details, nil, existing)
	if err != nil || len(plan.Covers) != 1 {
		t.Fatalf("missing cover not scheduled: %v", err)
	}
}

func TestCoverUploadsRetryTwiceAndReuseDownload(t *testing.T) {
	var downloads, attempts atomic.Int64
	m := newCoverUploadManager(coverUploadOptions{Workers: 2, Interval: time.Millisecond, Wait: time.Second,
		Download: func(context.Context, string) ([]byte, error) { downloads.Add(1); return []byte("image"), nil },
		Upload: func(_ context.Context, _ string, _ []byte, gate func() error) (string, error) {
			if err := gate(); err != nil {
				return "", err
			}
			if attempts.Add(1) < 3 {
				return "", &HTTPStatusError{StatusCode: 500}
			}
			return "token", nil
		},
		Backoff: func(int) time.Duration { return time.Millisecond }})
	defer m.Close()
	m.StartUpload("cover")
	m.StartUpload("cover")
	token, err := m.GetResult("cover")
	if err != nil || token != "token" || attempts.Load() != 3 || downloads.Load() != 1 {
		t.Fatalf("token=%q attempts=%d downloads=%d err=%v", token, attempts.Load(), downloads.Load(), err)
	}
}

func TestCoverUploadPermanentErrorDoesNotRetry(t *testing.T) {
	var attempts atomic.Int64
	m := newCoverUploadManager(coverUploadOptions{Workers: 1, Interval: time.Millisecond, Wait: time.Second,
		Download: func(context.Context, string) ([]byte, error) { return []byte("image"), nil },
		Upload: func(_ context.Context, _ string, _ []byte, gate func() error) (string, error) {
			if err := gate(); err != nil {
				return "", err
			}
			attempts.Add(1)
			return "", &HTTPStatusError{StatusCode: 403}
		}, Backoff: func(int) time.Duration { return 0 }})
	defer m.Close()
	_, err := m.GetResult("cover")
	if err == nil || attempts.Load() != 1 {
		t.Fatalf("attempts=%d err=%v", attempts.Load(), err)
	}
}

func TestCoverAttemptsSharePacingAndRateLimitCooldown(t *testing.T) {
	var mu sync.Mutex
	var starts []time.Time
	var attempts atomic.Int64
	m := newCoverUploadManager(coverUploadOptions{Workers: 2, Interval: 15 * time.Millisecond, Wait: time.Second,
		Download: func(context.Context, string) ([]byte, error) { return []byte("image"), nil },
		Upload: func(_ context.Context, _ string, _ []byte, gate func() error) (string, error) {
			if err := gate(); err != nil {
				return "", err
			}
			mu.Lock()
			starts = append(starts, time.Now())
			mu.Unlock()
			if attempts.Add(1) == 1 {
				return "", &HTTPStatusError{StatusCode: 429, RetryAfter: 60 * time.Millisecond}
			}
			return "ok", nil
		},
		Backoff: func(int) time.Duration { return 0 }})
	m.StartUpload("one")
	m.StartUpload("two")
	_, err1 := m.GetResult("one")
	_, err2 := m.GetResult("two")
	m.Close()
	if err1 != nil || err2 != nil {
		t.Fatal(errors.Join(err1, err2))
	}
	if len(starts) != 3 {
		t.Fatalf("attempts=%d", len(starts))
	}
	if starts[1].Sub(starts[0]) < 55*time.Millisecond {
		t.Fatal("other worker bypassed shared cooldown")
	}
	if starts[2].Sub(starts[1]) < 13*time.Millisecond {
		t.Fatal("retry bypassed shared pacing")
	}
}

func TestCoverUploadStopsAfterThreeFailures(t *testing.T) {
	var attempts atomic.Int64
	m := newCoverUploadManager(coverUploadOptions{Workers: 1, Interval: time.Millisecond, Wait: time.Second, Download: func(context.Context, string) ([]byte, error) { return []byte("image"), nil }, Upload: func(_ context.Context, _ string, _ []byte, gate func() error) (string, error) {
		if err := gate(); err != nil {
			return "", err
		}
		attempts.Add(1)
		return "", &HTTPStatusError{StatusCode: 500}
	}, Backoff: func(int) time.Duration { return 0 }})
	defer m.Close()
	_, err := m.GetResult("cover")
	if err == nil || attempts.Load() != 3 {
		t.Fatalf("attempts=%d err=%v", attempts.Load(), err)
	}
}
