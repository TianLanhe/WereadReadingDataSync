package main

import (
	"testing"
	"time"
)

func TestTokenBucketStopsAcceptingTokens(t *testing.T) {
	bucket := NewTokenBucket(1, 1)
	if !bucket.SyncTake(1, time.Millisecond) {
		t.Fatal("expected the initial token to be available")
	}

	bucket.Stop()
	if bucket.SyncTake(1, time.Millisecond) {
		t.Fatal("expected a stopped bucket to reject tokens")
	}
}
