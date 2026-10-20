package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"agentinder/backend/internal/keyring"
	"agentinder/backend/internal/server"
	"agentinder/backend/internal/store"
)

func main() {
	if err := loadEnvFile(".env"); err != nil {
		log.Fatal(err)
	}
	config, err := readConfig()
	if err != nil {
		log.Fatal(err)
	}
	data, err := store.Open(config.DataPath)
	if err != nil {
		log.Fatal("cannot open application data")
	}
	api := server.New(data, config.WebOrigin, config.Tokens)
	api.ResumeMCPDateWatchers()
	service := &http.Server{Addr: config.Address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("Agentinder API listening on %s (%s)", config.Address, config.Environment)
	if err := service.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

type config struct {
	Environment string
	Address     string
	DataPath    string
	WebOrigin   string
	Tokens      *keyring.Keyring
}

func readConfig() (config, error) {
	c := config{
		Environment: strings.TrimSpace(os.Getenv("APP_ENV")),
		Address:     os.Getenv("SERVER_ADDR"), DataPath: os.Getenv("DATA_PATH"), WebOrigin: os.Getenv("WEB_ORIGIN"),
	}
	tokens, developmentKey, err := readKeyring()
	if err != nil {
		return c, err
	}
	c.Tokens = tokens
	if c.Environment != "development" && c.Environment != "production" {
		return c, errors.New("APP_ENV must be development or production")
	}
	if c.Address == "" {
		c.Address = ":8080"
	}
	if c.DataPath == "" {
		c.DataPath = "data/agentinder.json"
	}
	if c.WebOrigin == "" {
		c.WebOrigin = "http://localhost:5173"
	}
	if c.Environment == "production" && developmentKey {
		return c, errors.New("AGENT_TOKEN_ENCRYPTION_KEY is the development key; generate a private one with: openssl rand -base64 32")
	}
	if c.Environment == "production" && !strings.HasPrefix(c.WebOrigin, "https://") {
		return c, errors.New("production requires HTTPS WEB_ORIGIN")
	}
	return c, nil
}

func loadEnvFile(path string) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return errors.New("invalid .env line")
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if key == "" {
			return errors.New("invalid .env key")
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return nil
}

// readKeyring builds the ring from the current key and, during a rotation,
// the retired keys that must stay readable until their responses expire.
// The boolean reports whether the current key is the all-zero key of .env.example.
func readKeyring() (*keyring.Keyring, bool, error) {
	current, err := decodeKey("AGENT_TOKEN_ENCRYPTION_KEY", os.Getenv("AGENT_TOKEN_ENCRYPTION_KEY"))
	if err != nil {
		return nil, false, err
	}
	development := bytes.Equal(current, make([]byte, keyring.KeySize))
	var retired [][]byte
	for _, encoded := range strings.Split(os.Getenv("AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS"), ",") {
		if strings.TrimSpace(encoded) == "" {
			continue
		}
		key, err := decodeKey("AGENT_TOKEN_ENCRYPTION_KEY_PREVIOUS", encoded)
		if err != nil {
			return nil, false, err
		}
		retired = append(retired, key)
	}
	ring, err := keyring.New(current, retired...)
	if err != nil {
		return nil, false, fmt.Errorf("AGENT_TOKEN_ENCRYPTION_KEY(_PREVIOUS): %w", err)
	}
	return ring, development, nil
}

func decodeKey(name, encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != keyring.KeySize {
		return nil, fmt.Errorf("%s must be a base64-encoded 32-byte key", name)
	}
	return key, nil
}
