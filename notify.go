package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"google.golang.org/api/calendar/v3"
)

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
