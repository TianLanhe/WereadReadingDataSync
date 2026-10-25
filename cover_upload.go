package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type coverUploadOptions struct {
	Workers        int
	Total          int
	Interval, Wait time.Duration
	Download       func(context.Context, string) ([]byte, error)
	Upload         func(context.Context, string, []byte, func() error) (string, error)
	Backoff        func(int) time.Duration
	Reporter       Reporter
}
type uploadResult struct {
	token string
	err   error
	done  chan struct{}
}
type CoverUploadManager struct {
	mu                         sync.Mutex
	results                    map[string]*uploadResult
	jobs                       chan string
	wg                         sync.WaitGroup
	once                       sync.Once
	ctx                        context.Context
	cancel                     context.CancelFunc
	options                    coverUploadOptions
	pacerMu                    sync.Mutex
	next                       time.Time
	completed                  int
	succeeded, failed, retried int
}

func NewCoverUploadManager(qps int64, wait time.Duration) *CoverUploadManager {
	if qps < 1 {
		qps = 1
	}
	return newCoverUploadManager(coverUploadOptions{Workers: 5, Interval: time.Second/time.Duration(qps) + 5*time.Millisecond, Wait: wait})
}
func newCoverUploadManager(o coverUploadOptions) *CoverUploadManager {
	if o.Workers < 1 {
		o.Workers = 5
	}
	if o.Wait <= 0 {
		o.Wait = maxPicUploadWaitTime
	}
	if o.Interval <= 0 {
		o.Interval = 205 * time.Millisecond
	}
	if o.Download == nil {
		o.Download = downloadCover
	}
	if o.Upload == nil {
		o.Upload = func(ctx context.Context, name string, data []byte, gate func() error) (string, error) {
			config, err := loadAppConfig()
			if err != nil {
				return "", err
			}
			return uploadMediaToBaseWithGate(ctx, name, data, config.Feishu.BaseAppID, gate)
		}
	}
	if o.Backoff == nil {
		o.Backoff = func(attempt int) time.Duration { return time.Duration(1<<(attempt-1)) * time.Second }
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.Wait)
	m := &CoverUploadManager{results: make(map[string]*uploadResult), jobs: make(chan string, 32), ctx: ctx, cancel: cancel, options: o}
	for i := 0; i < o.Workers; i++ {
		m.wg.Add(1)
		go m.worker()
	}
	return m
}
func (m *CoverUploadManager) StartUpload(cover string) {
	m.mu.Lock()
	if _, exists := m.results[cover]; exists {
		m.mu.Unlock()
		return
	}
	res := &uploadResult{done: make(chan struct{})}
	m.results[cover] = res
	m.mu.Unlock()
	select {
	case m.jobs <- cover:
	case <-m.ctx.Done():
		m.mu.Lock()
		res.err = m.ctx.Err()
		close(res.done)
		m.mu.Unlock()
	}
}
func (m *CoverUploadManager) GetResult(cover string) (string, error) {
	m.StartUpload(cover)
	m.mu.Lock()
	res := m.results[cover]
	m.mu.Unlock()
	select {
	case <-res.done:
		return res.token, res.err
	case <-m.ctx.Done():
		return "", m.ctx.Err()
	}
}
func (m *CoverUploadManager) Close() { m.once.Do(func() { close(m.jobs); m.wg.Wait(); m.cancel() }) }
func (m *CoverUploadManager) worker() {
	defer m.wg.Done()
	for cover := range m.jobs {
		token, err := m.process(cover)
		m.mu.Lock()
		res := m.results[cover]
		res.token, res.err = token, err
		close(res.done)
		m.completed++
		if err == nil {
			m.succeeded++
		} else {
			m.failed++
		}
		done, total := m.completed, len(m.results)
		if m.options.Total > 0 {
			total = m.options.Total
		}
		succeeded, failed, retried := m.succeeded, m.failed, m.retried
		m.mu.Unlock()
		report(m.options.Reporter, Event{Kind: EventProgress, Stage: "book", Task: "covers", Done: done, Total: total, Success: succeeded, Failed: failed, Retried: retried})
	}
}
func waitContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (m *CoverUploadManager) pace() error {
	// Hold the turn only during pacing, never during the HTTP request. Each wake
	// rechecks the deadline so a rate-limit response can extend a shared cooldown.
	for {
		m.pacerMu.Lock()
		wait := time.Until(m.next)
		if wait <= 0 {
			m.next = time.Now().Add(m.options.Interval)
			m.pacerMu.Unlock()
			return m.ctx.Err()
		}
		m.pacerMu.Unlock()
		if err := waitContext(m.ctx, wait); err != nil {
			return err
		}
	}
}
func (m *CoverUploadManager) cooldown(d time.Duration) {
	m.pacerMu.Lock()
	defer m.pacerMu.Unlock()
	until := time.Now().Add(d)
	if until.After(m.next) {
		m.next = until
	}
}
func retryableCoverError(err error) (bool, time.Duration) {
	var status *HTTPStatusError
	if errors.As(err, &status) {
		limited := status.StatusCode == 429 || status.Code == 99991400
		return limited || status.StatusCode >= 500 || status.Code == 1061001 || status.Code == 1061006 || status.Code == 1061045, status.RetryAfter
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return false, 0
	}
	var n net.Error
	return errors.As(err, &n) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF), 0
}
func (m *CoverUploadManager) process(cover string) (string, error) {
	var data []byte
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		// A successful download is reused by upload retries.
		if data == nil {
			data, err = m.options.Download(m.ctx, cover)
		} else {
			err = nil
		}
		if err == nil {
			var token string
			token, err = m.options.Upload(m.ctx, cover, data, m.pace)
			if err == nil {
				return token, nil
			}
		}
		retry, reset := retryableCoverError(err)
		if !retry {
			return "", err
		}
		var status *HTTPStatusError
		limited := errors.As(err, &status) && (status.StatusCode == 429 || status.Code == 99991400)
		if limited {
			if reset <= 0 {
				reset = time.Second
			}
			m.cooldown(reset)
		}
		if attempt == 2 {
			return "", fmt.Errorf("已尝试 3 次: %w", err)
		}
		delay := m.options.Backoff(attempt + 1)
		if reset > delay {
			delay = reset
		}
		m.mu.Lock()
		m.retried++
		m.mu.Unlock()
		report(m.options.Reporter, Event{Kind: EventRetry, Stage: "book", Task: "covers", Attempt: attempt + 1, Wait: delay, Err: err})
		if err = waitContext(m.ctx, delay); err != nil {
			return "", err
		}
	}
	return "", err
}
func downloadCover(ctx context.Context, cover string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cover, nil)
	if err != nil {
		return nil, err
	}
	resp, err := syncHTTPClient.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			return nil, u.Err
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
	}
	const maxSize = 20 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, errors.New("封面超过20MB")
	}
	return data, nil
}
