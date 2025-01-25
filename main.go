package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const TokenEndpoint = "https://oauth2.googleapis.com/token"
const TargetCalendarId = "family13253019517568372730@group.calendar.google.com"
const bucketName = "calendar-notifier"
const objectName = "nextsynctoken.txt"

// AccessTokenResponse GoogleのトークンAPIレスポンス
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

type BucketManager struct {
	client *storage.Client
}

func NewBucketManager() (*BucketManager, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &BucketManager{
		client: client,
	}, nil
}

func (b BucketManager) WriteTextObject(bucketName, objectName, content string) error {
	ctx := context.Background()
	bucket := b.client.Bucket(bucketName)

	w := bucket.Object(objectName).NewWriter(ctx)
	_, err := w.Write([]byte(content))
	if err != nil {
		w.Close()
		return err
	}

	// このチェックを忘れずに！
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}

func (b BucketManager) ReadTextObject(bucketName, objectName string) (string, error) {
	ctx := context.Background()
	bucket := b.client.Bucket(bucketName)
	rc, err := bucket.Object(objectName).NewReader(ctx)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	slurp, err := ioutil.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(slurp), nil
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

type CalendarNotifier struct {
	bucketManager *BucketManager
	srv		   *calendar.Service
	discordMessanger *DiscordMessanger
}

func NewCalendarNotifier() (*CalendarNotifier, error) {
	bucketManager, err := NewBucketManager()
	if err != nil {
		return nil, err
	}
	discordMessanger, err := NewDiscordMessanger()
	if err != nil {
		return nil, err
	}

	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	srv, err := calendar.NewService(ctx, option.WithTokenSource(
		oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: accessToken,
			})))
	if err != nil {
		return nil, err
	}
	return &CalendarNotifier{
		bucketManager: bucketManager,
		srv: srv,
		discordMessanger: discordMessanger,
	}, nil
}

func (c CalendarNotifier) GetEventsToNotify() ([]*calendar.Event, error) {
	allEvents := []*calendar.Event{}

	var pageToken *string = nil
	var syncToken *string = nil

	text, err := c.bucketManager.ReadTextObject(bucketName, objectName)
	if err == nil {
		log.Println("NextSyncToken loaded.")
		syncToken = &text
	}

	for {
		eventCall := c.srv.Events.List(TargetCalendarId).SingleEvents(true).EventTypes("default").MaxResults(10)
		if pageToken != nil {
			eventCall = eventCall.PageToken(*pageToken)
		}
		if syncToken != nil {
			eventCall = eventCall.SyncToken(*syncToken)
		}

		events, err := eventCall.Do()
		if err != nil {
			return nil, err
		}

		allEvents = append(allEvents, events.Items...)
		if events.NextPageToken == "" {
			err = c.bucketManager.WriteTextObject(bucketName, objectName, events.NextSyncToken)
			if err != nil {
				return nil, err
			}
			log.Println("NextSyncToken saved.")
			break
		}

		pageToken = &events.NextPageToken
	}
	return allEvents, nil
}

func (c CalendarNotifier) NotifyEvent(event calendar.Event) error {
	if event.Status != "confirmed" {
		return nil
	}

	message := fmt.Sprintf("以下の予定が追加されました\n\nイベント: %s\n開始: %s\n終了: %s\n場所: %s\n説明: %s", event.Summary, event.Start.DateTime, event.End.DateTime, event.Location, event.Description)
	log.Println("message: ", message)
	err := c.discordMessanger.SendMessage(message)
	return err
}


type DiscordMessanger struct {
	webhookURL string
}

func NewDiscordMessanger() (*DiscordMessanger, error) {
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		return nil, fmt.Errorf("環境変数 DISCORD_WEBHOOK_URL が設定されていません")
	}
	return &DiscordMessanger{
		webhookURL: webhookURL,
	}, nil
}

func (d DiscordMessanger) SendMessage(pushMessage string) error {
	payload := map[string]string{
		"content": pushMessage,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", d.webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	calendarNotifier, err := NewCalendarNotifier()
	if err != nil {
		log.Fatal(err)
	}
	server := http.Server{
		Addr:    ":8080",
		Handler: nil,
	}
	http.HandleFunc("/calendar/watch", func (w http.ResponseWriter, r *http.Request) {
		var state, ok = r.Header["X-Goog-Resource-State"]
		if !ok || state[0] != "exists" {
			return
		}

		eventsToNotify, err := calendarNotifier.GetEventsToNotify()
		if err != nil {
			log.Fatal(err)
		}
		for _, event := range eventsToNotify {
			err := calendarNotifier.NotifyEvent(*event)
			if err != nil {
				log.Fatal(err)
			}
		}
	})
	server.ListenAndServe()
}
