package main

import (
	"sync"
	"time"
)

// TokenBucket limits operations to a fixed rate while allowing a small burst.
type TokenBucket struct {
	tokens chan struct{}
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

func NewTokenBucket(qps, capacity int64) *TokenBucket {
	if qps < 1 {
		qps = 1
	}
	if capacity < 1 {
		capacity = 1
	}

	bucket := &TokenBucket{
		tokens: make(chan struct{}, capacity),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	for i := int64(0); i < capacity; i++ {
		bucket.tokens <- struct{}{}
	}

	interval := time.Second / time.Duration(qps)
	if interval < time.Nanosecond {
		interval = time.Nanosecond
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(bucket.done)
		for {
			select {
			case <-bucket.stop:
				return
			case <-ticker.C:
				select {
				case bucket.tokens <- struct{}{}:
				default:
				}
			}
		}
	}()

	return bucket
}

// SyncTake waits up to waitTime for the requested number of tokens.
func (b *TokenBucket) SyncTake(count int, waitTime time.Duration) bool {
	if count <= 0 {
		return true
	}
	deadline := time.NewTimer(waitTime)
	defer deadline.Stop()
	for i := 0; i < count; i++ {
		select {
		case <-b.stop:
			return false
		case <-deadline.C:
			return false
		case <-b.tokens:
		}
	}
	return true
}

func (b *TokenBucket) Stop() {
	b.once.Do(func() {
		close(b.stop)
		<-b.done
	})
}
