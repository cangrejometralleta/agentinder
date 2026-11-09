package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxCloudStateBytes = 900_000

type cloudStore struct {
	client *firestore.Client
	doc    *firestore.DocumentRef
}

func OpenCloud(ctx context.Context, projectID string) (*Store, error) {
	if projectID == "" {
		return nil, errors.New("GOOGLE_CLOUD_PROJECT is required")
	}
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &Store{cloud: &cloudStore{client: client, doc: client.Collection("agentinder").Doc("state")}}, nil
}

func (s *cloudStore) read(read func(State)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snapshot, err := s.doc.Get(ctx)
	if status.Code(err) == codes.NotFound {
		read(emptyState())
		return nil
	}
	if err != nil {
		return err
	}
	data, err := decodeCloudState(snapshot)
	if err != nil {
		return err
	}
	read(data)
	return nil
}

func (s *cloudStore) update(update func(*State) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snapshot, err := tx.Get(s.doc)
		var data State
		var before []byte
		if status.Code(err) == codes.NotFound {
			data = emptyState()
		} else if err != nil {
			return err
		} else {
			if data, err = decodeCloudState(snapshot); err != nil {
				return err
			}
			before, err = json.Marshal(data)
			if err != nil {
				return err
			}
		}
		if err := update(&data); err != nil {
			return err
		}
		body, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if len(body) > maxCloudStateBytes {
			return fmt.Errorf("agentinder state exceeds the Firestore document limit: %d bytes", len(body))
		}
		if before != nil && bytes.Equal(before, body) {
			return nil
		}
		return tx.Set(s.doc, map[string]any{"json": string(body)})
	})
}

func decodeCloudState(snapshot *firestore.DocumentSnapshot) (State, error) {
	value, err := snapshot.DataAt("json")
	if err != nil {
		return State{}, err
	}
	body, ok := value.(string)
	if !ok {
		return State{}, errors.New("Firestore state has an invalid format")
	}
	var data State
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return State{}, err
	}
	return normalizeState(data), nil
}
