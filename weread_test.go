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