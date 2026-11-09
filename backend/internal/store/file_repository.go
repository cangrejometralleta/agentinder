package store

import (
	"maps"
	"slices"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
)

var _ Repository = FileRepository{}

// FileRepository answers the Repository Door from the JSON file Store.
type FileRepository struct {
	store *Store
}

func NewFileRepository(store *Store) FileRepository {
	return FileRepository{store: store}
}

func TxOver(data *State) Tx {
	return fileTx{fileReader{data: data}}
}

func (r FileRepository) Read(read func(Reader)) error {
	return r.store.Read(func(data State) { read(fileReader{data: &data}) })
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

func (r fileReader) Ticket(hash string) (model.Ticket, bool) {
	ticket, ok := r.data.Tickets[hash]
	return ticket, ok
}

func (r fileReader) Tickets() map[string]model.Ticket {
	return maps.Clone(r.data.Tickets)
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

func (t fileTx) PutTicket(hash string, ticket model.Ticket) {
	t.data.Tickets[hash] = ticket
}

func (t fileTx) DeleteTicket(hash string) { delete(t.data.Tickets, hash) }

func (t fileTx) PutBlindEntry(agentID string, entry model.BlindEntry) {
	t.data.BlindPool[agentID] = entry
}
func (t fileTx) DeleteBlindEntry(agentID string) { delete(t.data.BlindPool, agentID) }

func (t fileTx) PutOperation(key string, receipt OperationReceipt) { t.data.Operations[key] = receipt }
func (t fileTx) DeleteOperation(key string)                        { delete(t.data.Operations, key) }
