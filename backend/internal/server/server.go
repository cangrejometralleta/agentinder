package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"agentinder/backend/internal/keyring"
	"agentinder/backend/internal/store"
)

const bodyLimit = 1 << 20

type Server struct {
	Store      *store.Store
	Repository store.Repository
	WebOrigin  string
	tokens     *keyring.Keyring
}

func New(data *store.Store, webOrigin string, tokens *keyring.Keyring) *Server {
	return &Server{Store: data, Repository: store.NewFileRepository(data), WebOrigin: webOrigin, tokens: tokens}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /api/v1/public/dates", s.publicObservatoryDates)
	mux.Handle("/mcp", s.mcpHandler())
	return s.cors(mux)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == s.WebOrigin && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Protocol-Version, Mcp-Session-Id, Mcp-Method")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "agentinder-api"})
}

func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if err := s.Store.CheckWritable(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func makeID(prefix string) (string, error) {
	randomBytes := make([]byte, 18)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "el cuerpo JSON no es válido")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "envía un solo objeto JSON")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeFailure(w http.ResponseWriter, _ error) {
	writeError(w, http.StatusInternalServerError, "no se pudo guardar la solicitud; vuelve a intentarlo")
}
