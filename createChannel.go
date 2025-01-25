package createChannel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/satori/go.uuid"
)

const targetCalendarId = "family13253019517568372730@group.calendar.google.com"
const TokenEndpoint = "https://oauth2.googleapis.com/token"
const watchAddress = "https://calendar-notifier-547061469071.asia-northeast1.run.app/calendar/watch"

// AccessTokenResponse GoogleのトークンAPIレスポンス
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// GetAccessToken リフレッシュトークンを使ってアクセストークンを取得
func GetAccessToken() (string, error) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	refreshToken := os.Getenv("GOOGLE_REFRESH_TOKEN")

	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return "", fmt.Errorf("環境変数 GOOGLE_CLIENT_ID または GOOGLE_CLIENT_SECRET が設定されていません")
	}

	payload := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"refresh_token": refreshToken,
		"grant_type":    "refresh_token",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", TokenEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("リクエスト作成エラー: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTPリクエストエラー: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("アクセストークン取得失敗: ステータスコード %d, レスポンス %s", resp.StatusCode, string(responseBody))
	}

	var tokenResp AccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("レスポンスデコードエラー: %v", err)
	}

	return tokenResp.AccessToken, nil
}

func main() {
	type m map[string]interface{}
	params := m{
		"id":      uuid.NewV4().String(),
		"type":    "web_hook",
		"address": watchAddress,
	}
	payload, _ := json.MarshalIndent(params, "", "    ")

	accessToken, err := GetAccessToken()
	req, _ := http.NewRequest(
		"POST",
		fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/watch", url.QueryEscape(targetCalendarId)),
		bytes.NewReader(payload),
	)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("calendar api request error.: %s", err.Error())
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("calendar api request error.: %s", err.Error())
	} else if resp.StatusCode != http.StatusOK {
		log.Fatalf("calendar api request error.: %s", err.Error())
	}
	fmt.Println(string(body))
}
