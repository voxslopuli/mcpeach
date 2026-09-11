package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/mcpeach/mcpeach/internal/config"
)

// SecretSource describes one environment variable source for a server. The
// resolved value is never exposed: only the reference (e.g. "keychain:..." or
// "env:...") and whether it resolves.
type SecretSource struct {
	Name       string `json:"name"`
	Reference  string `json:"reference"` // e.g. keychain:mcpeach/github/TOKEN or env:TOKEN
	Resolvable bool   `json:"resolvable"`
}

// listSecrets returns every server's environment secret sources (names +
// references, never values). Keys are sorted for determinism.
func (h *Handler) listSecrets(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.Load()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	type serverSecrets struct {
		Server  string         `json:"server"`
		Sources []SecretSource `json:"sources"`
	}
	out := []serverSecrets{}
	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sc := cfg.Servers[name]
		ss := serverSecrets{Server: name, Sources: []SecretSource{}}
		envKeys := make([]string, 0, len(sc.Env))
		for k := range sc.Env {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		for _, k := range envKeys {
			ref := sc.Env[k]
			src := SecretSource{Name: k, Reference: ref}
			if ref == "" {
				src.Resolvable = true // literal empty
			} else if strings.HasPrefix(ref, "keychain:") || strings.HasPrefix(ref, "env:") {
				_, err := h.res.Resolve(ref)
				src.Resolvable = err == nil
			} else {
				src.Resolvable = true // literal
			}
			ss.Sources = append(ss.Sources, src)
		}
		out = append(out, ss)
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": out})
}

// storeSecret stores (or replaces) a keychain secret for a server env var and
// points the config at it. The reference is keychain:mcpeach/<server>/<variable>.
func (h *Handler) storeSecret(w http.ResponseWriter, r *http.Request) {
	server := r.PathValue("server")
	variable := r.PathValue("variable")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}
	ref := "keychain:mcpeach/" + server + "/" + variable
	if err := h.res.Store(ref, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "store secret: "+err.Error())
		return
	}
	// Update the config to reference the keychain entry so the server uses it.
	// The server config (including its Env map) is deep-copied so we never
	// mutate the live published snapshot; only the atomic swap publishes it.
	// If persistence fails, remove the just-written keychain entry so we do
	// not leave an orphan behind.
	h.mu.Lock()
	cfg := h.cfg.Load()
	if cfg != nil {
		candidate := *cfg
		candidate.Servers = copyServers(cfg.Servers)
		if sc, ok := candidate.Servers[server]; ok {
			sc = copyServerConfig(sc)
			if sc.Env == nil {
				sc.Env = map[string]string{}
			}
			sc.Env[variable] = ref
			candidate.Servers[server] = sc
			if err := candidate.Validate(); err == nil {
				if !h.publishCandidate(w, &candidate) {
					// config not persisted; roll back the keychain write
					_ = h.res.Delete(ref)
					h.mu.Unlock()
					return
				}
			}
		}
	}
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"reference": ref})
}

// deleteSecret removes a keychain secret and its config reference.
func (h *Handler) deleteSecret(w http.ResponseWriter, r *http.Request) {
	server := r.PathValue("server")
	variable := r.PathValue("variable")
	ref := "keychain:mcpeach/" + server + "/" + variable

	h.mu.Lock()
	cfg := h.cfg.Load()
	if cfg == nil {
		h.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	if _, ok := cfg.Servers[server]; !ok {
		h.mu.Unlock()
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", server))
		return
	}
	candidate := *cfg
	candidate.Servers = copyServers(cfg.Servers)
	if sc, ok := candidate.Servers[server]; ok {
		sc = copyServerConfig(sc)
		delete(sc.Env, variable)
		candidate.Servers[server] = sc
		h.publishCandidate(w, &candidate)
	}
	h.mu.Unlock()

	if err := h.res.Delete(ref); err != nil && !isKeychainNotFound(err) {
		writeError(w, http.StatusInternalServerError, "delete secret: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// copyServerConfig deep-copies a server config so mutating its Env map never
// touches the live published snapshot.
func copyServerConfig(sc config.ServerConfig) config.ServerConfig {
	out := sc
	if sc.Env != nil {
		env := make(map[string]string, len(sc.Env))
		for k, v := range sc.Env {
			env[k] = v
		}
		out.Env = env
	}
	return out
}

// isKeychainNotFound reports whether a keychain error means the entry is absent.
func isKeychainNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no such")
}
