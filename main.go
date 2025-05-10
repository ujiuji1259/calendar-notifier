package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"net/http"
	"bytes"
	"io/ioutil"

	"cloud.google.com/go/datastore"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

const TokenEndpoint = "https://oauth2.googleapis.com/token"
const TargetCalendarId = "family13253019517568372730@group.calendar.google.com"

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

func NewCalendarService() (*calendar.Service, error) {
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
	return srv, nil
}

type SyncToken struct {
	Value string
}

type SyncTokenRepository interface {
	Save(ctx context.Context, token string) error
	Get(ctx context.Context) (string, error)
}

type DatastoreSyncTokenRepository struct {
	client *datastore.Client
}

func NewDatastoreSyncTokenRepository(ctx context.Context) (*DatastoreSyncTokenRepository, error) {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_PROJECT environment variable is not set")
	}

	databaseID := os.Getenv("GOOGLE_CLOUD_DATABASE")
	if databaseID == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_DATABASE environment variable is not set")
	}

	client, err := datastore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to create datastore client: %w", err)
	}

	return &DatastoreSyncTokenRepository{
		client: client,
	}, nil
}

func (r *DatastoreSyncTokenRepository) Save(ctx context.Context, token string) error {
	k := datastore.NameKey("Token", "SyncToken", nil)
	e := &SyncToken{
		Value: token,
	}

	if _, err := r.client.Put(ctx, k, e); err != nil {
		return fmt.Errorf("failed to save sync token: %w", err)
	}
	return nil
}

func (r *DatastoreSyncTokenRepository) Get(ctx context.Context) (string, error) {
	k := datastore.NameKey("Token", "SyncToken", nil)
	e := &SyncToken{}

	if err := r.client.Get(ctx, k, e); err != nil {
		if err == datastore.ErrNoSuchEntity {
			return "", nil
		}
		return "", fmt.Errorf("failed to get sync token: %w", err)
	}
	return e.Value, nil
}


type CalendarNotifier struct {
	repository *DatastoreSyncTokenRepository
	srv		   *calendar.Service
	notifier   *DiscordEventNotifier
}

func NewCalendarNotifier() (*CalendarNotifier, error) {
	ctx := context.Background()
	repository, err := NewDatastoreSyncTokenRepository(ctx)
	if err != nil {
		return nil, err
	}
	srv, err := NewCalendarService()
	if err != nil {
		return nil, err
	}
	notifier, err := NewDiscordEventNotifier()
	if err != nil {
		return nil, err
	}

	return &CalendarNotifier{
		repository: repository,
		srv: srv,
		notifier: notifier,
	}, nil
}

func (c CalendarNotifier) GetEventsToNotify() ([]*calendar.Event, error) {
	allEvents := []*calendar.Event{}

	var pageToken *string = nil
	var syncToken *string = nil

	ctx := context.Background()
	text, err := c.repository.Get(ctx)
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
			err = c.repository.Save(ctx, events.NextSyncToken)
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

func (c CalendarNotifier) NotifyEvents() error {
	events, err := c.GetEventsToNotify()
	if err != nil {
		return err
	}
	eventsToNotify := []*calendar.Event{}

	for _, event := range events {
		if event.Status != "confirmed" {
			continue
		}
		eventsToNotify = append(eventsToNotify, event)
	}

	return c.notifier.SendEvents(eventsToNotify)
}

type EventNotifier interface {
	SendEvents(events []*calendar.Event) error
}

type DiscordEventNotifier struct {
	webhookURL string
}

func NewDiscordEventNotifier() (*DiscordEventNotifier, error) {
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		return nil, fmt.Errorf("環境変数 DISCORD_WEBHOOK_URL が設定されていません")
	}
	return &DiscordEventNotifier{
		webhookURL: webhookURL,
	}, nil
}

func (d DiscordEventNotifier) MakeMessage(event *calendar.Event) string {
	message := fmt.Sprintf("以下の予定が追加されました\n\nイベント: %s\n開始: %s\n終了: %s\n場所: %s\n説明: %s", event.Summary, event.Start.DateTime, event.End.DateTime, event.Location, event.Description)
	log.Println("message: ", message)
	return message
}

func (d DiscordEventNotifier) SendEvent(event *calendar.Event) error {
	message := d.MakeMessage(event)
	payload := map[string]string{
		"content": message,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", d.webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	_, err = client.Do(req)
	return err
}

func (d DiscordEventNotifier) SendEvents(events []*calendar.Event) error {
	for _, event := range events {
		err := d.SendEvent(event)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	notifier, err := NewCalendarNotifier()
	if err != nil {
		log.Fatal(err)
	}
	defer notifier.repository.client.Close()

	server := http.Server{
		Addr:    ":8080",
		Handler: nil,
	}
	http.HandleFunc("/watch", func (w http.ResponseWriter, r *http.Request) {
		var state, ok = r.Header["X-Goog-Resource-State"]
		if !ok || state[0] != "exists" {
			return
		}

		err = notifier.NotifyEvents()
		if err != nil {
			log.Fatal(err)
		}
	})

	server.ListenAndServe()
}
