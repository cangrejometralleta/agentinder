package store

import (
	"maps"
	"slices"

	"agentinder/backend/internal/model"
)

var _ Repository = FileRepository{}

// FileRepository answers the Repository Door from the JSON file Store.
type FileRepository struct {
	store *Store
}

func NewFileRepository(store *Store) FileRepository {
	return FileRepository{store: store}
}

func (r FileRepository) Read(read func(Reader)) error {
	r.store.Read(func(data State) { read(fileReader{data: &data}) })
	return nil
}

func (r FileRepository) Update(update func(Tx) error) error {
	return r.store.Update(func(data *State) error { return update(fileTx{fileReader{data: data}}) })
}

func (r FileRepository) CheckWritable() error {
	return r.store.CheckWritable()
}

type fileReader struct {
	data *State
}

func (r fileReader) Date(id string) (model.Date, bool) {
	date, ok := r.data.Dates[id]
	return date, ok
}

func (r fileReader) Dates() []model.Date {
	return slices.Collect(maps.Values(r.data.Dates))
}

func (r fileReader) Agent(id string) (model.Agent, bool) {
	agent, ok := r.data.Agents[id]
	return agent, ok
}

func (r fileReader) Events(dateID string) []model.Event {
	return slices.Clone(r.data.Events[dateID])
}

func (r fileReader) AgentToken(digest string) (model.AgentMCPToken, bool) {
	token, ok := r.data.AgentTokens[digest]
	return token, ok
}

func (r fileReader) AgentTokens() map[string]model.AgentMCPToken {
	return maps.Clone(r.data.AgentTokens)
}

func (r fileReader) BlindEntry(agentID string) (model.BlindEntry, bool) {
	entry, ok := r.data.BlindPool[agentID]
	return entry, ok
}

func (r fileReader) BlindEntries() map[string]model.BlindEntry {
	return maps.Clone(r.data.BlindPool)
}

func (r fileReader) Operation(key string) (OperationReceipt, bool) {
	receipt, ok := r.data.Operations[key]
	return receipt, ok
}

func (r fileReader) Operations() map[string]OperationReceipt {
	return maps.Clone(r.data.Operations)
}

type fileTx struct {
	fileReader
}

func (t fileTx) PutDate(date model.Date)    { t.data.Dates[date.ID] = date }
func (t fileTx) PutAgent(agent model.Agent) { t.data.Agents[agent.ID] = agent }

func (t fileTx) AppendEvent(event model.Event) {
	t.data.Events[event.DateID] = append(t.data.Events[event.DateID], event)
}

func (t fileTx) PutAgentToken(digest string, token model.AgentMCPToken) {
	t.data.AgentTokens[digest] = token
}
func (t fileTx) DeleteAgentToken(digest string) { delete(t.data.AgentTokens, digest) }

func (t fileTx) PutBlindEntry(agentID string, entry model.BlindEntry) {
	t.data.BlindPool[agentID] = entry
}
func (t fileTx) DeleteBlindEntry(agentID string) { delete(t.data.BlindPool, agentID) }

func (t fileTx) PutOperation(key string, receipt OperationReceipt) { t.data.Operations[key] = receipt }
func (t fileTx) DeleteOperation(key string)                        { delete(t.data.Operations, key) }
