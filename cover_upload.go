package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// CoverUploadManager 管理并发封面上传
type CoverUploadManager struct {
	limiter *rate.Limiter
	mu      sync.RWMutex
	results map[string]*uploadResult
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// uploadResult 存储上传结果
type uploadResult struct {
	token string
	err   error
	done  bool
}

// NewCoverUploadManager 创建新的封面上传管理器
func NewCoverUploadManager(qps int) *CoverUploadManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &CoverUploadManager{
		limiter: rate.NewLimiter(rate.Limit(qps), qps),
		results: make(map[string]*uploadResult),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// StartUpload 开始异步上传封面
func (m *CoverUploadManager) StartUpload(coverURL string) {
	m.mu.Lock()
	if _, exists := m.results[coverURL]; exists {
		m.mu.Unlock()
		return // 已经在处理中
	}
	m.results[coverURL] = &uploadResult{done: false}
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		// 使用速率限制器
		err := m.limiter.Wait(m.ctx)
		if err != nil {
			m.mu.Lock()
			m.results[coverURL] = &uploadResult{err: fmt.Errorf("rate limiter error: %w", err), done: true}
			m.mu.Unlock()
			return
		}

		token, err := UploadCoverToSheet(coverURL)

		m.mu.Lock()
		m.results[coverURL] = &uploadResult{token: token, err: err, done: true}
		m.mu.Unlock()
	}()
}

// GetResult 获取上传结果，如果未完成则阻塞等待
func (m *CoverUploadManager) GetResult(coverURL string) (string, error) {
	// 检查是否已启动上传
	m.mu.RLock()
	_, exists := m.results[coverURL]
	m.mu.RUnlock()

	if !exists {
		// 如果未启动，则同步上传
		m.StartUpload(coverURL)
	}

	// 等待结果
	for {
		m.mu.RLock()
		result := m.results[coverURL]
		if result.done {
			token := result.token
			err := result.err
			m.mu.RUnlock()
			return token, err
		}
		m.mu.RUnlock()

		// 短暂休眠避免CPU占用过高
		time.Sleep(10 * time.Millisecond)
	}
}

// PreloadCovers 预加载所有需要的封面
func (m *CoverUploadManager) PreloadCovers(books map[string]*Book) {
	for _, book := range books {
		if book.Cover != "" {
			m.StartUpload(book.Cover)
		}
	}
}

// Close 等待所有上传完成
func (m *CoverUploadManager) Close() {
	m.wg.Wait()
	m.cancel()
}