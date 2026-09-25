package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testRoundTrip func(*http.Request) (*http.Response, error)

func (fn testRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestRefreshWeReadTokenRotatesCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/login" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("baseapi") != "30" {
			t.Errorf("baseapi = %q", r.Header.Get("baseapi"))
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]interface{}{
			"deviceId": "device123", "refreshToken": "old-refresh", "timestamp": float64(1000),
			"random": float64(7), "deviceType": float64(3),
			"signature": "027efaf4152940de2a1c362e0ab7f44fd35a8a5689dc574bb30a9aa07951a8f6",
		} {
			if body[key] != want {
				t.Errorf("%s = %v, want %v", key, body[key], want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vid":12345678,"accessToken":"new-access","refreshToken":"new-refresh"}`))
	}))
	defer server.Close()

	got, err := refreshWeReadToken(context.Background(), server.Client(), server.URL, weReadCredentials{
		VID: "12345678", DeviceID: "device123", RefreshToken: "old-refresh",
	}, 1000, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.VID != "12345678" || got.DeviceID != "device123" || got.AccessToken != "new-access" || got.RefreshToken != "new-refresh" {
		t.Errorf("unexpected credentials after rotation: vid=%q device=%q accessChanged=%t refreshChanged=%t", got.VID, got.DeviceID, got.AccessToken == "new-access", got.RefreshToken == "new-refresh")
	}
}

func TestRefreshWeReadTokenRejectsDifferentAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vid":123,"accessToken":"other-access","refreshToken":"other-refresh"}`))
	}))
	defer server.Close()
	_, err := refreshWeReadToken(context.Background(), server.Client(), server.URL, weReadCredentials{
		VID: "12345678", DeviceID: "device123", RefreshToken: "old-refresh",
	}, 1000, 7)
	if err == nil {
		t.Fatal("expected account mismatch error")
	}
}

func TestSaveWeReadCredentialsArePrivateAndReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "credentials.json")
	want := weReadCredentials{VID: "12345678", DeviceID: "device123", RefreshToken: "rotated-refresh"}
	if err := saveWeReadCredentials(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("credential mode = %o, want 600", info.Mode().Perm())
	}
	got, err := loadWeReadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.VID != want.VID || got.DeviceID != want.DeviceID || got.RefreshToken != want.RefreshToken {
		t.Error("saved credentials did not round trip")
	}
}

func TestRequestWeReadQRUsesTicketAndReturnsConfirmURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wxticket":
			if r.URL.Query().Get("nonceStr") != "weread" {
				t.Errorf("nonceStr = %q", r.URL.Query().Get("nonceStr"))
			}
			_, _ = w.Write([]byte(`{"signature":"ticket-proof","timeStamp":1000}`))
		case "/connect/sdk/qrconnect":
			if r.URL.Query().Get("signature") != "ticket-proof" || r.URL.Query().Get("timestamp") != "1000" {
				t.Errorf("QR ticket was not forwarded")
			}
			_, _ = w.Write([]byte(`{"errcode":0,"uuid":"qr-uuid"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	got, err := requestWeReadQR(context.Background(), server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.UUID != "qr-uuid" || got.ConfirmURL != "https://open.weixin.qq.com/connect/confirm?uuid=qr-uuid" {
		t.Errorf("unexpected QR request result: %+v", got)
	}
}

func TestPollWeReadCodeReturnsConfirmedCode(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("uuid") != "qr-uuid" {
			t.Errorf("uuid = %q", r.URL.Query().Get("uuid"))
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"wx_errcode":408}`))
		} else {
			_, _ = w.Write([]byte(`{"wx_errcode":405,"wx_code":"confirmed-code"}`))
		}
	}))
	defer server.Close()
	got, err := pollWeReadCode(context.Background(), server.Client(), server.URL, "qr-uuid", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "confirmed-code" || calls != 2 {
		t.Errorf("code obtained=%t, calls=%d", got == "confirmed-code", calls)
	}
}

func TestExchangeWeReadCodeUsesFreshSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]interface{}{
			"code": "confirmed-code", "deviceId": "device123", "installId": "install123",
			"timestamp": float64(1000), "random": float64(7), "deviceType": float64(3),
			"signature": "027efaf4152940de2a1c362e0ab7f44fd35a8a5689dc574bb30a9aa07951a8f6",
		} {
			if body[key] != want {
				t.Errorf("%s = %v, want %v", key, body[key], want)
			}
		}
		_, _ = w.Write([]byte(`{"vid":12345678,"accessToken":"new-access","refreshToken":"new-refresh"}`))
	}))
	defer server.Close()
	got, err := exchangeWeReadCode(context.Background(), server.Client(), server.URL, "confirmed-code", "device123", "install123", 1000, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.VID != "12345678" || got.AccessToken != "new-access" || got.RefreshToken != "new-refresh" || got.DeviceID != "device123" {
		t.Error("QR exchange did not return the expected credentials")
	}
}

func TestGetAccessTokenRefreshesAndPersistsInsteadOfUsingMock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := saveWeReadCredentials(path, weReadCredentials{VID: "12345678", DeviceID: "device123", RefreshToken: "old-refresh"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEREAD_AUTH_FILE", path)
	previousClient := weReadHTTPClient
	previousToken, previousExpiry := cachedAccessToken, tokenExpiryTime
	weReadHTTPClient = &http.Client{Transport: testRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"vid":12345678,"accessToken":"fresh-access","refreshToken":"rotated-refresh"}`)),
		}, nil
	})}
	cachedAccessToken, tokenExpiryTime = "", time.Time{}
	t.Cleanup(func() {
		weReadHTTPClient = previousClient
		cachedAccessToken, tokenExpiryTime = previousToken, previousExpiry
	})
	got, err := GetAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if got != "fresh-access" {
		t.Error("GetAccessToken returned a token other than the newly minted one")
	}
	saved, err := loadWeReadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshToken != "rotated-refresh" {
		t.Error("rotated refresh token was not saved")
	}
}

func TestLoginWithQRPersistsAccountForLaterRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wxticket":
			_, _ = w.Write([]byte(`{"signature":"ticket-proof","timeStamp":1000}`))
		case "/connect/sdk/qrconnect":
			_, _ = w.Write([]byte(`{"errcode":0,"uuid":"qr-uuid"}`))
		case "/connect/l/qrconnect":
			_, _ = w.Write([]byte(`{"wx_errcode":405,"wx_code":"confirmed-code"}`))
		case "/login":
			_, _ = w.Write([]byte(`{"vid":12345678,"accessToken":"new-access","refreshToken":"new-refresh"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "credentials.json")
	qrShown := false
	vid, err := loginWithQR(context.Background(), server.Client(), server.URL, server.URL, server.URL, path, 0, func(qr weReadQR) error {
		qrShown = qr.UUID == "qr-uuid"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !qrShown || vid != "12345678" {
		t.Errorf("QR was shown=%t, vid=%q", qrShown, vid)
	}
	saved, err := loadWeReadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshToken != "new-refresh" || saved.DeviceID == "" {
		t.Error("QR login did not persist usable refresh credentials")
	}
}

func TestMissingCredentialsTriggerLoginAndContinue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.local.json")
	t.Setenv("WEREAD_AUTH_FILE", path)
	previousToken, previousExpiry := cachedAccessToken, tokenExpiryTime
	cachedAccessToken, tokenExpiryTime = "", time.Time{}
	t.Cleanup(func() { cachedAccessToken, tokenExpiryTime = previousToken, previousExpiry })
	loginCount := 0
	got, err := getWeReadAccessTokenWithLogin(func() error {
		loginCount++
		if err := saveWeReadCredentials(path, weReadCredentials{VID: "123", DeviceID: "device", RefreshToken: "refresh"}); err != nil {
			return err
		}
		weReadAuthMu.Lock()
		cachedAccessToken, tokenExpiryTime = "scanned-token", time.Now().Add(time.Minute)
		weReadAuthMu.Unlock()
		return nil
	})
	if err != nil || got != "scanned-token" || loginCount != 1 {
		t.Fatalf("automatic login failed: token_ok=%t, logins=%d, err=%v", got == "scanned-token", loginCount, err)
	}
}

func TestExpiredAPIResponseTriggersLoginAndRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.local.json")
	t.Setenv("WEREAD_AUTH_FILE", path)
	if err := saveWeReadCredentials(path, weReadCredentials{VID: "123", DeviceID: "device", RefreshToken: "refresh"}); err != nil {
		t.Fatal(err)
	}
	previousToken, previousExpiry := cachedAccessToken, tokenExpiryTime
	cachedAccessToken, tokenExpiryTime = "old-token", time.Now().Add(time.Minute)
	t.Cleanup(func() { cachedAccessToken, tokenExpiryTime = previousToken, previousExpiry })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("vid") != "123" {
			t.Error("request did not use the configured WeRead VID")
		}
		if requests == 1 {
			if r.Header.Get("accessToken") != "old-token" {
				t.Error("first request did not use the cached token")
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errcode":-2012,"errmsg":"登录超时"}`))
			return
		}
		if r.Header.Get("accessToken") != "new-token" {
			t.Error("retry did not use the scanned token")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	logins := 0
	body, err := sendAuthenticatedWeReadGETWithLogin(server.URL, func() error {
		logins++
		weReadAuthMu.Lock()
		cachedAccessToken, tokenExpiryTime = "new-token", time.Now().Add(time.Minute)
		weReadAuthMu.Unlock()
		return nil
	})
	if err != nil || string(body) != `{"ok":true}` || requests != 2 || logins != 1 {
		t.Fatalf("authentication retry failed: requests=%d logins=%d err=%v", requests, logins, err)
	}
}

func TestRejectedRefreshTriggersLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.local.json")
	t.Setenv("WEREAD_AUTH_FILE", path)
	if err := saveWeReadCredentials(path, weReadCredentials{VID: "123", DeviceID: "device", RefreshToken: "expired"}); err != nil {
		t.Fatal(err)
	}
	previousClient := weReadHTTPClient
	previousToken, previousExpiry := cachedAccessToken, tokenExpiryTime
	weReadHTTPClient = &http.Client{Transport: testRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"errcode":-2041,"errmsg":"expired"}`)),
		}, nil
	})}
	cachedAccessToken, tokenExpiryTime = "", time.Time{}
	t.Cleanup(func() {
		weReadHTTPClient = previousClient
		cachedAccessToken, tokenExpiryTime = previousToken, previousExpiry
	})
	logins := 0
	got, err := getWeReadAccessTokenWithLogin(func() error {
		logins++
		weReadAuthMu.Lock()
		cachedAccessToken, tokenExpiryTime = "scanned-token", time.Now().Add(time.Minute)
		weReadAuthMu.Unlock()
		return nil
	})
	if err != nil || got != "scanned-token" || logins != 1 {
		t.Fatalf("refresh fallback failed: token_ok=%t logins=%d err=%v", got == "scanned-token", logins, err)
	}
}
