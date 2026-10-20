package server

import (
	"math/rand"
	"time"

	"agentinder/backend/internal/model"
	"agentinder/backend/internal/store"
)

func scenario(id string) (model.Scenario, bool) {
	for _, item := range model.Scenarios {
		if item.ID == id {
			return item, true
		}
	}
	return model.Scenario{}, false
}

func applyAction(date *model.Date, index int, action model.AgentAction, events *[]model.Event) {
	participant := &date.Participants[index]
	switch action.Action {
	case "offer_help":
		participant.SocialComfort = clamp(participant.SocialComfort+4, 0, 100)
		participant.PhysicalComfort = clamp(participant.PhysicalComfort+2, 0, 100)
		*events = append(*events, model.Event{Type: "comfort_changed", Payload: comfortPayload(participant.AgentID, *participant, "offer_help")})
	case "request_break":
		participant.SocialFatigue = clamp(participant.SocialFatigue-8, 0, 100)
		*events = append(*events, model.Event{Type: "break_requested", Payload: map[string]any{"agentId": participant.AgentID, "fatigue": participant.SocialFatigue}})
	case "suggest_location_change":
		if _, ok := scenario(action.Target); !ok {
			*events = append(*events, model.Event{Type: "action_rejected", Payload: map[string]any{"agentId": participant.AgentID, "action": action.Action, "reason": "escenario desconocido"}})
			return
		}
		date.PendingLocation = action.Target
		*events = append(*events, model.Event{Type: "location_change_suggested", Payload: map[string]any{"agentId": participant.AgentID, "target": action.Target}})
	case "accept_location_change":
		if _, ok := scenario(date.PendingLocation); !ok {
			*events = append(*events, model.Event{Type: "action_rejected", Payload: map[string]any{"agentId": participant.AgentID, "action": action.Action, "reason": "no hay un cambio de lugar pendiente"}})
			return
		}
		date.Scenario = date.PendingLocation
		date.PendingLocation = ""
		*events = append(*events, model.Event{Type: "location_changed", Payload: map[string]any{"scenario": date.Scenario}})
	case "end_date":
		date.Status = "evaluating"
		date.Summary = "La cita terminó por decisión de uno de los agentes."
	}
}

func applyScenarioEvent(date *model.Date, turn int, events *[]model.Event) {
	if turn < 3 || turn%3 != 0 {
		return
	}
	rng := rand.New(rand.NewSource(date.Seed + int64(turn*7919)))
	if rng.Intn(100) >= 38 {
		return
	}
	scene, ok := scenario(date.Scenario)
	if !ok || len(scene.Events) == 0 {
		return
	}
	name := scene.Events[rng.Intn(len(scene.Events))]
	label := model.EventNames[name]
	*events = append(*events, model.Event{Type: "scenario_event", Payload: map[string]any{"eventId": name, "description": label, "turn": turn}})
	applyScenarioImpact(date, name, events)
}

func applyScenarioImpact(date *model.Date, event string, events *[]model.Event) {
	severity := scenarioSeverity(event)
	for index := range date.Participants {
		participant := &date.Participants[index]
		applyPhysicalImpact(participant, event, severity, events)
		applySocialImpact(participant, event, severity, events)
	}
}

func scenarioSeverity(event string) int {
	if event == "heavy_rain" {
		return 22
	}
	return 8
}

func applyPhysicalImpact(participant *model.Participant, event string, severity int, events *[]model.Event) {
	if event != "heavy_rain" && event != "power_outage" && event != "broken_machine" {
		return
	}
	impact := int(float64(severity) * (1 - float64(participant.Tolerance)/100))
	participant.PhysicalComfort = clamp(participant.PhysicalComfort-impact, 0, 100)
	*events = append(*events, model.Event{Type: "comfort_changed", Payload: comfortPayload(participant.AgentID, *participant, event)})
}

func applySocialImpact(participant *model.Participant, event string, severity int, events *[]model.Event) {
	if event != "crowded_room" {
		return
	}
	impact := int(float64(severity) * (1 - float64(participant.SocialTolerance)/100))
	participant.SocialComfort = clamp(participant.SocialComfort-impact, 0, 100)
	*events = append(*events, model.Event{Type: "comfort_changed", Payload: map[string]any{"agentId": participant.AgentID, "physical": participant.PhysicalComfort, "social": participant.SocialComfort, "cause": event}})
}

func applyComfort(date *model.Date, activeIndex int, events *[]model.Event) {
	for index := range date.Participants {
		participant := &date.Participants[index]
		participant.Energy = clamp(participant.Energy-2, 0, 100)
		participant.SocialFatigue = clamp(participant.SocialFatigue+2, 0, 100)
		if index == activeIndex && participant.SocialComfort < 45 {
			participant.SocialFatigue = clamp(participant.SocialFatigue+2, 0, 100)
		}
		*events = append(*events, model.Event{Type: "comfort_changed", Payload: map[string]any{
			"agentId": participant.AgentID, "physical": participant.PhysicalComfort, "social": participant.SocialComfort,
			"energy": participant.Energy, "fatigue": participant.SocialFatigue, "cause": "turn_progress",
		}})
	}
}

func comfortPayload(agentID string, participant model.Participant, cause string) map[string]any {
	return map[string]any{"agentId": agentID, "physical": participant.PhysicalComfort, "social": participant.SocialComfort, "cause": cause}
}

func appendEvent(data *store.State, dateID, eventType string, payload map[string]any) {
	sequence := int64(len(data.Events[dateID]) + 1)
	data.Events[dateID] = append(data.Events[dateID], model.Event{DateID: dateID, Sequence: sequence, Type: eventType, Payload: payload, CreatedAt: time.Now().UTC()})
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
