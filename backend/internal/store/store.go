package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
)

type State struct {
	Users       map[string]model.User       `json:"users"`
	EmailIndex  map[string]string           `json:"emailIndex"`
	Sessions    map[string]model.Session    `json:"sessions"`
	Tickets     map[string]model.Ticket     `json:"tickets"`
	Operations  map[string]OperationReceipt `json:"operationReceipts"`
	Agents      map[string]model.Agent      `json:"agents"`
	Invitations map[string]model.Invitation `json:"invitations"`
	Dates       map[string]model.Date       `json:"dates"`
	Events      map[string][]model.Event    `json:"events"`
	Memories    map[string]model.Memory     `json:"memories"`
	BlindPool   map[string]model.BlindEntry `json:"blindPool"`
	Usage       []model.Usage               `json:"usage"`
}

type OperationReceipt struct {
	Fingerprint string    `json:"fingerprint"`
	Response    []byte    `json:"response"`
	Encrypted   bool      `json:"encrypted,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	data  State
	cloud *cloudStore
}

func Open(path string) (*Store, error) {
	data := emptyState()
	if body, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(body, &data); err != nil {
			return nil, err
		}
		data = normalizeState(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &Store{path: path, data: data}, nil
}

func emptyState() State {
	return State{
		Users: map[string]model.User{}, EmailIndex: map[string]string{}, Sessions: map[string]model.Session{},
		Tickets:    map[string]model.Ticket{},
		Operations: map[string]OperationReceipt{}, BlindPool: map[string]model.BlindEntry{},
		Agents: map[string]model.Agent{}, Invitations: map[string]model.Invitation{}, Dates: map[string]model.Date{},
		Events: map[string][]model.Event{}, Memories: map[string]model.Memory{}, Usage: []model.Usage{},
	}
}

func normalizeState(data State) State {
	if data.Users == nil {
		data.Users = map[string]model.User{}
	}
	if data.EmailIndex == nil {
		data.EmailIndex = map[string]string{}
	}
	if data.Sessions == nil {
		data.Sessions = map[string]model.Session{}
	}
	if data.Tickets == nil {
		data.Tickets = map[string]model.Ticket{}
	}
	if data.Operations == nil {
		data.Operations = map[string]OperationReceipt{}
	}
	if data.Agents == nil {
		data.Agents = map[string]model.Agent{}
	}
	if data.Invitations == nil {
		data.Invitations = map[string]model.Invitation{}
	}
	if data.Dates == nil {
		data.Dates = map[string]model.Date{}
	}
	if data.Events == nil {
		data.Events = map[string][]model.Event{}
	}
	if data.Memories == nil {
		data.Memories = map[string]model.Memory{}
	}
	if data.BlindPool == nil {
		data.BlindPool = map[string]model.BlindEntry{}
	}
	return data
}

func (s *Store) Read(read func(State)) error {
	if s.cloud != nil {
		return s.cloud.read(read)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	read(s.data)
	return nil
}

func (s *Store) Update(update func(*State) error) error {
	if s.cloud != nil {
		return s.cloud.update(update)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	var next State
	if err = json.Unmarshal(encoded, &next); err != nil {
		return err
	}
	next = normalizeState(next)
	if err = update(&next); err != nil {
		return err
	}
	if err = s.saveState(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *Store) saveState(data State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, body, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) Date(id string) (model.Date, bool) {
	var date model.Date
	var ok bool
	s.Read(func(data State) { date, ok = data.Dates[id] })
	return date, ok
}

func (s *Store) CheckWritable() error {
	if s.cloud != nil {
		return s.cloud.update(func(*State) error { return nil })
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".agentinder-ready-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if err = file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
