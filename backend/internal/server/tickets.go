package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

func issueTicket(tx store.Tx, agentID string, now time.Time) (string, error) {
	secret, err := makeTicketSecret()
	if err != nil {
		return "", err
	}

	ticket := model.Ticket{
		AgentID:        agentID,
		CreatedAt:      now,
		LastActivityAt: now,
		ExpiresAt:      ticketExpiry(now, now),
	}
	tx.PutTicket(digestTicket(secret), ticket)
	return secret, nil
}

func verifyTicket(tx store.Tx, secret string, now time.Time) (model.Ticket, bool) {
	hash := digestTicket(secret)
	ticket, ok := tx.Ticket(hash)
	if !ok {
		return model.Ticket{}, false
	}
	if ticketExpired(ticket, now) {
		tx.DeleteTicket(hash)
		return model.Ticket{}, false
	}
	return ticket, true
}

func renewTicket(tx store.Tx, secret string, now time.Time) (model.Ticket, bool) {
	hash := digestTicket(secret)
	ticket, ok := verifyTicket(tx, secret, now)
	if !ok {
		return model.Ticket{}, false
	}
	ticket.LastActivityAt = now
	ticket.ExpiresAt = ticketExpiry(ticket.CreatedAt, now)
	tx.PutTicket(hash, ticket)
	return ticket, true
}

func expireTickets(tx store.Tx, now time.Time) int {
	expired := 0
	for hash, ticket := range tx.Tickets() {
		if ticketExpired(ticket, now) {
			tx.DeleteTicket(hash)
			expired++
		}
	}
	return expired
}

func makeTicketSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}

func digestTicket(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func ticketExpired(ticket model.Ticket, now time.Time) bool {
	return !now.Before(ticket.ExpiresAt) || !now.Before(ticket.CreatedAt.Add(ticketMaximumLifetime))
}

func ticketExpiry(createdAt, activityAt time.Time) time.Time {
	idleExpiry := activityAt.Add(ticketIdleTimeout)
	maximumExpiry := createdAt.Add(ticketMaximumLifetime)
	if idleExpiry.Before(maximumExpiry) {
		return idleExpiry
	}
	return maximumExpiry
}
