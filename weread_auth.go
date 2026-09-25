package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// This device profile uses WeRead's e-ink login flow. It has its own device ID
// and refresh token, separate from the phone session captured in Proxyman.
const weReadBaseURL = "https://i.weread.qq.com"

const weReadUserAgent = "WeRead/2.1.2 WRBrand/Onyx wr_eink Dalvik/2.1.0 (Linux; U; Android 11; BOOX Build/onyx)"
const weChatBaseURL = "https://open.weixin.qq.com"
const weChatLongBaseURL = "https://long.open.weixin.qq.com"

var weReadHTTPClient = &http.Client{Timeout: 30 * time.Second}
var weReadAuthMu sync.Mutex
var weReadRefreshMu sync.Mutex

type weReadCredentials struct {
	VID          string `json:"vid"`
	DeviceID     string `json:"deviceId"`
	RefreshToken string `json:"refreshToken"`
	AccessToken  string `json:"-"`
}

type weReadQR struct {
	UUID       string
	ConfirmURL string
}

func weReadCredentialPath() (string, error) {
	if path := os.Getenv("WEREAD_AUTH_FILE"); path != "" {
		return path, nil
	}
	return "auth.local.json", nil
}

func validateWeReadCredentials(c weReadCredentials) error {
	if c.VID == "" || c.DeviceID == "" || c.RefreshToken == "" {
		return errors.New("微信读书凭证缺少 vid、deviceId 或 refreshToken")
	}
	return nil
}

func loadWeReadCredentials(path string) (weReadCredentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return weReadCredentials{}, fmt.Errorf("未找到微信读书凭证")
		}
		return weReadCredentials{}, fmt.Errorf("读取微信读书凭证失败: %w", err)
	}
	var credentials weReadCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return weReadCredentials{}, fmt.Errorf("微信读书凭证格式无效: %w", err)
	}
	if err := validateWeReadCredentials(credentials); err != nil {
		return weReadCredentials{}, err
	}
	return credentials, nil
}

func saveWeReadCredentials(path string, credentials weReadCredentials) error {
	if err := validateWeReadCredentials(credentials); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建凭证目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".credentials-*")
	if err != nil {
		return fmt.Errorf("创建临时凭证文件失败: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	if err := encoder.Encode(credentials); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("保存微信读书凭证失败: %w", err)
	}
	return nil
}

func weReadVersionHeaders() http.Header {
	headers := make(http.Header)
	headers.Set("baseapi", "30")
	headers.Set("appver", "2.1.2.10245900")
	headers.Set("basever", "2.1.2.10245900")
	headers.Set("osver", "11")
	headers.Set("channelId", "900")
	headers.Set("User-Agent", weReadUserAgent)
	return headers
}

func weReadSignature(timestamp int64, deviceID string, random int) string {
	hash := sha256.Sum256([]byte(strconv.FormatInt(timestamp, 10) + deviceID + strconv.Itoa(random)))
	return hex.EncodeToString(hash[:])
}

func weReadRandom(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func parseWeReadVID(raw json.RawMessage, fallback string) (string, error) {
	if len(raw) == 0 {
		return fallback, nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", err
	}
	return number.String(), nil
}

type weReadLoginResponse struct {
	VID          json.RawMessage `json:"vid"`
	AccessToken  string          `json:"accessToken"`
	RefreshToken string          `json:"refreshToken"`
	ErrCode      int             `json:"errCode"`
	ErrMsg       string          `json:"errMsg"`
	Errcode      int             `json:"errcode"`
	Errmsg       string          `json:"errmsg"`
}

func postWeReadLogin(ctx context.Context, client *http.Client, baseURL string, body any) (weReadLoginResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return weReadLoginResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/login", strings.NewReader(string(encoded)))
	if err != nil {
		return weReadLoginResponse{}, err
	}
	req.Header = weReadVersionHeaders()
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		return weReadLoginResponse{}, fmt.Errorf("微信读书登录请求失败: %w", err)
	}
	defer resp.Body.Close()
	var result weReadLoginResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return weReadLoginResponse{}, fmt.Errorf("微信读书登录响应无效: %w", err)
	}
	if resp.StatusCode != http.StatusOK || result.AccessToken == "" {
		code, message := result.ErrCode, result.ErrMsg
		if code == 0 {
			code, message = result.Errcode, result.Errmsg
		}
		return weReadLoginResponse{}, fmt.Errorf("微信读书登录被拒绝: HTTP %d, errcode %d, %s", resp.StatusCode, code, message)
	}
	return result, nil
}

func refreshWeReadToken(ctx context.Context, client *http.Client, baseURL string, credentials weReadCredentials, timestamp int64, random int) (weReadCredentials, error) {
	if err := validateWeReadCredentials(credentials); err != nil {
		return weReadCredentials{}, err
	}
	body := map[string]any{
		"deviceId":     credentials.DeviceID,
		"deviceName":   "BOOX",
		"inBackground": 0,
		"kickType":     1,
		"random":       random,
		"refCgi":       "",
		"refreshToken": credentials.RefreshToken,
		"signature":    weReadSignature(timestamp, credentials.DeviceID, random),
		"timestamp":    timestamp,
		"trackId":      "",
		"deviceType":   3,
	}
	result, err := postWeReadLogin(ctx, client, baseURL, body)
	if err != nil {
		return weReadCredentials{}, err
	}
	vid, err := parseWeReadVID(result.VID, credentials.VID)
	if err != nil || vid != credentials.VID {
		return weReadCredentials{}, errors.New("微信读书刷新返回了不同的账号")
	}
	credentials.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		credentials.RefreshToken = result.RefreshToken
	}
	return credentials, nil
}

func requestWeReadQR(ctx context.Context, client *http.Client, baseURL, wxBaseURL string) (weReadQR, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/wxticket?nonceStr=weread", nil)
	if err != nil {
		return weReadQR{}, err
	}
	req.Header = weReadVersionHeaders()
	resp, err := client.Do(req)
	if err != nil {
		return weReadQR{}, fmt.Errorf("获取微信读书扫码票据失败: %w", err)
	}
	var ticket struct {
		Signature string `json:"signature"`
		TimeStamp int64  `json:"timeStamp"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&ticket)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || decodeErr != nil || ticket.Signature == "" || ticket.TimeStamp == 0 {
		return weReadQR{}, fmt.Errorf("微信读书扫码票据无效: HTTP %d", resp.StatusCode)
	}
	query := url.Values{
		"appid":     {"wxab9b71ad2b90ff34"},
		"noncestr":  {"weread"},
		"timestamp": {strconv.FormatInt(ticket.TimeStamp, 10)},
		"scope":     {"snsapi_userinfo,snsapi_timeline,snsapi_friend"},
		"signature": {ticket.Signature},
	}
	qrURL := strings.TrimRight(wxBaseURL, "/") + "/connect/sdk/qrconnect?" + query.Encode()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, qrURL, nil)
	if err != nil {
		return weReadQR{}, err
	}
	req.Header.Set("User-Agent", weReadUserAgent)
	resp, err = client.Do(req)
	if err != nil {
		return weReadQR{}, fmt.Errorf("获取微信扫码地址失败: %w", err)
	}
	var qr struct {
		ErrCode int    `json:"errcode"`
		UUID    string `json:"uuid"`
	}
	decodeErr = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&qr)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || decodeErr != nil || qr.ErrCode != 0 || qr.UUID == "" {
		return weReadQR{}, fmt.Errorf("微信扫码地址无效: HTTP %d, errcode %d", resp.StatusCode, qr.ErrCode)
	}
	return weReadQR{UUID: qr.UUID, ConfirmURL: weChatBaseURL + "/connect/confirm?uuid=" + url.QueryEscape(qr.UUID)}, nil
}

func pollWeReadCode(ctx context.Context, client *http.Client, longBaseURL, uuid string, delay time.Duration) (string, error) {
	last := ""
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("微信扫码等待结束: %w", err)
		}
		query := url.Values{"f": {"json"}, "uuid": {uuid}}
		if last != "" {
			query.Set("last", last)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(longBaseURL, "/")+"/connect/l/qrconnect?"+query.Encode(), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("等待微信扫码确认失败: %w", err)
		}
		var status struct {
			Code      int    `json:"wx_errcode"`
			CodeToken string `json:"wx_code"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&status)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || decodeErr != nil {
			return "", fmt.Errorf("微信扫码状态无效: HTTP %d", resp.StatusCode)
		}
		switch status.Code {
		case 405:
			if status.CodeToken == "" {
				return "", errors.New("微信扫码已确认但未返回登录码")
			}
			return status.CodeToken, nil
		case 408, 404:
			last = strconv.Itoa(status.Code)
		case 402:
			return "", errors.New("微信扫码二维码已过期，请重新运行登录")
		case 403:
			return "", errors.New("微信扫码已拒绝")
		default:
			return "", fmt.Errorf("未知微信扫码状态: %d", status.Code)
		}
		if delay > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}
	}
}

func exchangeWeReadCode(ctx context.Context, client *http.Client, baseURL, code, deviceID, installID string, timestamp int64, random int) (weReadCredentials, error) {
	body := map[string]any{
		"appFirstInstall": 1,
		"code":            code,
		"deviceId":        deviceID,
		"deviceName":      "BOOX",
		"installId":       installID,
		"isAutoLogout":    0,
		"isFromQrcode":    1,
		"random":          random,
		"signature":       weReadSignature(timestamp, deviceID, random),
		"timestamp":       timestamp,
		"trackId":         "",
		"deviceType":      3,
	}
	result, err := postWeReadLogin(ctx, client, baseURL, body)
	if err != nil {
		return weReadCredentials{}, err
	}
	vid, err := parseWeReadVID(result.VID, "")
	if err != nil || vid == "" || result.RefreshToken == "" {
		return weReadCredentials{}, errors.New("微信读书扫码登录响应缺少账号或刷新凭证")
	}
	return weReadCredentials{VID: vid, DeviceID: deviceID, RefreshToken: result.RefreshToken, AccessToken: result.AccessToken}, nil
}

func weReadRandomDigits(count int) (string, error) {
	var digits strings.Builder
	for i := 0; i < count; i++ {
		digit, err := weReadRandom(10)
		if err != nil {
			return "", err
		}
		digits.WriteByte(byte('0' + digit))
	}
	return digits.String(), nil
}

func loginWithQR(ctx context.Context, client *http.Client, baseURL, wxBaseURL, longBaseURL, credentialPath string, pollDelay time.Duration, onQR func(weReadQR) error) (string, error) {
	qr, err := requestWeReadQR(ctx, client, baseURL, wxBaseURL)
	if err != nil {
		return "", err
	}
	if err := onQR(qr); err != nil {
		return "", err
	}
	code, err := pollWeReadCode(ctx, client, longBaseURL, qr.UUID, pollDelay)
	if err != nil {
		return "", err
	}
	deviceSuffix, err := weReadRandomDigits(19)
	if err != nil {
		return "", err
	}
	installSuffix, err := weReadRandomDigits(26)
	if err != nil {
		return "", err
	}
	random, err := weReadRandom(1000)
	if err != nil {
		return "", err
	}
	credentials, err := exchangeWeReadCode(ctx, client, baseURL, code, "eink334691225"+deviceSuffix, "eink31"+installSuffix, time.Now().UnixMilli(), random)
	if err != nil {
		return "", err
	}
	if existing, err := loadWeReadCredentials(credentialPath); err == nil && existing.VID != credentials.VID {
		return "", fmt.Errorf("扫码账号与现有微信读书账号不同，未覆盖凭证")
	}
	if err := saveWeReadCredentials(credentialPath, credentials); err != nil {
		return "", err
	}
	weReadAuthMu.Lock()
	cachedAccessToken = credentials.AccessToken
	tokenExpiryTime = time.Now().Add(3 * time.Minute)
	weReadAuthMu.Unlock()
	return credentials.VID, nil
}

func runWeReadLogin() error {
	path, err := weReadCredentialPath()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	qrPath := ""
	defer func() {
		if qrPath != "" {
			os.Remove(qrPath)
		}
	}()
	vid, err := loginWithQR(ctx, weReadHTTPClient, weReadBaseURL, weChatBaseURL, weChatLongBaseURL, path, time.Second, func(qr weReadQR) error {
		image, err := qrcode.New(qr.ConfirmURL, qrcode.Medium)
		if err != nil {
			return err
		}
		file, err := os.CreateTemp("", "weread-login-*.png")
		if err != nil {
			return err
		}
		qrPath = file.Name()
		png, err := image.PNG(320)
		if err != nil {
			file.Close()
			return err
		}
		if _, err := file.Write(png); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		fmt.Printf("请用微信扫描二维码，图片路径：%s\n", qrPath)
		fmt.Println(image.ToSmallString(false))
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("微信读书登录完成，账号 vid=%s；刷新凭证已保存到 %s\n", vid, path)
	return nil
}

func getWeReadAccessToken() (string, error) {
	return getWeReadAccessTokenWithLogin(runWeReadLogin)
}

func getWeReadAccessTokenWithLogin(login func() error) (string, error) {
	weReadAuthMu.Lock()
	if time.Now().Before(tokenExpiryTime) && cachedAccessToken != "" {
		token := cachedAccessToken
		weReadAuthMu.Unlock()
		return token, nil
	}
	weReadAuthMu.Unlock()
	weReadRefreshMu.Lock()
	defer weReadRefreshMu.Unlock()
	weReadAuthMu.Lock()
	if time.Now().Before(tokenExpiryTime) && cachedAccessToken != "" {
		token := cachedAccessToken
		weReadAuthMu.Unlock()
		return token, nil
	}
	weReadAuthMu.Unlock()
	path, err := weReadCredentialPath()
	if err != nil {
		return "", err
	}
	credentials, err := loadWeReadCredentials(path)
	if err != nil {
		return loginAndReturnToken(login, err)
	}
	random, err := weReadRandom(1000)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	updated, err := refreshWeReadToken(ctx, weReadHTTPClient, weReadBaseURL, credentials, time.Now().UnixMilli(), random+1)
	if err != nil {
		return loginAndReturnToken(login, err)
	}
	if err := saveWeReadCredentials(path, updated); err != nil {
		return "", err
	}
	weReadAuthMu.Lock()
	cachedAccessToken = updated.AccessToken
	tokenExpiryTime = time.Now().Add(3 * time.Minute)
	weReadAuthMu.Unlock()
	return updated.AccessToken, nil
}

func loginAndReturnToken(login func() error, cause error) (string, error) {
	fmt.Fprintln(os.Stderr, "微信读书登录已失效，请扫码重新登录。")
	if err := login(); err != nil {
		return "", fmt.Errorf("微信读书登录失败: %w（原错误: %v）", err, cause)
	}
	weReadAuthMu.Lock()
	token := cachedAccessToken
	weReadAuthMu.Unlock()
	if token == "" {
		return "", errors.New("扫码登录完成但未取得 accessToken")
	}
	return token, nil
}

func weReadAuthenticationExpired(body []byte, requestErr error) bool {
	if isHTTPStatus(requestErr, http.StatusUnauthorized) {
		return true
	}
	var response struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false
	}
	return response.ErrCode == -2012 || response.ErrCode == -2041 || strings.Contains(response.ErrMsg, "登录超时")
}

func sendAuthenticatedWeReadGET(path string) ([]byte, error) {
	return sendAuthenticatedWeReadGETWithLogin(path, runWeReadLogin)
}

func sendAuthenticatedWeReadGETWithLogin(path string, login func() error) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		accessToken, err := getWeReadAccessTokenWithLogin(login)
		if err != nil {
			return nil, err
		}
		credentialPath, err := weReadCredentialPath()
		if err != nil {
			return nil, err
		}
		credentials, err := loadWeReadCredentials(credentialPath)
		if err != nil {
			return nil, err
		}
		body, requestErr := SendHTTPRequest(http.MethodGet, path, nil, map[string]string{
			"accessToken": accessToken,
			"vid":         credentials.VID,
		})
		if !weReadAuthenticationExpired(body, requestErr) {
			return body, requestErr
		}
		if attempt == 1 {
			return nil, errors.New("微信读书重新登录后仍返回登录失效")
		}
		weReadRefreshMu.Lock()
		weReadAuthMu.Lock()
		alreadyReplaced := cachedAccessToken != accessToken && cachedAccessToken != ""
		if !alreadyReplaced {
			cachedAccessToken = ""
			tokenExpiryTime = time.Time{}
		}
		weReadAuthMu.Unlock()
		if !alreadyReplaced {
			fmt.Fprintln(os.Stderr, "微信读书 token 已失效，请扫码重新登录。")
			if err := login(); err != nil {
				weReadRefreshMu.Unlock()
				return nil, fmt.Errorf("微信读书扫码登录失败: %w", err)
			}
		}
		weReadRefreshMu.Unlock()
	}
	return nil, errors.New("微信读书请求在重新登录后仍未通过认证")
}
