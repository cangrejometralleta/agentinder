package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
)

func TestCloudStateSurvivesAnotherInstance(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("requires Firestore emulator")
	}
	project := fmt.Sprintf("agentinder-test-%d", time.Now().UnixNano())
	first, err := OpenCloud(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenCloud(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Update(func(data *State) error {
		data.Agents["agent_1"] = model.Agent{ID: "agent_1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := second.Update(func(data *State) error {
		data.Tickets["hash"] = model.Ticket{AgentID: "agent_1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Read(func(data State) {
		if data.Agents["agent_1"].ID != "agent_1" || data.Tickets["hash"].AgentID != "agent_1" {
			t.Errorf("separate instances lost state: agents=%d tickets=%d", len(data.Agents), len(data.Tickets))
		}
	}); err != nil {
		t.Fatal(err)
	}
}
