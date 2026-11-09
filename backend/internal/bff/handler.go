package bff

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const datesPath = "/api/v1/public/dates"

type Handler struct {
	apiURL     string
	token      string
	webOrigins map[string]bool
	client     *http.Client
}

func NewHandler(apiURL, token, webOrigin string) *Handler {
	origins := map[string]bool{}
	for _, origin := range strings.Split(webOrigin, ",") {
		origins[strings.TrimSpace(origin)] = true
	}
	return &Handler{
		apiURL: strings.TrimRight(apiURL, "/"), token: token, webOrigins: origins,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.URL.Path == "/healthz" {
		writeJSON(w, http.StatusOK, `{"status":"ok","service":"agentinder-bff"}`)
		return
	}
	if r.URL.Path != datesPath {
		http.NotFound(w, r)
		return
	}
	if !h.allowOrigin(w, r) {
		writeJSON(w, http.StatusForbidden, `{"error":"origin not allowed"}`)
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, OPTIONS")
		writeJSON(w, http.StatusMethodNotAllowed, `{"error":"method not allowed"}`)
		return
	}
	h.readDates(w, r)
}

func (h *Handler) allowOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		referrer, err := url.Parse(r.Referer())
		if err != nil || r.Header.Get("Sec-Fetch-Site") != "same-origin" {
			return false
		}
		origin = referrer.Scheme + "://" + referrer.Host
	}
	if !h.webOrigins[origin] {
		return false
	}
	if r.Header.Get("Origin") != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Add("Vary", "Origin")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	return true
}

func (h *Handler) readDates(w http.ResponseWriter, r *http.Request) {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.apiURL+datesPath, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, `{"error":"upstream unavailable"}`)
		return
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+h.token)
	response, err := h.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, `{"error":"upstream unavailable"}`)
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	if _, err := io.Copy(w, response.Body); err != nil {
		return
	}
}

func ValidateSettings(apiURL, token, webOrigin string) error {
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("AGENTINDER_API_URL must be an HTTPS base URL")
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("AGENTINDER_WEB_API_TOKEN is required")
	}
	for _, value := range strings.Split(webOrigin, ",") {
		origin, err := url.Parse(strings.TrimSpace(value))
		if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil ||
			origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
			return fmt.Errorf("WEB_ORIGINS must contain HTTPS origins")
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
