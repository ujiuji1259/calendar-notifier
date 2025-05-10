package main

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/datastore"
)

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

func (r *DatastoreSyncTokenRepository) Close() error {
	return r.client.Close()
} 