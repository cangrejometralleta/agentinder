package server

import (
	"net/http"
	"sort"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

type observatoryParticipant struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Tags   []string `json:"tags,omitempty"`
	Skills []string `json:"skills,omitempty"`
}

type observatoryDate struct {
	ID            string                   `json:"id"`
	Scenario      string                   `json:"scenario"`
	Status        string                   `json:"status"`
	CurrentTurn   int                      `json:"currentTurn"`
	MaxTurns      int                      `json:"maxTurns"`
	SimulatedTime int                      `json:"simulatedTime"`
	Participants  []observatoryParticipant `json:"participants"`
	Events        []model.Event            `json:"events"`
	Summary       string                   `json:"summary,omitempty"`
	CreatedAt     time.Time                `json:"createdAt"`
}

var publicDateEventTypes = map[string]bool{
	"date_started": true, "agent_message": true, "scenario_event": true,
	"location_change_suggested": true, "location_changed": true, "break_requested": true,
	"date_finished": true, "consent_requested": true, "consent_granted": true, "consent_declined": true,
}

// publicMCPDateVisible says whether a date may appear in the public observatory.
func publicMCPDateVisible(date model.Date) bool {
	return date.Mode == "mcp" && (date.Status == "running" || date.Status == "evaluating" || date.Status == "completed")
}

func (s *Server) publicObservatoryDates(w http.ResponseWriter, _ *http.Request) {
	if s.serverless {
		if err := s.closeIdleDates(); err != nil {
			writeFailure(w, err)
			return
		}
	}
	dates := []observatoryDate{}
	err := s.Repository.Read(func(data store.Reader) {
		for _, date := range data.Dates() {
			if !publicMCPDateVisible(date) {
				continue
			}
			item := observatoryDate{
				ID: date.ID, Scenario: date.Scenario, Status: date.Status,
				CurrentTurn: date.CurrentTurn, MaxTurns: date.MaxTurns,
				SimulatedTime: date.SimulatedTime, Summary: date.Summary,
				CreatedAt: date.CreatedAt, Participants: []observatoryParticipant{}, Events: []model.Event{},
			}
			names := map[string]string{}
			for _, participant := range date.Participants {
				agent, _ := data.Agent(participant.AgentID)
				names[agent.ID] = agent.Name
				item.Participants = append(item.Participants, observatoryParticipant{ID: agent.ID, Name: agent.Name, Tags: agent.Tags, Skills: agent.Skills})
			}
			for _, event := range data.Events(date.ID) {
				if !publicDateEventTypes[event.Type] {
					continue
				}
				payload := map[string]any{}
				for key, value := range event.Payload {
					if key == "agentId" || key == "requestedBy" || key == "targetAgentId" {
						if id, ok := value.(string); ok {
							nameKey := "agentName"
							if key == "requestedBy" {
								nameKey = "requestedByName"
							}
							if key == "targetAgentId" {
								nameKey = "targetAgentName"
							}
							if name := names[id]; name != "" {
								payload[nameKey] = name
							}
						}
						continue
					}
					if key == "participants" {
						continue
					}
					payload[key] = value
				}
				item.Events = append(item.Events, model.Event{DateID: date.ID, Sequence: event.Sequence, Type: event.Type, Payload: payload, CreatedAt: event.CreatedAt})
			}
			dates = append(dates, item)
		}
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].CreatedAt.Before(dates[j].CreatedAt) })
	writeJSON(w, http.StatusOK, dates)
}
