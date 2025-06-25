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
	tokenURL       = "https://fsopen.bytedance.net/open-apis/auth/v3/tenant_access_token/internal"
	appID          = "cli_a4d37a9d357cd013"
	appSecret      = "OeHwPL7hGxRf7TFOFlrtxbCrp3xROaTV"
	readSheetURL   = "https://fsopen.bytedance.net/open-apis/bitable/v1/apps/Q8rAbZMgwacvnXswZ2VlBNc1goh/tables/tblWb9n3b9nNVBJb/records/search?page_size=500"
	batchDeleteURL = "https://open.larkoffice.com/open-apis/bitable/v1/apps/Q8rAbZMgwacvnXswZ2VlBNc1goh/tables/tblWb9n3b9nNVBJb/records/batch_delete"
	batchAddURL    = "https://open.larkoffice.com/open-apis/bitable/v1/apps/Q8rAbZMgwacvnXswZ2VlBNc1goh/tables/tblWb9n3b9nNVBJb/records/batch_create"
	batchUpdateURL = "https://open.larkoffice.com/open-apis/bitable/v1/apps/Q8rAbZMgwacvnXswZ2VlBNc1goh/tables/tblWb9n3b9nNVBJb/records/batch_update"
	uploadPicURL   = "https://fsopen.bytedance.net/open-apis/drive/v1/medias/upload_all"
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
func ReadSheetRecords() ([]*SheetRecord, error) {
	token, err := GetTalentAccessToken()
	if err != nil {
		fmt.Printf("Error getting access token: %v\n", err)
		return nil, err
	}

	var records []*SheetRecord
	pageToken := ""

	for {
		// 构建请求URL，包含page_token
		url := readSheetURL
		if pageToken != "" {
			url = fmt.Sprintf("%s&page_token=%s", readSheetURL, pageToken)
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
func BatchDeleteSheetRecords(recordIDs []string) error {
	token, err := GetTalentAccessToken()
	if err != nil {
		return fmt.Errorf("获取访问令牌失败: %w", err)
	}

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

	respBody, err := SendHTTPRequest("POST", batchDeleteURL, reqBody, headers)
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
func BatchAddSheetRecords(records []map[string]interface{}) error {
	token, err := GetTalentAccessToken()
	if err != nil {
		return fmt.Errorf("获取访问令牌失败: %w", err)
	}

	// 构造请求体
	m := []map[string]interface{}{}
	for _, record := range records {
		m = append(m, map[string]interface{}{
			"fields": record,
		})
	}
	reqBody, err := json.Marshal(map[string]interface{}{
		"records": m,
	})
	if err != nil {
		return fmt.Errorf("构造请求体失败: %w", err)
	}

	headers := map[string]string{
		"Authorization": token,
		"Content-Type":  "application/json; charset=utf-8",
	}

	respBody, err := SendHTTPRequest("POST", batchAddURL, reqBody, headers)
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
		return fmt.Errorf("批量添加记录失败，错误码: %d, 错误信息: %s", sheetResp.Code, sheetResp.Msg)
	}

	return nil
}

// BatchUpdateSheetRecord 批量更新多维表格记录
func BatchUpdateSheetRecords(records []*SheetRecord) error {
	token, err := GetTalentAccessToken()
	if err != nil {
		return fmt.Errorf("获取访问令牌失败: %w", err)
	}

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

	respBody, err := SendHTTPRequest("POST", batchUpdateURL, reqBody, headers)
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
	fields := map[string]string{
		"file_name":   fileName,
		"parent_type": "bitable_image",
		"parent_node": "Q8rAbZMgwacvnXswZ2VlBNc1goh",
		"size":        fmt.Sprintf("%d", len(data)),
	}

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
