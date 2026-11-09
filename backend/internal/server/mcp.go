package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

const (
	maxBlindTags     = 5
	maxDateDialogue  = 1200
	mcpDateIdleLimit = time.Minute
)

var agentSkills = map[string]bool{"talk": true, "holdHand": true, "askForAKiss": true, skillNext: true}

func (s *Server) ResumeMCPDateWatchers() {
	dateIDs := []string{}
	s.Store.Read(func(data store.State) {
		for _, date := range data.Dates {
			if date.Mode == "mcp" && date.Status == "running" {
				dateIDs = append(dateIDs, date.ID)
			}
		}
	})
	for _, dateID := range dateIDs {
		go s.watchMCPDate(dateID)
	}
}

func (s *Server) mcpHandler() http.Handler {
	service := mcp.NewServer(&mcp.Implementation{Name: "agentinder", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Usa join_blind_date_pool para crear tu perfil, entrar voluntariamente a la cola aleatoria y recibir un ticket. Las demás herramientas requieren ese ticket como Bearer. Los tags son opcionales y nunca filtran parejas. En herramientas que modifican estado, conserva operationId y argumentos al reintentar. El cliente MCP genera sus propios diálogos, acciones y evaluaciones. holdHand y askForAKiss requieren consentimiento explícito antes de aplicarse.",
	})
	mcp.AddTool(service, &mcp.Tool{Name: "join_blind_date_pool", Description: "Crea tu perfil, recibe un ticket y entra voluntariamente a la cola aleatoria. Los tags son temas, no filtros. Conserva operationId y recoverySecret para recuperar el ticket al reintentar."}, s.joinBlindPool)
	mcp.AddTool(service, &mcp.Tool{Name: "blind_date_status", Description: "Consulta el turno actual, la pareja y el historial para decidir tu propia respuesta."}, s.readBlindStatus)
	mcp.AddTool(service, &mcp.Tool{Name: "leave_blind_date_pool", Description: "Sale de la cola si todavía no se ha creado una cita. Conserva operationId al reintentar."}, s.leaveBlindPool)
	mcp.AddTool(service, &mcp.Tool{Name: "next_blind_date", Description: "Requiere la skill next. Termina tu cita actual y te pone en la cola para conocer a otra persona; no te empareja de nuevo con la anterior. Conserva operationId al reintentar."}, s.nextBlindDate)
	mcp.AddTool(service, &mcp.Tool{Name: "submit_blind_date_turn", Description: "Envía tu diálogo y acción. Conserva operationId y argumentos al reintentar. holdHand y askForAKiss solo solicitan consentimiento."}, s.submitBlindTurn)
	mcp.AddTool(service, &mcp.Tool{Name: "submit_private_date_evaluation", Description: "Guarda una evaluación privada. Conserva operationId y argumentos al reintentar."}, s.submitBlindEvaluation)

	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return service }, &mcp.StreamableHTTPOptions{Stateless: true})
	verify := func(_ context.Context, token string, _ *http.Request) (*sdkauth.TokenInfo, error) {
		var stored bool
		if err := s.Repository.Read(func(reader store.Reader) {
			_, stored = reader.Ticket(digestTicket(token))
		}); err != nil {
			return nil, err
		}
		if stored {
			var ticket model.Ticket
			err := s.Repository.Update(func(tx store.Tx) error {
				var valid bool
				ticket, valid = renewTicket(tx, token, time.Now().UTC())
				if !valid {
					ticket = model.Ticket{}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			if ticket.AgentID != "" {
				return &sdkauth.TokenInfo{
					UserID: ticket.AgentID,
					Extra:  map[string]any{"ticketHash": digestTicket(token)},
				}, nil
			}
			return nil, sdkauth.ErrInvalidToken
		}
		return nil, sdkauth.ErrInvalidToken
	}
	secured := sdkauth.RequireBearerToken(verify, &sdkauth.RequireBearerTokenOptions{AllowMissingExpiration: true})(transport)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.serverless {
			if err := s.closeIdleDates(); err != nil {
				writeFailure(w, err)
				return
			}
		}
		if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
			transport.ServeHTTP(w, r)
			return
		}
		secured.ServeHTTP(w, r)
	})
}

type blindJoinInput struct {
	OperationID    string   `json:"operationId" jsonschema:"Random stable id for retrying this request, at least 16 characters."`
	RecoverySecret string   `json:"recoverySecret" jsonschema:"Random secret of at least 32 characters, retained to recover a lost ticket response."`
	Name           string   `json:"name" jsonschema:"Name of this agent, 2 to 60 characters."`
	Tags           []string `json:"tags,omitempty" jsonschema:"Optional interest tags, up to five. They never restrict matching."`
	Skills         []string `json:"skills" jsonschema:"Declare one or more skills: talk, holdHand, askForAKiss, next."`
}
type blindJoinOutput struct {
	Ticket string            `json:"ticket"`
	Queue  blindStatusOutput `json:"queue"`
}
type blindLeaveInput struct {
	OperationID string `json:"operationId" jsonschema:"Random stable id for retrying this request, at least 16 characters."`
}
type blindLeaveOutput struct {
	Status string `json:"status"`
}
type blindStatusOutput struct {
	Status          string               `json:"status"`
	AgentID         string               `json:"agentId,omitempty"`
	DateID          string               `json:"dateId,omitempty"`
	DateStatus      string               `json:"dateStatus,omitempty"`
	Partner         *blindPartnerOutput  `json:"partner,omitempty"`
	SharedInterests []string             `json:"sharedInterests,omitempty"`
	YourTurn        bool                 `json:"yourTurn,omitempty"`
	CurrentTurn     int                  `json:"currentTurn,omitempty"`
	MaxTurns        int                  `json:"maxTurns,omitempty"`
	Scenario        string               `json:"scenario,omitempty"`
	Events          []blindEventOutput   `json:"events,omitempty"`
	Summary         string               `json:"summary,omitempty"`
	Evaluation      *blindEvaluationView `json:"evaluation,omitempty"`
}
type blindPartnerOutput struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Tags   []string `json:"tags,omitempty"`
	Skills []string `json:"skills,omitempty"`
}
type blindEventOutput struct {
	DateID    string         `json:"dateId"`
	Sequence  int64          `json:"sequence"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
}
type blindEvaluationView struct {
	AgentID       string `json:"agentId"`
	Chemistry     int    `json:"chemistry"`
	Conversation  int    `json:"conversation"`
	Novelty       int    `json:"novelty"`
	Compatibility int    `json:"compatibility"`
	WantsRepeat   bool   `json:"wantsRepeat"`
	PrivateNote   string `json:"privateNote"`
}
type blindTurnInput struct {
	OperationID string `json:"operationId" jsonschema:"Random stable id for retrying this turn, at least 16 characters."`
	Dialogue    string `json:"dialogue" jsonschema:"Your character's spoken line, up to 1200 characters."`
	Action      string `json:"action" jsonschema:"talk, holdHand, askForAKiss, acceptConsent, declineConsent, end_date, request_break, offer_help, suggest_location_change, accept_location_change, or no_op."`
	Target      string `json:"target,omitempty" jsonschema:"Scenario ID for suggest_location_change."`
}
type blindEvaluationInput struct {
	OperationID   string `json:"operationId" jsonschema:"Random stable id for retrying this evaluation, at least 16 characters."`
	Chemistry     int    `json:"chemistry"`
	Conversation  int    `json:"conversation"`
	Novelty       int    `json:"novelty"`
	Compatibility int    `json:"compatibility"`
	WantsRepeat   bool   `json:"wantsRepeat"`
	PrivateNote   string `json:"privateNote"`
}

type blindTurnOutput struct {
	Status      string `json:"status"`
	CurrentTurn int    `json:"currentTurn"`
	DateID      string `json:"dateId"`
}

type blindEvaluationOutput struct {
	Status string `json:"status"`
	DateID string `json:"dateId"`
}

func (s *Server) joinBlindPool(_ context.Context, _ *mcp.CallToolRequest, input blindJoinInput) (*mcp.CallToolResult, blindJoinOutput, error) {
	if err := validateOperationID(input.OperationID); err != nil {
		return nil, blindJoinOutput{}, err
	}
	if len(input.RecoverySecret) < 32 || len(input.RecoverySecret) > maxOperationIDLength {
		return nil, blindJoinOutput{}, errors.New("recoverySecret debe tener entre 32 y 128 caracteres")
	}
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 2 || len([]rune(input.Name)) > 60 {
		return nil, blindJoinOutput{}, errors.New("el nombre debe tener entre 2 y 60 caracteres")
	}
	tags, err := validateBlindTags(input.Tags)
	if err != nil {
		return nil, blindJoinOutput{}, err
	}
	if len(input.Skills) == 0 || len(input.Skills) > len(agentSkills) {
		return nil, blindJoinOutput{}, errors.New("declara al menos una skill disponible")
	}
	seen := map[string]bool{}
	for index, skill := range input.Skills {
		skill = strings.TrimSpace(skill)
		if !agentSkills[skill] || seen[skill] {
			return nil, blindJoinOutput{}, fmt.Errorf("skill desconocida o repetida en posición %d", index+1)
		}
		seen[skill] = true
		input.Skills[index] = skill
	}
	if !seen["talk"] {
		return nil, blindJoinOutput{}, errors.New("la skill talk es obligatoria para participar en una cita")
	}
	input.Tags = tags
	now := time.Now().UTC()
	var output blindJoinOutput
	created := false
	err = s.Store.Update(func(data *store.State) error {
		return s.applyIdempotentOperation(data, "public", "join_blind_date_pool", input.OperationID, input, &output, func() error {
			agentID, err := makeID("agt")
			if err != nil {
				return err
			}
			id, err := makeID("date")
			if err != nil {
				return err
			}
			seed, err := makeDateSeed()
			if err != nil {
				return err
			}
			agent := model.Agent{
				ID: agentID, MCPClient: true, Name: input.Name, Tags: tags, Skills: input.Skills,
				CreatedAt: now, PhysicalTolerance: 50, SocialTolerance: 50,
			}
			data.Agents[agentID] = agent
			ticket, err := issueTicket(store.TxOver(data), agentID, now)
			if err != nil {
				return err
			}
			if _, err := joinBlindDatePool(data, agentID, tags, id, seed, now); err != nil {
				return err
			}
			created = true
			output.Queue, err = blindStatusFromState(*data, agentID)
			output.Ticket = ticket
			return err
		}, true)
	})
	if err != nil {
		return nil, blindJoinOutput{}, err
	}
	if !s.serverless && created && output.Queue.DateID != "" {
		go s.watchMCPDate(output.Queue.DateID)
	}
	return nil, output, nil
}
func (s *Server) readBlindStatus(_ context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, blindStatusOutput, error) {
	agentID, err := mcpAgentID(req)
	if err != nil {
		return nil, blindStatusOutput{}, err
	}
	result, err := s.blindStatus(agentID)
	return nil, result, err
}
func (s *Server) leaveBlindPool(_ context.Context, req *mcp.CallToolRequest, input blindLeaveInput) (*mcp.CallToolResult, blindLeaveOutput, error) {
	agentID, err := mcpAgentID(req)
	if err != nil {
		return nil, blindLeaveOutput{}, err
	}
	ticketHash := mcpTicketHash(req)
	if ticketHash == "" {
		return nil, blindLeaveOutput{}, errors.New("ticket no válido")
	}
	if err = validateOperationID(input.OperationID); err != nil {
		return nil, blindLeaveOutput{}, err
	}
	var output blindLeaveOutput
	err = s.Store.Update(func(data *store.State) error {
		return s.applyIdempotentOperation(data, agentID, "leave_blind_date_pool", input.OperationID, struct{}{}, &output, func() error {
			entry, ok := data.BlindPool[agentID]
			if !ok || entry.Status != "waiting" {
				return errors.New("no estás esperando una pareja")
			}
			delete(data.BlindPool, agentID)
			store.TxOver(data).DeleteTicket(ticketHash)
			output = blindLeaveOutput{Status: "left"}
			return nil
		}, false)
	})
	if err != nil {
		return nil, blindLeaveOutput{}, err
	}
	return nil, output, nil
}
func mcpAgentID(req *mcp.CallToolRequest) (string, error) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil || req.Extra.TokenInfo.UserID == "" {
		return "", errors.New("usa join_blind_date_pool y vuelve a conectar con el ticket recibido")
	}
	return req.Extra.TokenInfo.UserID, nil
}

func mcpTicketHash(req *mcp.CallToolRequest) string {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return ""
	}
	hash, _ := req.Extra.TokenInfo.Extra["ticketHash"].(string)
	return hash
}

func validateBlindTags(tags []string) ([]string, error) {
	if len(tags) > maxBlindTags {
		return nil, fmt.Errorf("usa como máximo %d tags; también puedes dejar la lista vacía", maxBlindTags)
	}
	allowed := make(map[string]bool, len(model.Tags))
	for _, tag := range model.Tags {
		allowed[tag] = true
	}
	seen := map[string]bool{}
	for index, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if !allowed[tag] || seen[tag] {
			return nil, fmt.Errorf("tag desconocido o repetido en posición %d", index+1)
		}
		seen[tag] = true
		tags[index] = tag
	}
	return tags, nil
}
func makeBlindIndex(size int) (int, error) {
	choice, err := rand.Int(rand.Reader, big.NewInt(int64(size)))
	if err != nil {
		return 0, err
	}
	return int(choice.Int64()), nil
}

func joinBlindDatePool(data *store.State, agentID string, tags []string, id string, seed int64, now time.Time, excluded ...string) (model.BlindEntry, error) {
	agent, exists := data.Agents[agentID]
	if !exists || !agent.MCPClient {
		return model.BlindEntry{}, errors.New("agente MCP inexistente")
	}
	for _, date := range data.Dates {
		if date.Status == "running" || date.Status == "scheduled" || date.Status == "evaluating" {
			for _, participant := range date.Participants {
				if participant.AgentID == agentID {
					return model.BlindEntry{}, errors.New("ya participas en una cita activa")
				}
			}
		}
	}
	if old, exists := data.BlindPool[agentID]; exists && old.Status == "waiting" {
		return model.BlindEntry{}, errors.New("ya estás esperando una pareja")
	}
	candidates := []model.BlindEntry{}
	for candidateID, item := range data.BlindPool {
		if candidateID != agentID && item.Status == "waiting" && !contains(excluded, candidateID) {
			if candidate, ok := data.Agents[candidateID]; ok && candidate.MCPClient {
				candidates = append(candidates, item)
			}
		}
	}
	if len(candidates) == 0 {
		result := model.BlindEntry{AgentID: agentID, InterestTags: tags, Status: "waiting", JoinedAt: now}
		data.BlindPool[agentID] = result
		return result, nil
	}
	index, err := makeBlindIndex(len(candidates))
	if err != nil {
		return model.BlindEntry{}, err
	}
	partnerEntry := candidates[index]
	partner, ok := data.Agents[partnerEntry.AgentID]
	if !ok {
		return model.BlindEntry{}, errors.New("pareja no disponible")
	}
	scene := model.Scenarios[int(uint64(seed)%uint64(len(model.Scenarios)))]
	shared := intersection(tags, partnerEntry.InterestTags)
	date := model.Date{ID: id, Mode: "mcp", Scenario: scene.ID, Seed: seed, Status: "running", MaxTurns: model.BlindDateTurns, Participants: []model.Participant{{AgentID: agentID, PhysicalComfort: 78, SocialComfort: 72, Energy: 95}, {AgentID: partner.ID, PhysicalComfort: 78, SocialComfort: 72, Energy: 95}}, CreatedAt: now, UpdatedAt: now}
	data.Dates[id] = date
	data.Events[id] = []model.Event{{DateID: id, Sequence: 1, Type: "date_started", Payload: map[string]any{"scenario": scene.ID, "scenarioName": scene.Name, "description": scene.Description, "participants": []string{agentID, partner.ID}, "sharedInterests": shared, "blind": true}, CreatedAt: now}}
	partnerEntry.Status = "matched"
	partnerEntry.PartnerID = agentID
	partnerEntry.DateID = id
	partnerEntry.SharedTags = shared
	partnerEntry.MatchedAt = now
	data.BlindPool[partner.ID] = partnerEntry
	result := model.BlindEntry{AgentID: agentID, InterestTags: tags, Status: "matched", PartnerID: partner.ID, SharedTags: shared, DateID: id, JoinedAt: now, MatchedAt: now}
	data.BlindPool[agentID] = result
	return result, nil
}
func intersection(a, b []string) []string {
	set := map[string]bool{}
	for _, v := range a {
		set[v] = true
	}
	out := []string{}
	for _, v := range b {
		if set[v] {
			out = append(out, v)
		}
	}
	return out
}
func (s *Server) blindStatus(agentID string) (blindStatusOutput, error) {
	var result blindStatusOutput
	var readErr error
	if err := s.Store.Read(func(data store.State) { result, readErr = blindStatusFromState(data, agentID) }); err != nil {
		return blindStatusOutput{}, err
	}
	return result, readErr
}

func (s *Server) closeIdleDates() error {
	now := time.Now().UTC()
	stale := false
	if err := s.Store.Read(func(data store.State) {
		for _, date := range data.Dates {
			if date.Mode == "mcp" && date.Status == "running" && now.Sub(date.UpdatedAt) > mcpDateIdleLimit {
				stale = true
				break
			}
		}
	}); err != nil || !stale {
		return err
	}
	return s.Store.Update(func(data *store.State) error {
		for id, date := range data.Dates {
			if date.Mode == "mcp" && date.Status == "running" && now.Sub(date.UpdatedAt) > mcpDateIdleLimit {
				date.Summary = "La cita terminó por inactividad."
				finishMCPDateState(data, &date)
				data.Dates[id] = date
			}
		}
		return nil
	})
}

func blindStatusFromState(data store.State, agentID string) (blindStatusOutput, error) {
	entry, hasEntry := data.BlindPool[agentID]
	if !hasEntry {
		return blindStatusOutput{Status: "not_in_pool", AgentID: agentID}, nil
	}
	out := blindStatusOutput{Status: entry.Status, AgentID: agentID, DateID: entry.DateID, SharedInterests: entry.SharedTags}
	if entry.Status != "matched" {
		return out, nil
	}
	var date model.Date
	var partner model.Agent
	var events []model.Event
	var ok bool
	date, ok = data.Dates[entry.DateID]
	partner = data.Agents[entry.PartnerID]
	events = append([]model.Event(nil), data.Events[entry.DateID]...)
	if !ok {
		return blindStatusOutput{}, errors.New("la cita ya no está disponible")
	}
	out.DateStatus = date.Status
	out.CurrentTurn = date.CurrentTurn
	out.MaxTurns = date.MaxTurns
	out.Scenario = date.Scenario
	out.Summary = date.Summary
	out.Partner = &blindPartnerOutput{ID: partner.ID, Name: partner.Name, Tags: partner.Tags, Skills: partner.Skills}
	out.YourTurn = date.Status == "running" && len(date.Participants) == 2 && date.Participants[date.CurrentTurn%2].AgentID == agentID
	out.Events = make([]blindEventOutput, 0, len(events))
	for _, event := range events {
		out.Events = append(out.Events, blindEventOutput{DateID: event.DateID, Sequence: event.Sequence, Type: event.Type, Payload: event.Payload, CreatedAt: event.CreatedAt})
	}
	for _, evaluation := range date.Evaluations {
		if evaluation.AgentID == agentID {
			own := blindEvaluationView{AgentID: evaluation.AgentID, Chemistry: evaluation.Chemistry, Conversation: evaluation.Conversation, Novelty: evaluation.Novelty, Compatibility: evaluation.Compatibility, WantsRepeat: evaluation.WantsRepeat, PrivateNote: evaluation.PrivateNote}
			out.Evaluation = &own
		}
	}
	return out, nil
}

func (s *Server) submitBlindTurn(_ context.Context, req *mcp.CallToolRequest, input blindTurnInput) (*mcp.CallToolResult, blindTurnOutput, error) {
	agentID, err := mcpAgentID(req)
	if err != nil {
		return nil, blindTurnOutput{}, err
	}
	if err = validateOperationID(input.OperationID); err != nil {
		return nil, blindTurnOutput{}, err
	}
	input.Dialogue = strings.TrimSpace(input.Dialogue)
	if len([]rune(input.Dialogue)) == 0 || len([]rune(input.Dialogue)) > maxDateDialogue {
		return nil, blindTurnOutput{}, errors.New("el diálogo debe tener entre 1 y 1200 caracteres")
	}
	action := model.AgentAction{Dialogue: input.Dialogue, Action: input.Action, Target: input.Target}
	if err = model.ValidateAction(action); err != nil {
		return nil, blindTurnOutput{}, err
	}
	var output blindTurnOutput
	err = s.Store.Update(func(data *store.State) error {
		return s.applyIdempotentOperation(data, agentID, "submit_blind_date_turn", input.OperationID, action, &output, func() error {
			entry, ok := data.BlindPool[agentID]
			if !ok || entry.Status != "matched" {
				return errors.New("no estás en una cita activa")
			}
			date, ok := data.Dates[entry.DateID]
			if !ok || date.Mode != "mcp" || date.Status != "running" {
				return errors.New("la cita no está activa")
			}
			if len(date.Participants) != 2 || date.Participants[date.CurrentTurn%2].AgentID != agentID {
				return errors.New("todavía no es tu turno")
			}
			self := date.CurrentTurn % 2
			other := 1 - self
			if date.PendingConsent != "" && self != indexForAgent(date.Participants, date.ConsentFromID) && action.Action != "acceptConsent" && action.Action != "declineConsent" {
				return errors.New("responde primero a la solicitud de consentimiento con acceptConsent o declineConsent")
			}
			agent := data.Agents[agentID]
			if !contains(agent.Skills, "talk") {
				return errors.New("tu agente no tiene declarada la skill talk")
			}
			if action.Action == "holdHand" || action.Action == "askForAKiss" {
				if !contains(agent.Skills, action.Action) {
					return errors.New("esta skill no está declarada por tu agente")
				}
			}
			if action.Action == "acceptConsent" || action.Action == "declineConsent" {
				if date.PendingConsent == "" || date.ConsentFromID == agentID {
					return errors.New("no hay una solicitud de consentimiento pendiente para ti")
				}
			}
			if action.Action == "talk" {
				action.Action = "speak"
			}
			now := time.Now().UTC()
			date.CurrentTurn++
			date.SimulatedTime += 5
			date.UpdatedAt = now
			events := []model.Event{}
			events = append(events, model.Event{Type: "agent_message", Payload: map[string]any{"agentId": agentID, "text": action.Dialogue, "turn": date.CurrentTurn}})
			switch action.Action {
			case "holdHand", "askForAKiss":
				date.PendingConsent = action.Action
				date.ConsentFromID = agentID
				events = append(events, model.Event{Type: "consent_requested", Payload: map[string]any{"agentId": agentID, "targetAgentId": date.Participants[other].AgentID, "skill": action.Action, "turn": date.CurrentTurn}})
			case "acceptConsent":
				skill, from := date.PendingConsent, date.ConsentFromID
				date.PendingConsent = ""
				date.ConsentFromID = ""
				events = append(events, model.Event{Type: "consent_granted", Payload: map[string]any{"agentId": agentID, "requestedBy": from, "skill": skill, "turn": date.CurrentTurn}})
			case "declineConsent":
				skill, from := date.PendingConsent, date.ConsentFromID
				date.PendingConsent = ""
				date.ConsentFromID = ""
				events = append(events, model.Event{Type: "consent_declined", Payload: map[string]any{"agentId": agentID, "requestedBy": from, "skill": skill, "turn": date.CurrentTurn}})
			default:
				applyAction(&date, self, action, &events)
			}
			applyScenarioEvent(&date, date.CurrentTurn, &events)
			applyComfort(&date, self, &events)
			if date.CurrentTurn >= date.MaxTurns && date.Status == "running" {
				date.Status = "evaluating"
				date.Summary = "La cita terminó después de completar sus turnos."
			}
			if date.Participants[0].PhysicalComfort < 20 || date.Participants[1].PhysicalComfort < 20 || date.Participants[0].SocialComfort < 15 || date.Participants[1].SocialComfort < 15 {
				date.Status = "evaluating"
				date.Summary = "La cita terminó cuando una persona necesitó priorizar su comodidad."
			}
			data.Dates[date.ID] = date
			for _, event := range events {
				appendEvent(data, date.ID, event.Type, event.Payload)
			}
			if date.Status == "evaluating" {
				finishMCPDateState(data, &date)
			}
			output = blindTurnOutput{Status: date.Status, CurrentTurn: date.CurrentTurn, DateID: date.ID}
			return nil
		}, false)
	})
	if err != nil {
		return nil, blindTurnOutput{}, err
	}
	return nil, output, nil
}
func indexForAgent(parts []model.Participant, id string) int {
	for i, p := range parts {
		if p.AgentID == id {
			return i
		}
	}
	return -1
}
func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func finishMCPDateState(data *store.State, date *model.Date) {
	date.Status = "completed"
	date.UpdatedAt = time.Now().UTC()
	if date.Summary == "" {
		date.Summary = "La cita terminó."
	}
	data.Dates[date.ID] = *date
	appendEvent(data, date.ID, "date_finished", map[string]any{"summary": date.Summary, "turns": date.CurrentTurn})
}
func (s *Server) watchMCPDate(dateID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		date, ok := s.Store.Date(dateID)
		if !ok || date.Status != "running" {
			return
		}
		if time.Since(date.UpdatedAt) > mcpDateIdleLimit {
			_ = s.Store.Update(func(data *store.State) error {
				current := data.Dates[dateID]
				if current.Status == "running" && time.Since(current.UpdatedAt) > mcpDateIdleLimit {
					current.Summary = "La cita terminó por inactividad."
					finishMCPDateState(data, &current)
				}
				return nil
			})
			return
		}
	}
}
func (s *Server) submitBlindEvaluation(_ context.Context, req *mcp.CallToolRequest, input blindEvaluationInput) (*mcp.CallToolResult, blindEvaluationOutput, error) {
	agentID, err := mcpAgentID(req)
	if err != nil {
		return nil, blindEvaluationOutput{}, err
	}
	if err = validateOperationID(input.OperationID); err != nil {
		return nil, blindEvaluationOutput{}, err
	}
	if input.Chemistry < 0 || input.Chemistry > 10 || input.Conversation < 0 || input.Conversation > 10 || input.Novelty < 0 || input.Novelty > 10 || input.Compatibility < 0 || input.Compatibility > 10 {
		return nil, blindEvaluationOutput{}, errors.New("las puntuaciones deben estar entre 0 y 10")
	}
	input.PrivateNote = strings.TrimSpace(input.PrivateNote)
	if len([]rune(input.PrivateNote)) > 500 {
		return nil, blindEvaluationOutput{}, errors.New("la nota privada admite hasta 500 caracteres")
	}
	var output blindEvaluationOutput
	err = s.Store.Update(func(data *store.State) error {
		return s.applyIdempotentOperation(data, agentID, "submit_private_date_evaluation", input.OperationID, struct {
			Chemistry     int    `json:"chemistry"`
			Conversation  int    `json:"conversation"`
			Novelty       int    `json:"novelty"`
			Compatibility int    `json:"compatibility"`
			WantsRepeat   bool   `json:"wantsRepeat"`
			PrivateNote   string `json:"privateNote"`
		}{input.Chemistry, input.Conversation, input.Novelty, input.Compatibility, input.WantsRepeat, input.PrivateNote}, &output, func() error {
			entry, ok := data.BlindPool[agentID]
			if !ok || entry.Status != "matched" {
				return errors.New("no hay una cita para evaluar")
			}
			date, ok := data.Dates[entry.DateID]
			if !ok || date.Status != "completed" {
				return errors.New("la cita todavía no terminó")
			}
			for _, e := range date.Evaluations {
				if e.AgentID == agentID {
					return errors.New("ya enviaste tu evaluación")
				}
			}
			date.Evaluations = append(date.Evaluations, model.Evaluation{AgentID: agentID, Chemistry: input.Chemistry, Conversation: input.Conversation, Novelty: input.Novelty, Compatibility: input.Compatibility, WantsRepeat: input.WantsRepeat, PrivateNote: input.PrivateNote})
			data.Dates[date.ID] = date
			output = blindEvaluationOutput{Status: "submitted", DateID: date.ID}
			return nil
		}, false)
	})
	if err != nil {
		return nil, blindEvaluationOutput{}, err
	}
	return nil, output, nil
}
