package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

func TestServerlessRequestClosesIdleDate(t *testing.T) {
	data, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := data.Update(func(state *store.State) error {
		state.Agents["agent_1"] = model.Agent{ID: "agent_1", Name: "Ada"}
		state.Dates["date_1"] = model.Date{
			ID: "date_1", Mode: "mcp", Status: "running",
			UpdatedAt:    time.Now().Add(-mcpDateIdleLimit - time.Minute),
			Participants: []model.Participant{{AgentID: "agent_1"}},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	api := New(data, "", ringOf(t, 1))
	api.UseServerless()
	recorder := httptest.NewRecorder()
	api.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/public/dates", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("observatory status: %d", recorder.Code)
	}
	if err := data.Read(func(state store.State) {
		if state.Dates["date_1"].Status != "completed" {
			t.Error("idle date stayed open after a request")
		}
	}); err != nil {
		t.Fatal(err)
	}
}
