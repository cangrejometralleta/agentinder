package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

func observatoryResponse(t *testing.T, seed func(*store.State)) *httptest.ResponseRecorder {
	t.Helper()
	opened, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = opened.Update(func(data *store.State) error { seed(data); return nil }); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	New(opened, "", nil).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/public/dates", nil))
	return recorder
}

func TestPublicObservatoryEmptyIsAnArray(t *testing.T) {
	response := observatoryResponse(t, func(*store.State) {})
	if response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Errorf("want 200 [], got %d %q", response.Code, response.Body.String())
	}
}

func TestPublicObservatoryShapesPublicDates(t *testing.T) {
	base := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
	response := observatoryResponse(t, func(data *store.State) {
		data.Agents["a"] = model.Agent{ID: "a", Name: "Ada", Tags: []string{"té"}, Skills: []string{"talk"}}
		data.Agents["b"] = model.Agent{ID: "b", Name: "Bruno"}
		pair := []model.Participant{{AgentID: "a"}, {AgentID: "b"}}
		data.Dates["late"] = model.Date{ID: "late", Mode: "mcp", Status: "completed", Scenario: "cafe", Participants: pair, CreatedAt: base.Add(time.Hour)}
		data.Dates["early"] = model.Date{ID: "early", Mode: "mcp", Status: "running", Scenario: "park", Participants: pair, CreatedAt: base}
		data.Dates["waiting"] = model.Date{ID: "waiting", Mode: "mcp", Status: "waiting", CreatedAt: base}
		data.Dates["solo"] = model.Date{ID: "solo", Mode: "solo", Status: "running", CreatedAt: base}
		data.Events["early"] = []model.Event{
			{DateID: "early", Sequence: 1, Type: "date_started", Payload: map[string]any{"participants": []string{"a", "b"}, "scenario": "park"}, CreatedAt: base},
			{DateID: "early", Sequence: 2, Type: "agent_message", Payload: map[string]any{"agentId": "a", "text": "hola"}, CreatedAt: base},
			{DateID: "early", Sequence: 3, Type: "location_change_suggested", Payload: map[string]any{"requestedBy": "b", "targetAgentId": "a"}, CreatedAt: base},
			{DateID: "early", Sequence: 4, Type: "private_note", Payload: map[string]any{"text": "secreto"}, CreatedAt: base},
		}
	})
	if response.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", response.Code)
	}
	var dates []struct {
		ID           string `json:"id"`
		Participants []struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Tags   []string `json:"tags"`
			Skills []string `json:"skills"`
		} `json:"participants"`
		Events []model.Event `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &dates); err != nil {
		t.Fatal(err)
	}
	if len(dates) != 2 || dates[0].ID != "early" || dates[1].ID != "late" {
		t.Fatalf("want [early late] only, got %+v", dates)
	}
	early := dates[0]
	if len(early.Participants) != 2 || early.Participants[0].Name != "Ada" || early.Participants[0].Skills[0] != "talk" || early.Participants[1].Name != "Bruno" {
		t.Errorf("participants: %+v", early.Participants)
	}
	if len(early.Events) != 3 {
		t.Fatalf("want 3 public events, got %+v", early.Events)
	}
	for _, event := range early.Events {
		if event.DateID != "early" {
			t.Errorf("event %d lost its dateId: %+v", event.Sequence, event)
		}
		if event.Type == "private_note" {
			t.Error("a non-public event type leaked")
		}
	}
	if _, leaked := early.Events[0].Payload["participants"]; leaked {
		t.Error("participants leaked into the public payload")
	}
	if early.Events[0].Payload["scenario"] != "park" {
		t.Errorf("plain payload keys must stay: %+v", early.Events[0].Payload)
	}
	message := early.Events[1].Payload
	if _, leaked := message["agentId"]; leaked || message["agentName"] != "Ada" || message["text"] != "hola" {
		t.Errorf("agent id must become agentName: %+v", message)
	}
	suggestion := early.Events[2].Payload
	if suggestion["requestedByName"] != "Bruno" || suggestion["targetAgentName"] != "Ada" {
		t.Errorf("requester and target must become names: %+v", suggestion)
	}
}
