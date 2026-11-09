package bff

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestBFFKeepsWebTokenUpstream(t *testing.T) {
	const token = "server-only-token"
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != datesPath || r.Method != http.MethodGet {
			t.Errorf("upstream request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("upstream Authorization = %q", r.Header.Get("Authorization"))
		}
		return response(http.StatusOK, `[]`), nil
	})}
	handler := NewHandler("https://api.example", token, "https://agentinder.dev")
	handler.client = client
	request := httptest.NewRequest(http.MethodGet, datesPath, nil)
	request.Header.Set("Origin", "https://agentinder.dev")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), token) {
		t.Fatal("server token appeared in the browser response")
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "https://agentinder.dev" {
		t.Errorf("CORS origin = %q", response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestBFFRejectsOtherOrigins(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return response(http.StatusOK, `[]`), nil
	})}
	handler := NewHandler("https://api.example", "server-only-token", "https://agentinder.dev")
	handler.client = client
	request := httptest.NewRequest(http.MethodGet, datesPath, nil)
	request.Header.Set("Origin", "https://other.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d upstream called=%t", response.Code, called)
	}
}

func TestBFFRequiresOrigin(t *testing.T) {
	handler := NewHandler("https://api.example", "server-only-token", "https://agentinder.dev")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, datesPath, nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestBFFAcceptsSameOriginHostingFetch(t *testing.T) {
	handler := NewHandler("https://api.example", "server-only-token", "https://agentinder.app,https://agentinder.web.app")
	handler.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `[]`), nil
	})}
	for _, site := range []string{"https://agentinder.app", "https://agentinder.web.app", "https://other.example"} {
		request := httptest.NewRequest(http.MethodGet, datesPath, nil)
		request.Header.Set("Referer", site+"/")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		want := http.StatusOK
		if site == "https://other.example" {
			want = http.StatusForbidden
		}
		if recorder.Code != want {
			t.Fatalf("site=%s status=%d want=%d", site, recorder.Code, want)
		}
		if recorder.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("BFF responses must not be cached by Hosting")
		}
	}
}

func TestBFFHandlesPreflightWithoutCallingAPI(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return response(http.StatusOK, `[]`), nil
	})}
	handler := NewHandler("https://api.example", "server-only-token", "https://agentinder.dev")
	handler.client = client
	request := httptest.NewRequest(http.MethodOptions, datesPath, nil)
	request.Header.Set("Origin", "https://agentinder.dev")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || called {
		t.Fatalf("status=%d upstream called=%t", response.Code, called)
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestBFFValidatesServerSettings(t *testing.T) {
	for _, test := range []struct {
		name, apiURL, token, origin string
	}{
		{name: "valid", apiURL: "https://api.example", token: "secret", origin: "https://agentinder.dev"},
		{name: "api must use https", apiURL: "http://api.example", token: "secret", origin: "https://agentinder.dev"},
		{name: "token required", apiURL: "https://api.example", origin: "https://agentinder.dev"},
		{name: "origin must be an origin", apiURL: "https://api.example", token: "secret", origin: "https://agentinder.dev/path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateSettings(test.apiURL, test.token, test.origin)
			if (test.name == "valid") != (err == nil) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
