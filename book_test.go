package main

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestTransferSheetRecordToBook(t *testing.T) {
	records, err := ReadSheetRecords(sheetAppID, bookListTableID)
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

func TestCoverTokenOrEmptySkipsFailedUpload(t *testing.T) {
	token := coverTokenOrEmpty("cover.jpg", func(string) (string, error) {
		return "", errors.New("cover source unavailable")
	})

	if token != "" {
		t.Errorf("token = %q, want an empty attachment token after upload failure", token)
	}
}

func TestConvertBookToMapOmitsCoverWithoutUploadToken(t *testing.T) {
	book := &Book{BookId: "book-1", Title: "测试书", Cover: "https://example.com/cover.jpg"}
	fields := convertBookToMap(book, "")

	if _, exists := fields["封面"]; exists {
		t.Fatal("expected an unavailable cover upload to omit the attachment field")
	}
}
