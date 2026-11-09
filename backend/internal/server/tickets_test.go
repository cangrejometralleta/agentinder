package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

func newTicketRepository(t *testing.T) store.FileRepository {
	t.Helper()
	data, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return store.NewFileRepository(data)
}

func TestIssuedTicketStoresOnlyItsHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewFileRepository(data)
	now := time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC)
	var secret string
	err = repository.Update(func(tx store.Tx) error {
		var err error
		secret, err = issueTicket(tx, "agent_1", now)
		if err != nil {
			return err
		}
		ticket, ok := tx.Ticket(digestTicket(secret))
		if !ok || ticket.AgentID != "agent_1" {
			t.Fatalf("issued ticket was not stored: %+v %v", ticket, ok)
		}
		if ticket.CreatedAt != now || ticket.LastActivityAt != now {
			t.Errorf("ticket timestamps are wrong: %+v", ticket)
		}
		if ticket.ExpiresAt != now.Add(ticketIdleTimeout) {
			t.Errorf("ticket expiry is wrong: %v", ticket.ExpiresAt)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || digestTicket(secret) == secret {
		t.Errorf("ticket secret was not opaque: %q", secret)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secret) {
		t.Error("ticket secret was written to state")
	}
	if !strings.Contains(string(body), digestTicket(secret)) {
		t.Error("ticket hash was not written to state")
	}
}

func TestTicketVerificationRenewsActivity(t *testing.T) {
	repository := newTicketRepository(t)
	createdAt := time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC)
	activityAt := createdAt.Add(10 * time.Minute)
	var secret string
	err := repository.Update(func(tx store.Tx) error {
		var err error
		secret, err = issueTicket(tx, "agent_1", createdAt)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = repository.Update(func(tx store.Tx) error {
		if ticket, ok := verifyTicket(tx, secret, activityAt); !ok || ticket.AgentID != "agent_1" {
			t.Fatalf("valid ticket was rejected: %+v %v", ticket, ok)
		}
		ticket, ok := renewTicket(tx, secret, activityAt)
		if !ok {
			t.Fatal("valid ticket was not renewed")
		}
		if ticket.LastActivityAt != activityAt || ticket.ExpiresAt != activityAt.Add(ticketIdleTimeout) {
			t.Errorf("ticket was not renewed: %+v", ticket)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExpiredTicketIsRemovedOnVerification(t *testing.T) {
	repository := newTicketRepository(t)
	createdAt := time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC)
	var secret string
	err := repository.Update(func(tx store.Tx) error {
		var err error
		secret, err = issueTicket(tx, "agent_1", createdAt)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = repository.Update(func(tx store.Tx) error {
		if _, ok := verifyTicket(tx, secret, createdAt.Add(ticketIdleTimeout)); ok {
			t.Fatal("ticket was accepted at its expiry time")
		}
		if _, ok := tx.Ticket(digestTicket(secret)); ok {
			t.Error("expired ticket was not removed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTicketRenewalCannotExceedMaximumLifetime(t *testing.T) {
	repository := newTicketRepository(t)
	createdAt := time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC)
	var secret string
	err := repository.Update(func(tx store.Tx) error {
		var err error
		secret, err = issueTicket(tx, "agent_1", createdAt)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	lastActivity := createdAt.Add(ticketMaximumLifetime - time.Minute)
	err = repository.Update(func(tx store.Tx) error {
		for activityAt := createdAt.Add(ticketIdleTimeout - time.Minute); activityAt.Before(lastActivity); activityAt = activityAt.Add(ticketIdleTimeout - time.Minute) {
			if _, ok := renewTicket(tx, secret, activityAt); !ok {
				t.Fatalf("ticket was rejected during regular activity at %v", activityAt)
			}
		}
		ticket, ok := renewTicket(tx, secret, lastActivity)
		if !ok {
			t.Fatal("ticket was rejected before its maximum lifetime")
		}
		if ticket.ExpiresAt != createdAt.Add(ticketMaximumLifetime) {
			t.Errorf("expiry exceeded the maximum lifetime: %v", ticket.ExpiresAt)
		}
		if _, ok := renewTicket(tx, secret, createdAt.Add(ticketMaximumLifetime)); ok {
			t.Error("ticket was renewed after its maximum lifetime")
		}
		if _, ok := tx.Ticket(digestTicket(secret)); ok {
			t.Error("expired ticket was not removed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTicketSweepRemovesOnlyExpiredTickets(t *testing.T) {
	repository := newTicketRepository(t)
	now := time.Date(2026, time.October, 23, 12, 0, 0, 0, time.UTC)
	err := repository.Update(func(tx store.Tx) error {
		tx.PutTicket("expired", model.Ticket{CreatedAt: now.Add(-ticketIdleTimeout), ExpiresAt: now})
		tx.PutTicket("active", model.Ticket{CreatedAt: now, ExpiresAt: now.Add(ticketIdleTimeout)})
		if count := expireTickets(tx, now); count != 1 {
			t.Errorf("expired ticket count: got %d, want 1", count)
		}
		if _, ok := tx.Ticket("expired"); ok {
			t.Error("sweep left expired ticket in storage")
		}
		if _, ok := tx.Ticket("active"); !ok {
			t.Error("sweep removed active ticket")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
