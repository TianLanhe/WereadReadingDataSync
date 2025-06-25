package main

import (
	"encoding/json"
	"testing"
)

func TestGetAccessToken(t *testing.T) {
	at, err := GetAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("data:%s", at)
}

func TestGetMineReadBook(t *testing.T) {
	all, _, _, err := GetMineReadBook()
	if err != nil {
		t.Fatal(err)
	}
	alldata, _ := json.Marshal(all)
	t.Logf("all:%s", alldata)
}

func TestGetYearReadingTime(t *testing.T) {
	got, got2, err := GetYearReadingTime(0)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	got2JSON, _ := json.Marshal(got2)
	t.Logf("data:%s, map:%s", string(gotJSON), string(got2JSON))
}
