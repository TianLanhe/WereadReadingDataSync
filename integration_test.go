package main

import (
	"os"
	"testing"
)

func requireLiveTest(t *testing.T) {
	t.Helper()
	if os.Getenv("WEREAD_RUN_LIVE_TESTS") != "1" {
		t.Skip("set WEREAD_RUN_LIVE_TESTS=1 to call live WeRead and Feishu APIs")
	}
}

func requireLiveMutationTest(t *testing.T) {
	t.Helper()
	if os.Getenv("WEREAD_RUN_LIVE_MUTATION_TESTS") != "1" {
		t.Skip("set WEREAD_RUN_LIVE_MUTATION_TESTS=1 to change live Feishu records")
	}
}
