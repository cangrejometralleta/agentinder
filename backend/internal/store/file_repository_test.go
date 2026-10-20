package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agentinder/backend/internal/model"
)

func openRepository(t *testing.T, path string) FileRepository {
	t.Helper()
	opened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return NewFileRepository(opened)
}

func TestFileRepositoryPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	repository := openRepository(t, path)
	err := repository.Update(func(tx Tx) error {
		tx.PutDate(model.Date{ID: "date_1", Status: "running"})
		tx.PutAgent(model.Agent{ID: "agent_1"})
		tx.AppendEvent(model.Event{DateID: "date_1", Sequence: 1, Type: "date_started"})
		tx.PutAgentToken("digest", model.AgentMCPToken{AgentID: "agent_1"})
		tx.PutBlindEntry("agent_1", model.BlindEntry{DateID: "date_1"})
		tx.PutOperation("key", OperationReceipt{Fingerprint: "f", ExpiresAt: time.Now().Add(time.Hour)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	reopened := openRepository(t, path)
	err = reopened.Read(func(r Reader) {
		if date, ok := r.Date("date_1"); !ok || date.Status != "running" {
			t.Errorf("date not persisted: %+v %v", date, ok)
		}
		if _, ok := r.Agent("agent_1"); !ok {
			t.Error("agent not persisted")
		}
		if events := r.Events("date_1"); len(events) != 1 || events[0].Type != "date_started" {
			t.Errorf("events not persisted: %+v", events)
		}
		if token, ok := r.AgentToken("digest"); !ok || token.AgentID != "agent_1" {
			t.Errorf("token not persisted: %+v %v", token, ok)
		}
		if entry, ok := r.BlindEntry("agent_1"); !ok || entry.DateID != "date_1" {
			t.Errorf("blind entry not persisted: %+v %v", entry, ok)
		}
		if receipt, ok := r.Operation("key"); !ok || receipt.Fingerprint != "f" {
			t.Errorf("operation not persisted: %+v %v", receipt, ok)
		}
		if len(r.Dates()) != 1 {
			t.Errorf("dates: %d", len(r.Dates()))
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFileRepositoryFailedUpdateChangesNothing(t *testing.T) {
	repository := openRepository(t, filepath.Join(t.TempDir(), "state.json"))
	failure := errors.New("refused")
	err := repository.Update(func(tx Tx) error {
		tx.PutDate(model.Date{ID: "date_1"})
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("expected the update error, got %v", err)
	}
	_ = repository.Read(func(r Reader) {
		if _, ok := r.Date("date_1"); ok {
			t.Error("a failed update left its write behind")
		}
	})
}

func TestFileRepositoryDeletes(t *testing.T) {
	repository := openRepository(t, filepath.Join(t.TempDir(), "state.json"))
	_ = repository.Update(func(tx Tx) error {
		tx.PutAgentToken("digest", model.AgentMCPToken{AgentID: "agent_1"})
		tx.PutBlindEntry("agent_1", model.BlindEntry{})
		tx.PutOperation("key", OperationReceipt{})
		return nil
	})
	_ = repository.Update(func(tx Tx) error {
		tx.DeleteAgentToken("digest")
		tx.DeleteBlindEntry("agent_1")
		tx.DeleteOperation("key")
		return nil
	})
	_ = repository.Read(func(r Reader) {
		if len(r.AgentTokens()) != 0 || len(r.BlindEntries()) != 0 || len(r.Operations()) != 0 {
			t.Error("deleted records are still present")
		}
	})
}

func TestFileRepositoryReadersReturnCopies(t *testing.T) {
	repository := openRepository(t, filepath.Join(t.TempDir(), "state.json"))
	_ = repository.Update(func(tx Tx) error {
		tx.PutBlindEntry("agent_1", model.BlindEntry{DateID: "date_1"})
		tx.AppendEvent(model.Event{DateID: "date_1", Sequence: 1})
		return nil
	})
	_ = repository.Read(func(r Reader) {
		delete(r.BlindEntries(), "agent_1")
		_ = append(r.Events("date_1")[:0], model.Event{Sequence: 99})
	})
	_ = repository.Read(func(r Reader) {
		if _, ok := r.BlindEntry("agent_1"); !ok {
			t.Error("mutating a returned map changed the store")
		}
		if events := r.Events("date_1"); events[0].Sequence != 1 {
			t.Errorf("mutating a returned slice changed the store: %+v", events)
		}
	})
}
