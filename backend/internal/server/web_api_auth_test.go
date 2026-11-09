package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

func TestPublicHTTPAPIRequiresServerBearer(t *testing.T) {
	data, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	api := New(data, "https://agentinder.dev", ringOf(t, 1))
	api.UseWebAPIToken("server-only-token")
	handler := api.Handler()

	for _, test := range []struct {
		name   string
		token  string
		status int
	}{
		{name: "missing", status: http.StatusUnauthorized},
		{name: "wrong", token: "wrong-token", status: http.StatusUnauthorized},
		{name: "server token", token: "server-only-token", status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/public/dates", nil)
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d", response.Code, test.status)
			}
		})
	}
}

func TestMCPStaysPublicWhenHTTPAPIRequiresToken(t *testing.T) {
	data, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	api := New(data, "https://agentinder.dev", ringOf(t, 1))
	api.UseWebAPIToken("server-only-token")
	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code == http.StatusUnauthorized {
		t.Fatal("MCP was protected by the web API token")
	}
}
