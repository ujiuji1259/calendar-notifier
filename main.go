package main

import (
	"context"
	"log"
	"net/http"

	"google.golang.org/api/calendar/v3"
)

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

func main() {
	notifier, err := NewCalendarNotifier()
	if err != nil {
		log.Fatal(err)
	}
	defer notifier.repository.Close()

	server := http.Server{
		Addr:    ":8080",
		Handler: nil,
	}
	http.HandleFunc("/watch/v2", func (w http.ResponseWriter, r *http.Request) {
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
