package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangrejometralleta/agentinder/backend/internal/keyring"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

type toolResult struct {
	status     int
	structured map[string]any
	failed     bool
	text       string
}

func (r toolResult) str(key string) string {
	value, _ := r.structured[key].(string)
	return value
}

func ringOf(t *testing.T, current byte, retired ...byte) *keyring.Keyring {
	t.Helper()
	old := make([][]byte, 0, len(retired))
	for _, fill := range retired {
		old = append(old, bytes.Repeat([]byte{fill}, keyring.KeySize))
	}
	ring, err := keyring.New(bytes.Repeat([]byte{current}, keyring.KeySize), old...)
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func mcpServerAt(t *testing.T, path string, ring *keyring.Keyring) http.Handler {
	t.Helper()
	opened, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return New(opened, "", ring).Handler()
}

func newMCP(t *testing.T) (http.Handler, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	return mcpServerAt(t, path, ringOf(t, 1)), path
}

func callTool(t *testing.T, handler http.Handler, bearer, tool string, arguments map[string]any) toolResult {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": arguments},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	result := toolResult{status: recorder.Code}
	if recorder.Code != http.StatusOK {
		return result
	}
	payload := recorder.Body.String()
	for _, line := range strings.Split(payload, "\n") {
		if strings.HasPrefix(line, "data:") {
			payload = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			break
		}
	}
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			IsError           bool           `json:"isError"`
			StructuredContent map[string]any `json:"structuredContent"`
			Content           []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatalf("unreadable MCP response %q: %v", payload, err)
	}
	if envelope.Error != nil {
		result.failed = true
		result.text = envelope.Error.Message
		return result
	}
	result.failed = envelope.Result.IsError
	result.structured = envelope.Result.StructuredContent
	if len(envelope.Result.Content) > 0 {
		result.text = envelope.Result.Content[0].Text
	}
	return result
}

var opCounter int

func freshOp() string {
	opCounter++
	return strings.Repeat("o", 12) + string(rune('a'+opCounter%26)) + strings.Repeat("0", 3) + strings.Repeat("x", opCounter%7) + string(rune('A'+(opCounter/26)%26))
}

func ticketJoin(name string) map[string]any {
	return map[string]any{
		"operationId":    freshOp(),
		"recoverySecret": strings.Repeat("r", 40),
		"name":           name,
		"tags":           []string{"música"},
		"skills":         []string{"talk", "next"},
	}
}

func TestPublicJoinReturnsRecoverableTicket(t *testing.T) {
	handler, path := newMCP(t)
	arguments := ticketJoin("Ada")
	first := callTool(t, handler, "", "join_blind_date_pool", arguments)
	replay := callTool(t, handler, "", "join_blind_date_pool", arguments)
	if first.failed || replay.failed {
		t.Fatalf("join or replay failed: %+v %+v", first, replay)
	}
	if first.str("ticket") == "" || first.str("ticket") != replay.str("ticket") {
		t.Errorf("retry must recover the same ticket: %+v %+v", first.structured, replay.structured)
	}
	firstQueue, _ := first.structured["queue"].(map[string]any)
	replayQueue, _ := replay.structured["queue"].(map[string]any)
	if firstQueue["agentId"] == "" || firstQueue["agentId"] != replayQueue["agentId"] {
		t.Errorf("retry must recover the same profile: %+v %+v", firstQueue, replayQueue)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), first.str("ticket")) {
		t.Error("ticket secret was written to state")
	}
	if !strings.Contains(string(saved), digestTicket(first.str("ticket"))) {
		t.Error("ticket hash was not written to state")
	}
}

func TestPublicJoinTicketAuthenticatesPoolTools(t *testing.T) {
	handler, _ := newMCP(t)
	joined := callTool(t, handler, "", "join_blind_date_pool", ticketJoin("Ada"))
	if joined.failed || joined.str("ticket") == "" {
		t.Fatalf("join failed: %+v", joined)
	}
	status := callTool(t, handler, joined.str("ticket"), "blind_date_status", map[string]any{})
	if status.failed || status.str("agentId") == "" || status.str("status") != "waiting" {
		t.Errorf("ticket did not authenticate the new agent: %+v", status)
	}
	if got := callTool(t, handler, "ticket_inventado", "blind_date_status", map[string]any{}); got.status != http.StatusUnauthorized {
		t.Errorf("invented ticket must be unauthorized, got %d", got.status)
	}
	if got := callTool(t, handler, "", "blind_date_status", map[string]any{}); !got.failed {
		t.Error("pool tools still require authentication")
	}
}

func TestPublicJoinRejectsInvalidProfile(t *testing.T) {
	handler, _ := newMCP(t)
	cases := map[string]func(map[string]any){
		"short recovery secret": func(input map[string]any) { input["recoverySecret"] = "short" },
		"short name":            func(input map[string]any) { input["name"] = "A" },
		"unknown tag":           func(input map[string]any) { input["tags"] = []string{"paz"} },
		"unknown skill":         func(input map[string]any) { input["skills"] = []string{"talk", "fly"} },
		"talk missing":          func(input map[string]any) { input["skills"] = []string{"next"} },
	}
	for name, change := range cases {
		input := ticketJoin("Ada")
		change(input)
		if result := callTool(t, handler, "", "join_blind_date_pool", input); !result.failed {
			t.Errorf("%s: want failure, got %+v", name, result.structured)
		}
	}
}

func TestPublicJoinOperationConflictsOnChangedArguments(t *testing.T) {
	handler, _ := newMCP(t)
	input := ticketJoin("Ada")
	if result := callTool(t, handler, "", "join_blind_date_pool", input); result.failed {
		t.Fatalf("initial join failed: %+v", result)
	}
	input["name"] = "Beto"
	if result := callTool(t, handler, "", "join_blind_date_pool", input); !result.failed || !strings.Contains(result.text, "otros argumentos") {
		t.Errorf("changed retry arguments must conflict: %+v", result)
	}
}

func TestLegacyTokenToolsAreRemoved(t *testing.T) {
	handler, _ := newMCP(t)
	for _, name := range []string{"connect_agent", "rotate_agent_token", "revoke_agent_token"} {
		if result := callTool(t, handler, "", name, map[string]any{}); !result.failed {
			t.Errorf("legacy tool %q is still registered", name)
		}
	}
}

func TestLeavingPoolRevokesTicket(t *testing.T) {
	handler, path := newMCP(t)
	joined := callTool(t, handler, "", "join_blind_date_pool", ticketJoin("Ada"))
	if joined.failed {
		t.Fatalf("join failed: %+v", joined)
	}
	left := callTool(t, handler, joined.str("ticket"), "leave_blind_date_pool", map[string]any{"operationId": freshOp()})
	if left.failed || left.str("status") != "left" {
		t.Fatalf("leave failed: %+v", left)
	}
	if status := callTool(t, handler, joined.str("ticket"), "blind_date_status", map[string]any{}); status.status != http.StatusUnauthorized {
		t.Errorf("leaving the pool must revoke the ticket, got HTTP %d", status.status)
	}
	data, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.NewFileRepository(data).Read(func(reader store.Reader) {
		if len(reader.Tickets()) != 0 {
			t.Errorf("ticket remains in storage: %+v", reader.Tickets())
		}
		queue, _ := joined.structured["queue"].(map[string]any)
		agentID, _ := queue["agentId"].(string)
		if _, ok := reader.BlindEntry(agentID); ok {
			t.Error("left agent remains in the queue")
		}
	})
}

func TestJoinReplaySurvivesKeyRotationUntilTheOldKeyIsDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	arguments := ticketJoin("Ada")
	original := callTool(t, mcpServerAt(t, path, ringOf(t, 1)), "", "join_blind_date_pool", arguments)
	if original.failed {
		t.Fatalf("initial join failed: %+v", original)
	}

	rotated := mcpServerAt(t, path, ringOf(t, 2, 1))
	if replay := callTool(t, rotated, "", "join_blind_date_pool", arguments); replay.failed || replay.str("ticket") != original.str("ticket") {
		t.Errorf("old key must recover the same ticket: %+v", replay)
	}
	dropped := mcpServerAt(t, path, ringOf(t, 2))
	if replay := callTool(t, dropped, "", "join_blind_date_pool", arguments); !replay.failed {
		t.Error("dropping the old key must prevent replaying the protected response")
	}
	if status := callTool(t, dropped, original.str("ticket"), "blind_date_status", map[string]any{}); status.failed {
		t.Errorf("ticket authentication must not depend on the response key: %+v", status)
	}
}
