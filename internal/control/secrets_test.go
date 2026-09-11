package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/gateway"
	"github.com/mcpeach/mcpeach/internal/secrets"
	"github.com/mcpeach/mcpeach/internal/server"
)

type memStore struct {
	values map[string]string
}

func (m *memStore) Get(service, user string) (string, error) {
	if v, ok := m.values[service+"/"+user]; ok {
		return v, nil
	}
	return "", secrets.ErrNotFound
}
func (m *memStore) Set(service, user, secret string) error {
	m.values[service+"/"+user] = secret
	return nil
}
func (m *memStore) Delete(service, user string) error {
	delete(m.values, service+"/"+user)
	return nil
}

// newSecretsHandler builds a handler with an in-memory keychain store.
func newSecretsHandler(t *testing.T) (*Handler, *memStore) {
	t.Helper()
	cfg := config.Default()
	cfg.Servers["github"] = config.ServerConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
		Env:     map[string]string{"GITHUB_TOKEN": "env:MCPEACH_TEST_UNSET_XYZ"},
		Enabled: true,
	}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	store := &memStore{values: map[string]string{}}
	h.res.SetStore(store)
	// Point Save at a temp path so publishCandidate succeeds without touching
	// the real config.
	h.configPath = filepath.Join(t.TempDir(), "mcpeach.yml")
	return h, store
}

func TestListSecrets(t *testing.T) {
	h, store := newSecretsHandler(t)
	store.values["mcpeach/github/TOKEN"] = "s3cret"
	cfg := h.cfg.Load()
	cfg.Servers["github"].Env["TOKEN"] = "keychain:mcpeach/github/TOKEN"
	h.cfg.Store(cfg)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/secrets", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Servers []struct {
			Server  string         `json:"server"`
			Sources []SecretSource `json:"sources"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Servers) != 1 || resp.Servers[0].Server != "github" {
		t.Fatalf("servers = %+v", resp.Servers)
	}
	// Keys sorted: GITHUB_TOKEN, TOKEN.
	if len(resp.Servers[0].Sources) != 2 {
		t.Fatalf("sources = %+v", resp.Servers[0].Sources)
	}
	// The keychain ref resolves; the missing env ref does not.
	if resp.Servers[0].Sources[0].Resolvable {
		t.Error("env:MCPEACH_TEST_UNSET_XYZ should not resolve (unset)")
	}
	if !resp.Servers[0].Sources[1].Resolvable {
		t.Error("keychain TOKEN should resolve")
	}
	// Values must never leak.
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Error("secret value leaked in response")
	}
}

func TestStoreSecret(t *testing.T) {
	h, store := newSecretsHandler(t)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"value":"newsecret"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/secrets/github/TOKEN", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if v, ok := store.values["mcpeach/github/TOKEN"]; !ok || v != "newsecret" {
		t.Errorf("keychain value = %q, want newsecret", v)
	}
	// Config now references the keychain entry.
	cfg := h.cfg.Load()
	if cfg.Servers["github"].Env["TOKEN"] != "keychain:mcpeach/github/TOKEN" {
		t.Errorf("config env = %q", cfg.Servers["github"].Env["TOKEN"])
	}
}

func TestStoreSecretEmptyValue(t *testing.T) {
	h, _ := newSecretsHandler(t)
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"value":""}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/secrets/github/TOKEN", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDeleteSecret(t *testing.T) {
	h, store := newSecretsHandler(t)
	store.values["mcpeach/github/TOKEN"] = "s3cret"
	cfg := h.cfg.Load()
	cfg.Servers["github"].Env["TOKEN"] = "keychain:mcpeach/github/TOKEN"
	h.cfg.Store(cfg)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v0/secrets/github/TOKEN", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if _, ok := store.values["mcpeach/github/TOKEN"]; ok {
		t.Error("keychain entry still present after delete")
	}
	cfg = h.cfg.Load()
	if _, ok := cfg.Servers["github"].Env["TOKEN"]; ok {
		t.Error("config env still references deleted secret")
	}
}

var _ = context.Background // keep context import
