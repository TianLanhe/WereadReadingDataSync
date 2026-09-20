package main

import (
	"fmt"
	"net/url"
	"testing"
)

func TestFeishuEndpointsUsePublicAPIHost(t *testing.T) {
	endpoints := []string{
		tokenURL,
		fmt.Sprintf(readSheetURLTemplate, "app", "table"),
		fmt.Sprintf(batchDeleteURLTemplate, "app", "table"),
		fmt.Sprintf(batchAddURLTemplate, "app", "table"),
		fmt.Sprintf(batchUpdateURLTemplate, "app", "table"),
		uploadPicURL,
	}

	for _, endpoint := range endpoints {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			t.Fatalf("parse endpoint %q: %v", endpoint, err)
		}
		if parsed.Scheme != "https" || parsed.Host != "open.feishu.cn" {
			t.Errorf("endpoint %q uses %s://%s, want https://open.feishu.cn", endpoint, parsed.Scheme, parsed.Host)
		}
	}
}

func TestMediaUploadFieldsTargetCurrentBase(t *testing.T) {
	fields := mediaUploadFields("cover.jpg", 123)

	if fields["parent_type"] != "bitable_image" {
		t.Errorf("parent_type = %q, want bitable_image", fields["parent_type"])
	}
	if fields["parent_node"] != sheetAppID {
		t.Errorf("parent_node = %q, want current Base ID %q", fields["parent_node"], sheetAppID)
	}
}

func TestSplitRecordBatchesRespectsBatchLimit(t *testing.T) {
	records := make([]map[string]interface{}, 401)
	for i := range records {
		records[i] = map[string]interface{}{"index": i}
	}

	batches := splitRecordBatches(records, 200)
	if len(batches) != 3 {
		t.Fatalf("batch count = %d, want 3", len(batches))
	}
	for i, wantSize := range []int{200, 200, 1} {
		if len(batches[i]) != wantSize {
			t.Errorf("batch %d size = %d, want %d", i, len(batches[i]), wantSize)
		}
	}
	if batches[0][0]["index"] != 0 || batches[2][0]["index"] != 400 {
		t.Fatal("batches do not preserve record order")
	}
}
