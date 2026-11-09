package server

import "time"

const (
	ticketIdleTimeout     = 30 * time.Minute
	ticketMaximumLifetime = 6 * time.Hour
)
