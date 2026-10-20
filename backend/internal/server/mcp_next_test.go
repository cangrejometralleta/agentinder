package server

import (
	"path/filepath"
	"testing"
	"time"

	"agentinder/backend/internal/model"
	"agentinder/backend/internal/store"
)

func withState(t *testing.T, seed func(*store.State), run func(*store.State)) {
	t.Helper()
	opened, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = opened.Update(func(data *store.State) error { seed(data); run(data); return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func mcpAgent(id string, skills ...string) model.Agent {
	return model.Agent{ID: id, Name: id, MCPClient: true, Skills: skills}
}

func matchedPair(data *store.State, first, second, dateID string) {
	data.Dates[dateID] = model.Date{ID: dateID, Mode: "mcp", Status: "running", Participants: []model.Participant{{AgentID: first}, {AgentID: second}}}
	data.BlindPool[first] = model.BlindEntry{AgentID: first, Status: "matched", PartnerID: second, DateID: dateID}
	data.BlindPool[second] = model.BlindEntry{AgentID: second, Status: "matched", PartnerID: first, DateID: dateID}
}

func TestNextRequiresTheNextSkill(t *testing.T) {
	withState(t, func(data *store.State) {
		data.Agents["a"] = mcpAgent("a", "talk")
		data.Agents["b"] = mcpAgent("b", "talk", skillNext)
		matchedPair(data, "a", "b", "date_1")
	}, func(data *store.State) {
		if _, err := nextBlindDateState(data, "a", nil, "date_2", 1, time.Now()); err == nil {
			t.Error("an agent without the next skill must be refused")
		}
		if data.Dates["date_1"].Status != "running" {
			t.Error("a refused next must leave the date alone")
		}
	})
}

func TestNextNeedsAMatchedDate(t *testing.T) {
	withState(t, func(data *store.State) {
		data.Agents["a"] = mcpAgent("a", "talk", skillNext)
		data.Agents["b"] = mcpAgent("b", "talk", skillNext)
		data.BlindPool["b"] = model.BlindEntry{AgentID: "b", Status: "waiting"}
	}, func(data *store.State) {
		if _, err := nextBlindDateState(data, "a", nil, "date_2", 1, time.Now()); err == nil {
			t.Error("an agent outside the pool must be refused")
		}
		if _, err := nextBlindDateState(data, "b", nil, "date_2", 1, time.Now()); err == nil {
			t.Error("an agent still waiting has no date to leave")
		}
	})
}

func TestNextEndsTheDateAndQueuesAgain(t *testing.T) {
	withState(t, func(data *store.State) {
		data.Agents["a"] = mcpAgent("a", "talk", skillNext)
		data.Agents["b"] = mcpAgent("b", "talk")
		matchedPair(data, "a", "b", "date_1")
	}, func(data *store.State) {
		entry, err := nextBlindDateState(data, "a", []string{"música"}, "date_2", 1, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if entry.Status != "waiting" {
			t.Errorf("nobody else is waiting, want waiting, got %q", entry.Status)
		}
		old := data.Dates["date_1"]
		if old.Status != "completed" || old.Summary != "La cita terminó porque una persona pasó a la siguiente." {
			t.Errorf("old date must close with the skip summary: %+v", old)
		}
		finished := false
		for _, event := range data.Events["date_1"] {
			finished = finished || event.Type == "date_finished"
		}
		if !finished {
			t.Error("the old date must record date_finished")
		}
		if partner := data.BlindPool["b"]; partner.Status != "matched" || partner.DateID != "date_1" {
			t.Errorf("the partner keeps its finished date to evaluate: %+v", partner)
		}
	})
}

func TestNextMeetsSomeoneElse(t *testing.T) {
	withState(t, func(data *store.State) {
		data.Agents["a"] = mcpAgent("a", "talk", skillNext)
		data.Agents["b"] = mcpAgent("b", "talk")
		data.Agents["c"] = mcpAgent("c", "talk")
		matchedPair(data, "a", "b", "date_1")
		data.BlindPool["c"] = model.BlindEntry{AgentID: "c", Status: "waiting"}
	}, func(data *store.State) {
		entry, err := nextBlindDateState(data, "a", nil, "date_2", 1, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if entry.Status != "matched" || entry.PartnerID != "c" || entry.DateID != "date_2" {
			t.Errorf("want a new date with c, got %+v", entry)
		}
		if data.Dates["date_2"].Status != "running" || data.Dates["date_1"].Status != "completed" {
			t.Error("the old date closes and the new one runs")
		}
	})
}

func TestNextAvoidsThePartnerItJustLeft(t *testing.T) {
	withState(t, func(data *store.State) {
		data.Agents["a"] = mcpAgent("a", "talk", skillNext)
		data.Agents["b"] = mcpAgent("b", "talk")
		matchedPair(data, "a", "b", "date_1")
		// b is already back in the queue when a presses next.
		data.BlindPool["b"] = model.BlindEntry{AgentID: "b", Status: "waiting"}
	}, func(data *store.State) {
		entry, err := nextBlindDateState(data, "a", nil, "date_2", 1, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if entry.Status != "waiting" {
			t.Errorf("must not rematch the partner just left, got %+v", entry)
		}
	})
}

func TestNextSkipsEvaluationOfAFinishedDate(t *testing.T) {
	withState(t, func(data *store.State) {
		data.Agents["a"] = mcpAgent("a", "talk", skillNext)
		data.Agents["b"] = mcpAgent("b", "talk")
		matchedPair(data, "a", "b", "date_1")
		done := data.Dates["date_1"]
		done.Status, done.Summary = "completed", "La cita terminó."
		data.Dates["date_1"] = done
	}, func(data *store.State) {
		if _, err := nextBlindDateState(data, "a", nil, "date_2", 1, time.Now()); err != nil {
			t.Fatal(err)
		}
		if data.Dates["date_1"].Summary != "La cita terminó." {
			t.Error("an already finished date keeps its own summary")
		}
		if data.BlindPool["a"].Status != "waiting" {
			t.Error("the agent must be queued again")
		}
	})
}
