package main

import (
	"encoding/json"
	"testing"
)

func TestGetAccessToken(t *testing.T) {
	requireLiveTest(t)
	at, err := GetAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if at == "" {
		t.Fatal("GetAccessToken returned an empty token")
	}
}

func TestGetMineReadBook(t *testing.T) {
	requireLiveTest(t)
	all, _, _, err := GetMineReadBook()
	if err != nil {
		t.Fatal(err)
	}
	alldata, _ := json.Marshal(all)
	t.Logf("all:%s", alldata)
}

func TestGetYearReadingTime(t *testing.T) {
	requireLiveTest(t)
	got, got2, err := GetYearReadingTime(0)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	got2JSON, _ := json.Marshal(got2)
	t.Logf("data:%s, map:%s", string(gotJSON), string(got2JSON))
}
