package agentinder

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/cangrejometralleta/agentinder/backend/internal/bff"
	"github.com/cangrejometralleta/agentinder/backend/internal/keyring"
	"github.com/cangrejometralleta/agentinder/backend/internal/server"
	"github.com/cangrejometralleta/agentinder/backend/internal/store"
)

var (
	loadMu   sync.Mutex
	bffMu    sync.Mutex
	cloudAPI http.Handler
	webBFF   http.Handler
)

func init() {
	functions.HTTP("ServeAgentinder", ServeAgentinder)
	functions.HTTP("ServeAgentinderBFF", ServeAgentinderBFF)
}

func ServeAgentinder(w http.ResponseWriter, r *http.Request) {
	api, err := loadCloudAPI()
	if err != nil {
		log.Printf("Agentinder startup failed: %v", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	api.ServeHTTP(w, r)
}

func loadCloudAPI() (http.Handler, error) {
	loadMu.Lock()
	defer loadMu.Unlock()
	if cloudAPI != nil {
		return cloudAPI, nil
	}
	api, err := buildCloudAPI()
	if err != nil {
		return nil, err
	}
	cloudAPI = api
	return cloudAPI, nil
}

func buildCloudAPI() (http.Handler, error) {
	origin := os.Getenv("WEB_ORIGIN")
	if !strings.HasPrefix(origin, "https://") {
		return nil, errors.New("WEB_ORIGIN must be an HTTPS origin")
	}
	ring, err := cloudKeyring()
	if err != nil {
		return nil, err
	}
	data, err := store.OpenCloud(context.Background(), os.Getenv("GOOGLE_CLOUD_PROJECT"))
	if err != nil {
		return nil, err
	}
	api := server.New(data, origin, ring)
	webToken := os.Getenv("AGENTINDER_WEB_API_TOKEN")
	if webToken == "" {
		return nil, errors.New("AGENTINDER_WEB_API_TOKEN is required")
	}
	api.UseWebAPIToken(webToken)
	api.UseServerless()
	return api.Handler(), nil
}

func ServeAgentinderBFF(w http.ResponseWriter, r *http.Request) {
	handler, err := loadWebBFF()
	if err != nil {
		log.Printf("Agentinder BFF startup failed: %v", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

func loadWebBFF() (http.Handler, error) {
	bffMu.Lock()
	defer bffMu.Unlock()
	if webBFF != nil {
		return webBFF, nil
	}
	apiURL := os.Getenv("AGENTINDER_API_URL")
	token := os.Getenv("AGENTINDER_WEB_API_TOKEN")
	origin := os.Getenv("WEB_ORIGINS")
	if origin == "" {
		origin = os.Getenv("WEB_ORIGIN")
	}
	if err := bff.ValidateSettings(apiURL, token, origin); err != nil {
		return nil, err
	}
	webBFF = bff.NewHandler(apiURL, token, origin)
	return webBFF, nil
}

func cloudKeyring() (*keyring.Keyring, error) {
	current, err := decodeCloudKey(os.Getenv("AGENT_TOKEN_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	if bytes.Equal(current, make([]byte, keyring.KeySize)) {
		return nil, errors.New("production encryption key cannot be the development key")
	}
	previous := [][]byte{}
	for _, encoded := range strings.Split(os.Getenv("AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS"), ",") {
		if strings.TrimSpace(encoded) == "" {
			continue
		}
		key, err := decodeCloudKey(encoded)
		if err != nil {
			return nil, err
		}
		previous = append(previous, key)
	}
	return keyring.New(current, previous...)
}

func decodeCloudKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != keyring.KeySize {
		return nil, errors.New("AGENT_TOKEN_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	return key, nil
}
