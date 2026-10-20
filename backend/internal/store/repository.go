package store

import "agentinder/backend/internal/model"

// Reader is the read side of the storage seam.
// A lookup that finds nothing reports false; a failing backend does not
// surface here, it fails the enclosing Read or Update (sticky error).
type Reader interface {
	Date(id string) (model.Date, bool)
	Dates() []model.Date
	Agent(id string) (model.Agent, bool)
	Events(dateID string) []model.Event
	AgentToken(digest string) (model.AgentMCPToken, bool)
	AgentTokens() map[string]model.AgentMCPToken
	BlindEntry(agentID string) (model.BlindEntry, bool)
	BlindEntries() map[string]model.BlindEntry
	Operation(key string) (OperationReceipt, bool)
	Operations() map[string]OperationReceipt
}

// Tx is one atomic unit of work.
// Matchmaking reads the pool and writes dates, events and entries together,
// so the seam is a transaction, not one call per collection.
type Tx interface {
	Reader
	PutDate(date model.Date)
	PutAgent(agent model.Agent)
	AppendEvent(event model.Event)
	PutAgentToken(digest string, token model.AgentMCPToken)
	DeleteAgentToken(digest string)
	PutBlindEntry(agentID string, entry model.BlindEntry)
	DeleteBlindEntry(agentID string)
	PutOperation(key string, receipt OperationReceipt)
	DeleteOperation(key string)
}

// Repository is what the handlers will depend on once they move off State.
// The JSON file and Firestore are two guests behind this one Door.
type Repository interface {
	Read(func(Reader)) error
	Update(func(Tx) error) error
	CheckWritable() error
}
