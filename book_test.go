package main

import (
	"encoding/json"
	"testing"
)

func TestTransferSheetRecordToBook(t *testing.T) {
	records, err := ReadSheetRecords()
	if err != nil {
		t.Fatal(err)
	}

	got, got2 := TransferSheetRecordToBook(records)
	gotJSON, _ := json.Marshal(got)
	got2JSON, _ := json.Marshal(got2)
	t.Logf("data:%s, map:%s", string(gotJSON), string(got2JSON))
}

func TestUploadCoverToSheet(t *testing.T) {
	token, err := UploadCoverToSheet("https://wfqqreader-1252317822.image.myqcloud.com/cover/600/33810600/t6_33810600.jpg")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("token:%s", token)
}
