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

	"agentinder/backend/internal/keyring"
	"agentinder/backend/internal/store"
)

type toolResult struct {
	status     int
	structured map[string]any
	failed     bool
	text       string
}

func (r toolResult) str(key string) string { s, _ := r.structured[key].(string); return s }

func ringOf(t *testing.T, current byte, retired ...byte) *keyring.Keyring {
	t.Helper()
	old := [][]byte{}
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
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": arguments}})
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

func registration(name string) map[string]any {
	return map[string]any{"operationId": freshOp(), "recoverySecret": strings.Repeat("s", 40), "name": name, "tags": []string{"música"}, "skills": []string{"talk"}}
}

func connect(t *testing.T, handler http.Handler, name string) toolResult {
	t.Helper()
	result := callTool(t, handler, "", "connect_agent", registration(name))
	if result.failed || result.str("token") == "" {
		t.Fatalf("connect_agent failed: %+v", result)
	}
	return result
}

func TestConnectAgentReplaysTheSameBearer(t *testing.T) {
	handler, _ := newMCP(t)
	arguments := registration("Ada")
	first := callTool(t, handler, "", "connect_agent", arguments)
	again := callTool(t, handler, "", "connect_agent", arguments)
	if first.failed || again.failed {
		t.Fatalf("unexpected failure: %+v %+v", first, again)
	}
	if first.str("agentId") != again.str("agentId") || first.str("token") != again.str("token") || first.str("tokenId") != again.str("tokenId") {
		t.Errorf("a retry must return the original registration: %+v vs %+v", first.structured, again.structured)
	}
	other := callTool(t, handler, "", "connect_agent", registration("Ada"))
	if other.str("agentId") == first.str("agentId") || other.str("token") == first.str("token") {
		t.Error("a new operationId is a new registration")
	}
}

func TestConnectAgentRejectsReusedOperationWithOtherArguments(t *testing.T) {
	handler, _ := newMCP(t)
	arguments := registration("Ada")
	callTool(t, handler, "", "connect_agent", arguments)
	arguments["name"] = "Otra"
	result := callTool(t, handler, "", "connect_agent", arguments)
	if !result.failed || !strings.Contains(result.text, "otros argumentos") {
		t.Errorf("want an idempotency conflict, got %+v", result)
	}
}

func TestConnectAgentValidatesInput(t *testing.T) {
	handler, _ := newMCP(t)
	cases := map[string]func(map[string]any){
		"recovery secret too short": func(a map[string]any) { a["recoverySecret"] = "corta" },
		"name too short":            func(a map[string]any) { a["name"] = "A" },
		"unknown tag":               func(a map[string]any) { a["tags"] = []string{"paz"} },
		"unknown skill":             func(a map[string]any) { a["skills"] = []string{"talk", "volar"} },
		"repeated skill":            func(a map[string]any) { a["skills"] = []string{"talk", "talk"} },
		"talk missing":              func(a map[string]any) { a["skills"] = []string{"holdHand"} },
		"operation id too short":    func(a map[string]any) { a["operationId"] = "corto" },
	}
	for name, change := range cases {
		arguments := registration("Ada")
		change(arguments)
		if result := callTool(t, handler, "", "connect_agent", arguments); !result.failed {
			t.Errorf("%s: want a failure, got %+v", name, result.structured)
		}
	}
}

func TestBearerAuthentication(t *testing.T) {
	handler, _ := newMCP(t)
	agent := connect(t, handler, "Ada")
	if status := callTool(t, handler, agent.str("token"), "blind_date_status", map[string]any{}); status.failed || status.str("status") != "not_in_pool" || status.str("agentId") != agent.str("agentId") {
		t.Errorf("a valid bearer identifies its agent: %+v", status)
	}
	if status := callTool(t, handler, "agt_inventado", "blind_date_status", map[string]any{}); status.status != http.StatusUnauthorized {
		t.Errorf("an invented bearer must be unauthorized, got HTTP %d", status.status)
	}
	if status := callTool(t, handler, "", "blind_date_status", map[string]any{}); !status.failed {
		t.Errorf("an authenticated tool without a bearer must fail: %+v", status)
	}
}

func TestRotationKeepsTheOldTokenUntilRevoked(t *testing.T) {
	handler, _ := newMCP(t)
	first := connect(t, handler, "Ada")
	rotation := map[string]any{"operationId": freshOp()}
	second := callTool(t, handler, first.str("token"), "rotate_agent_token", rotation)
	replay := callTool(t, handler, first.str("token"), "rotate_agent_token", rotation)
	if second.failed || second.str("token") == "" || second.str("token") == first.str("token") {
		t.Fatalf("rotation must issue another bearer: %+v", second)
	}
	if replay.str("token") != second.str("token") || replay.str("tokenId") != second.str("tokenId") {
		t.Error("retrying a rotation must return the same bearer")
	}
	for name, token := range map[string]string{"old": first.str("token"), "new": second.str("token")} {
		if got := callTool(t, handler, token, "blind_date_status", map[string]any{}); got.failed || got.str("agentId") != first.str("agentId") {
			t.Errorf("the %s token must belong to the same agent: %+v", name, got)
		}
	}
}

func TestRevocation(t *testing.T) {
	handler, _ := newMCP(t)
	first := connect(t, handler, "Ada")
	second := callTool(t, handler, first.str("token"), "rotate_agent_token", map[string]any{"operationId": freshOp()})

	self := callTool(t, handler, second.str("token"), "revoke_agent_token", map[string]any{"operationId": freshOp(), "tokenId": second.str("tokenId")})
	if !self.failed || !strings.Contains(self.text, "no puedes revocar el token de esta conexión") {
		t.Errorf("a connection cannot revoke itself: %+v", self)
	}
	unknown := callTool(t, handler, second.str("token"), "revoke_agent_token", map[string]any{"operationId": freshOp(), "tokenId": strings.Repeat("z", 16)})
	if !unknown.failed || !strings.Contains(unknown.text, "no encontrado") {
		t.Errorf("an unknown tokenId must fail: %+v", unknown)
	}
	malformed := callTool(t, handler, second.str("token"), "revoke_agent_token", map[string]any{"operationId": freshOp(), "tokenId": "corto"})
	if !malformed.failed {
		t.Error("a malformed tokenId must fail")
	}

	revoke := map[string]any{"operationId": freshOp(), "tokenId": first.str("tokenId")}
	done := callTool(t, handler, second.str("token"), "revoke_agent_token", revoke)
	again := callTool(t, handler, second.str("token"), "revoke_agent_token", revoke)
	if done.failed || done.str("status") != "revoked" || again.failed || again.str("status") != "revoked" {
		t.Errorf("revoking, and retrying the same operation, must succeed: %+v %+v", done, again)
	}
	if got := callTool(t, handler, first.str("token"), "blind_date_status", map[string]any{}); got.status != http.StatusUnauthorized {
		t.Errorf("a revoked token must be unauthorized, got HTTP %d", got.status)
	}
	if got := callTool(t, handler, second.str("token"), "blind_date_status", map[string]any{}); got.failed {
		t.Error("the remaining token keeps working")
	}
}

func TestAnAgentCannotRevokeAnotherAgentsToken(t *testing.T) {
	handler, _ := newMCP(t)
	ada := connect(t, handler, "Ada")
	beto := connect(t, handler, "Beto")
	result := callTool(t, handler, beto.str("token"), "revoke_agent_token", map[string]any{"operationId": freshOp(), "tokenId": ada.str("tokenId")})
	if !result.failed {
		t.Errorf("revoking someone else's token must fail: %+v", result)
	}
	if got := callTool(t, handler, ada.str("token"), "blind_date_status", map[string]any{}); got.failed {
		t.Error("the other agent's token must keep working")
	}
}

func TestActiveTokenLimit(t *testing.T) {
	handler, _ := newMCP(t)
	agent := connect(t, handler, "Ada")
	for index := 1; index < maxAgentTokens; index++ {
		if got := callTool(t, handler, agent.str("token"), "rotate_agent_token", map[string]any{"operationId": freshOp()}); got.failed {
			t.Fatalf("rotation %d should fit under the limit: %+v", index, got)
		}
	}
	if got := callTool(t, handler, agent.str("token"), "rotate_agent_token", map[string]any{"operationId": freshOp()}); !got.failed {
		t.Errorf("rotating past %d tokens must fail", maxAgentTokens)
	}
}

func TestStateFileNeverHoldsBearersInTheClear(t *testing.T) {
	handler, path := newMCP(t)
	agent := connect(t, handler, "Ada")
	rotated := callTool(t, handler, agent.str("token"), "rotate_agent_token", map[string]any{"operationId": freshOp()})
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{agent.str("token"), rotated.str("token")} {
		if bytes.Contains(saved, []byte(secret)) {
			t.Error("a bearer was written to the state file in the clear")
		}
	}
}

func TestReplaySurvivesKeyRotationUntilTheOldKeyIsDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	arguments := registration("Ada")
	original := callTool(t, mcpServerAt(t, path, ringOf(t, 1)), "", "connect_agent", arguments)

	rotated := mcpServerAt(t, path, ringOf(t, 2, 1))
	if got := callTool(t, rotated, "", "connect_agent", arguments); got.failed || got.str("token") != original.str("token") {
		t.Errorf("with the old key retired, the replay must still work: %+v", got)
	}
	dropped := mcpServerAt(t, path, ringOf(t, 2))
	if got := callTool(t, dropped, "", "connect_agent", arguments); !got.failed {
		t.Error("without the old key the sealed response cannot be recovered")
	}
	if got := callTool(t, dropped, original.str("token"), "blind_date_status", map[string]any{}); got.failed {
		t.Error("the bearer itself must keep authenticating after the key is dropped")
	}
}
