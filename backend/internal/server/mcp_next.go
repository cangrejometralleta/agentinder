package server

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/cangrejometralleta/agentinder/backend/internal/model"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// skillNext lets an agent leave its current date and meet someone else.
const skillNext = "next"

type blindNextInput struct {
	OperationID string   `json:"operationId" jsonschema:"Random stable id for retrying this request, at least 16 characters."`
	Interests   []string `json:"interests,omitempty" jsonschema:"Optional interest tags for the next date; they do not restrict matching."`
}

func makeDateSeed() (int64, error) {
	seedBytes := make([]byte, 8)
	if _, err := rand.Read(seedBytes); err != nil {
		return 0, err
	}
	seed := int64(0)
	for _, b := range seedBytes {
		seed = seed*256 + int64(b)
	}
	return seed, nil
}

// nextBlindDateState ends the agent's current date, if it is still open,
// and queues the agent again without matching the partner it just left.
func nextBlindDateState(data *store.State, agentID string, tags []string, id string, seed int64, now time.Time) (model.BlindEntry, error) {
	agent, exists := data.Agents[agentID]
	if !exists || !agent.MCPClient {
		return model.BlindEntry{}, errors.New("agente MCP inexistente")
	}
	if !contains(agent.Skills, skillNext) {
		return model.BlindEntry{}, errors.New("tu agente no declaró la skill next")
	}
	entry, inPool := data.BlindPool[agentID]
	if !inPool {
		return model.BlindEntry{}, errors.New("no estás en una cita; entra primero con join_blind_date_pool")
	}
	if entry.Status == "waiting" {
		return model.BlindEntry{}, errors.New("todavía no tienes pareja; no hay cita que saltar")
	}
	if date, ok := data.Dates[entry.DateID]; ok && date.Status != "completed" {
		date.Summary = "La cita terminó porque una persona pasó a la siguiente."
		finishMCPDateState(data, &date)
	}
	return joinBlindDatePool(data, agentID, tags, id, seed, now, entry.PartnerID)
}

func (s *Server) nextBlindDate(_ context.Context, req *mcp.CallToolRequest, input blindNextInput) (*mcp.CallToolResult, blindStatusOutput, error) {
	agentID, err := mcpAgentID(req)
	if err != nil {
		return nil, blindStatusOutput{}, err
	}
	if err = validateOperationID(input.OperationID); err != nil {
		return nil, blindStatusOutput{}, err
	}
	tags, err := validateBlindTags(input.Interests)
	if err != nil {
		return nil, blindStatusOutput{}, err
	}
	var result blindStatusOutput
	created := false
	err = s.Store.Update(func(data *store.State) error {
		return s.applyIdempotentOperation(data, agentID, "next_blind_date", input.OperationID, struct {
			Interests []string `json:"interests"`
		}{tags}, &result, func() error {
			id, err := makeID("date")
			if err != nil {
				return err
			}
			seed, err := makeDateSeed()
			if err != nil {
				return err
			}
			if _, err = nextBlindDateState(data, agentID, tags, id, seed, time.Now().UTC()); err != nil {
				return err
			}
			created = true
			result, err = blindStatusFromState(*data, agentID)
			return err
		}, false)
	})
	if err != nil {
		return nil, blindStatusOutput{}, err
	}
	if !s.serverless && created && result.DateID != "" && result.Status == "matched" {
		go s.watchMCPDate(result.DateID)
	}
	return nil, result, nil
}
