package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/mcpconfig"
)

// importServer handles POST /v0/import: parse a Claude Code MCP config file
// into a candidate, apply a conflict policy, migrate likely plaintext secrets
// to keychain references (when requested), then publish atomically.
func (h *Handler) importServer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		Path            string `json:"path"`
		ConflictsPolicy string `json:"conflicts_policy"` // review|keep|replace
		SecretsPolicy   string `json:"secrets_policy"`   // keep|keychain
	}
	if !decodeImportRequest(w, r, &req) {
		return
	}
	cfg := h.cfg.Load()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	candidate, conflicts, err := mcpconfig.Import(req.Path, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, "import: "+err.Error())
		return
	}
	applyConflictPolicy(candidate, cfg, conflicts, req.ConflictsPolicy)
	migrated, err := h.migratePlaintextSecrets(candidate, req.SecretsPolicy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store secret: "+err.Error())
		return
	}
	if err := candidate.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.mu.Lock()
	ok := h.publishCandidate(w, candidate)
	h.mu.Unlock()
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imported":  sortedServerNames(candidate),
		"conflicts": conflicts,
		"migrated":  migrated,
	})
}

// decodeImportRequest decodes and validates an import request body, writing an
// error response and returning false on failure.
func decodeImportRequest(w http.ResponseWriter, r *http.Request, req *struct {
	Path            string `json:"path"`
	ConflictsPolicy string `json:"conflicts_policy"`
	SecretsPolicy   string `json:"secrets_policy"`
}) bool {
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return false
	}
	switch req.ConflictsPolicy {
	case "", "review", "keep", "replace":
	default:
		writeError(w, http.StatusBadRequest, "conflicts_policy must be review, keep or replace")
		return false
	}
	if req.SecretsPolicy == "" {
		req.SecretsPolicy = "keychain" // safe default
	}
	return true
}

// applyConflictPolicy keeps or replaces existing servers per the policy.
// "review" returns conflicts and preserves existing servers until the client
// decides; "keep" never overwrites; "replace" uses the imported values.
func applyConflictPolicy(candidate, base *config.Config, conflicts []string, policy string) {
	if policy != "keep" && policy != "review" {
		return
	}
	for _, name := range conflicts {
		delete(candidate.Servers, name)
		candidate.Servers[name] = base.Servers[name]
	}
}

// migratePlaintextSecrets rewrites likely plaintext secret env values into
// keychain references when the policy is "keychain". Returns the migrations.
func (h *Handler) migratePlaintextSecrets(candidate *config.Config, policy string) ([]mcpconfig.SecretRef, error) {
	if policy != "keychain" {
		return nil, nil
	}
	var migrated []mcpconfig.SecretRef
	for name, sc := range candidate.Servers {
		for k, v := range sc.Env {
			if strings.HasPrefix(v, "env:") || strings.HasPrefix(v, "keychain:") {
				continue
			}
			if !looksLikeSecretName(k) {
				continue
			}
			ref := "keychain:mcpeach/" + name + "/" + k
			if err := h.res.Store(ref, v); err != nil {
				return nil, err
			}
			sc.Env[k] = ref
			migrated = append(migrated, mcpconfig.SecretRef{Server: name, Name: k})
		}
		candidate.Servers[name] = sc
	}
	return migrated, nil
}

// exportServer handles POST /v0/export: write the config to a Claude Code MCP
// config JSON file. Secrets are preserved as references by default; resolving
// them to plaintext requires an explicit allow_plaintext flag.
func (h *Handler) exportServer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req struct {
		Path           string `json:"path"`
		Secrets        string `json:"secrets"` // references|plaintext
		AllowPlaintext bool   `json:"allow_plaintext"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if req.Secrets == "" {
		req.Secrets = "references" // safe default
	}
	if req.Secrets == "plaintext" && !req.AllowPlaintext {
		writeError(w, http.StatusBadRequest, "plaintext export requires allow_plaintext=true")
		return
	}

	cfg := h.cfg.Load()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	if req.Secrets == "plaintext" {
		resolved, err := h.resolveForExport(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg = resolved
	}
	if err := mcpconfig.Export(req.Path, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "export: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exported": true, "secrets": req.Secrets})
}

// resolveForExport returns a copy of cfg with all env values resolved to
// plaintext, used only when the caller explicitly opts into plaintext export.
func (h *Handler) resolveForExport(cfg *config.Config) (*config.Config, error) {
	resolved := *cfg
	resolved.Servers = make(map[string]config.ServerConfig, len(cfg.Servers))
	for name, sc := range cfg.Servers {
		sc = copyServerConfig(sc)
		for k, v := range sc.Env {
			rv, err := h.res.Resolve(v)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %v", name, k, err)
			}
			sc.Env[k] = rv
		}
		resolved.Servers[name] = sc
	}
	return &resolved, nil
}

// looksLikeSecretName reports whether an env name is likely a secret.
func looksLikeSecretName(name string) bool {
	up := strings.ToUpper(name)
	for _, h := range []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "API_KEY", "PRIVATE_KEY", "CREDENTIAL", "AUTH", "APIKEY", "ACCESS_KEY", "SECRET_KEY"} {
		if strings.Contains(up, h) {
			return true
		}
	}
	return false
}

// sortedServerNames returns the server names of cfg, sorted.
func sortedServerNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
