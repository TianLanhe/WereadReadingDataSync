package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"time"
)

const (
	sheetAppID             = "ZOdzbb1CiaBdEbslbTzcOw4FnOh"
	bookListTableID        = "tbl7Tl0Y60BrBLy3"
	readTimeTableID        = "tblKmQNrjVSuQxkN"
	tokenURL               = "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	appID                  = "cli_aa97e021ccf9dcb5"
	appSecret              = "eFOMPafjjN9efiEVOjZaYgKCo3hxEOM7"
	readSheetURLTemplate   = "https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/records/search?page_size=500"
	batchDeleteURLTemplate = "https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/records/batch_delete"
	batchAddURLTemplate    = "https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/records/batch_create"
	batchUpdateURLTemplate = "https://open.feishu.cn/open-apis/bitable/v1/apps/%s/tables/%s/records/batch_update"
	uploadPicURL           = "https://open.feishu.cn/open-apis/drive/v1/medias/upload_all"
)

type TokenRequest struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type TokenResponse struct {
	Code              int    `json:"code"`
	TenantAccessToken string `json:"tenant_access_token"`
}

type SheetRecord struct {
	Fields   map[string]interface{} `json:"fields"`
	RecordID string                 `json:"record_id,omitempty"`
}

type SheetResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		HasMore   bool           `json:"has_more"`
		Items     []*SheetRecord `json:"items"`
		Total     int            `json:"total"`
		PageToken string         `json:"page_token"`
		FileToken string         `json:"file_token"`
	} `json:"data"`
}

var (
	cachedToken        string
	tokenExpiration    time.Time
	tokenExpirationTTL = 2 * time.Hour // 假设token有效期为2小时
)

const maxBatchCreateRecords = 200

func GetTalentAccessToken() (string, error) {
	if time.Now().Before(tokenExpiration) && cachedToken != "" {
		return cachedToken, nil
	}

	jsonData, err := json.Marshal(TokenRequest{
		AppID:     appID,
		AppSecret: appSecret,
	})
	if err != nil {
		return "", err
	}

	headers := map[string]string{
		"Content-Type": "application/json; charset=utf-8",
	}

	respBody, err := SendHTTPRequest("POST", tokenURL, jsonData, headers)
	if err != nil {
		return "", err
	}

	var tokenResp TokenResponse
	err = json.Unmarshal(respBody, &tokenResp)
	if err != nil {
		return "", err
	}

	if tokenResp.Code != 0 {
		return "", fmt.Errorf("unexpected response code: %d", tokenResp.Code)
	}

	cachedToken = "Bearer " + tokenResp.TenantAccessToken
	tokenExpiration = time.Now().Add(tokenExpirationTTL)
	return cachedToken, nil
}

// ReadSheetRecords 批量读取多维表格的记录
func ReadSheetRecords(appID, tableID string) ([]*SheetRecord, error) {
	token, err := GetTalentAccessToken()
	if err != nil {
		fmt.Printf("Error getting access token: %v\n", err)
		return nil, err
	}

	var records []*SheetRecord
	pageToken := ""
	baseURL := fmt.Sprintf(readSheetURLTemplate, appID, tableID)

	for {
		// 构建请求URL，包含page_token
		url := baseURL
		if pageToken != "" {
			url = fmt.Sprintf("%s&page_token=%s", baseURL, pageToken)
		}

		// 设置请求体为JSON空对象
		reqBody := []byte("{}")

		headers := map[string]string{
			"Authorization": token,
			"Content-Type":  "application/json; charset=utf-8",
		}

		respBody, err := SendHTTPRequest("POST", url, reqBody, headers)
		if err != nil {
			fmt.Printf("Error sending HTTP request: %v\n", err)
			return nil, err
		}

		var sheetResp SheetResponse
		err = json.Unmarshal(respBody, &sheetResp)
		if err != nil {
			fmt.Printf("Error decoding sheet response: %v\n", err)
			return nil, err
		}

		if sheetResp.Code != 0 {
			fmt.Printf("Unexpected response code: %d, message: %s\n", sheetResp.Code, sheetResp.Msg)
			return nil, fmt.Errorf("unexpected response code: %d, message: %s", sheetResp.Code, sheetResp.Msg)
		}

		// 收集当前页的记录
		for _, item := range sheetResp.Data.Items {
			records = append(records, item)
		}

		// 检查是否还有更多数据
		if !sheetResp.Data.HasMore {
			break
		}

		// 更新page_token
		pageToken = sheetResp.Data.PageToken
	}

	return records, nil
}

// BatchDeleteSheetRecords 批量删除多维表格的记录
func BatchDeleteSheetRecords(appID, tableID string, recordIDs []string) error {
	token, err := GetTalentAccessToken()
	if err != nil {
		return fmt.Errorf("获取访问令牌失败: %w", err)
	}

	url := fmt.Sprintf(batchDeleteURLTemplate, appID, tableID)

	// 构建新的批量删除请求体
	reqBody, err := json.Marshal(map[string]interface{}{
		"records": recordIDs,
	})
	if err != nil {
		return fmt.Errorf("构造请求体失败: %w", err)
	}

	headers := map[string]string{
		"Authorization": token,
		"Content-Type":  "application/json; charset=utf-8",
	}

	respBody, err := SendHTTPRequest("POST", url, reqBody, headers)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %w", err)
	}

	// 检查HTTP状态码
	var sheetResp SheetResponse
	err = json.Unmarshal(respBody, &sheetResp)
	if err != nil {
		return fmt.Errorf("解析响应体失败: %w", err)
	}

	if sheetResp.Code != 0 {
		return fmt.Errorf("批量删除记录失败，错误码: %d, 错误信息: %s", sheetResp.Code, sheetResp.Msg)
	}

	return nil
}

// BatchAddSheetRecords 批量新增多维表格记录
func BatchAddSheetRecords(appID, tableID string, records []map[string]interface{}) error {
	token, err := GetTalentAccessToken()
	if err != nil {
		return fmt.Errorf("获取访问令牌失败: %w", err)
	}

	url := fmt.Sprintf(batchAddURLTemplate, appID, tableID)

	headers := map[string]string{
		"Authorization": token,
		"Content-Type":  "application/json; charset=utf-8",
	}

	for _, batch := range splitRecordBatches(records, maxBatchCreateRecords) {
		payload := make([]map[string]interface{}, 0, len(batch))
		for _, record := range batch {
			payload = append(payload, map[string]interface{}{
				"fields": record,
			})
		}
		reqBody, err := json.Marshal(map[string]interface{}{
			"records": payload,
		})
		if err != nil {
			return fmt.Errorf("构造请求体失败: %w", err)
		}

		respBody, err := SendHTTPRequest("POST", url, reqBody, headers)
		if err != nil {
			return fmt.Errorf("发送HTTP请求失败: %w", err)
		}

		var sheetResp SheetResponse
		if err := json.Unmarshal(respBody, &sheetResp); err != nil {
			return fmt.Errorf("解析响应体失败: %w", err)
		}
		if sheetResp.Code != 0 {
			return fmt.Errorf("批量添加记录失败，错误码: %d, 错误信息: %s", sheetResp.Code, sheetResp.Msg)
		}
	}

	return nil
}

func splitRecordBatches(records []map[string]interface{}, size int) [][]map[string]interface{} {
	var batches [][]map[string]interface{}
	for start := 0; start < len(records); start += size {
		end := start + size
		if end > len(records) {
			end = len(records)
		}
		batches = append(batches, records[start:end])
	}
	return batches
}

// BatchUpdateSheetRecord 批量更新多维表格记录
func BatchUpdateSheetRecords(appID, tableID string, records []*SheetRecord) error {
	token, err := GetTalentAccessToken()
	if err != nil {
		return fmt.Errorf("获取访问令牌失败: %w", err)
	}

	url := fmt.Sprintf(batchUpdateURLTemplate, appID, tableID)

	reqBody, err := json.Marshal(map[string]interface{}{
		"records": records,
	})
	if err != nil {
		return fmt.Errorf("构造请求体失败: %w", err)
	}

	headers := map[string]string{
		"Authorization": token,
		"Content-Type":  "application/json; charset=utf-8",
	}

	respBody, err := SendHTTPRequest("POST", url, reqBody, headers)
	if err != nil {
		return fmt.Errorf("发送HTTP请求失败: %w", err)
	}

	// 检查HTTP状态码
	var sheetResp SheetResponse
	err = json.Unmarshal(respBody, &sheetResp)
	if err != nil {
		return fmt.Errorf("解析响应体失败: %w", err)
	}

	if sheetResp.Code != 0 {
		return fmt.Errorf("批量更新记录失败，错误码: %d, 错误信息: %s", sheetResp.Code, sheetResp.Msg)
	}

	return nil
}

// UploadMedia 上传素材到飞书
func UploadMediaToSheet(fileName string, data []byte) (string, error) {
	token, err := GetTalentAccessToken()
	if err != nil {
		return "", fmt.Errorf("获取访问令牌失败: %w", err)
	}

	// 创建一个缓冲区来存储请求体
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文本字段
	fields := mediaUploadFields(fileName, len(data))

	for key, value := range fields {
		writer.WriteField(key, value)
	}

	// 添加文件字段
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		fmt.Println("Error creating form file:", err)
		return "", err
	}

	reader := bytes.NewReader(data)
	_, err = io.Copy(part, reader)
	if err != nil {
		fmt.Println("Error copying image to request:", err)
		return "", err
	}

	// 关闭writer以完成请求体
	err = writer.Close()
	if err != nil {
		fmt.Println("Error closing writer:", err)
		return "", err
	}

	headers := map[string]string{
		"Content-Type":  writer.FormDataContentType(),
		"Authorization": token,
	}

	respBody, err := SendHTTPRequest("POST", uploadPicURL, body.Bytes(), headers)
	if err != nil {
		return "", fmt.Errorf("发送HTTP请求失败: %w", err)
	}

	// 检查HTTP状态码
	var mediaResp SheetResponse
	err = json.Unmarshal(respBody, &mediaResp)
	if err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if mediaResp.Code != 0 {
		return "", fmt.Errorf("上传素材失败，错误码: %d", mediaResp.Code)
	}

	return mediaResp.Data.FileToken, nil
}

func mediaUploadFields(fileName string, dataSize int) map[string]string {
	return map[string]string{
		"file_name":   fileName,
		"parent_type": "bitable_image",
		"parent_node": sheetAppID,
		"size":        fmt.Sprintf("%d", dataSize),
	}
}
