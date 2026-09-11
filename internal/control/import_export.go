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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	switch req.ConflictsPolicy {
	case "", "review", "keep", "replace":
	default:
		writeError(w, http.StatusBadRequest, "conflicts_policy must be review, keep or replace")
		return
	}
	if req.SecretsPolicy == "" {
		req.SecretsPolicy = "keychain" // safe default
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

	// Apply the conflict policy.
	if req.ConflictsPolicy == "keep" || req.ConflictsPolicy == "review" {
		// "review" means: return conflicts and let the client decide; until
		// then, preserve existing servers (do not overwrite).
		for _, name := range conflicts {
			delete(candidate.Servers, name)
			candidate.Servers[name] = cfg.Servers[name]
		}
	}

	// Migrate likely plaintext secrets to keychain references.
	var migrated []mcpconfig.SecretRef
	if req.SecretsPolicy == "keychain" {
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
					writeError(w, http.StatusInternalServerError, "store secret: "+err.Error())
					return
				}
				sc.Env[k] = ref
				migrated = append(migrated, mcpconfig.SecretRef{Server: name, Name: k})
			}
			candidate.Servers[name] = sc
		}
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

	// Resolve plaintext requires a config with resolved values; build a copy.
	exportCfg := cfg
	if req.Secrets == "plaintext" {
		resolved := *cfg
		resolved.Servers = make(map[string]config.ServerConfig, len(cfg.Servers))
		for name, sc := range cfg.Servers {
			sc = copyServerConfig(sc)
			for k, v := range sc.Env {
				rv, err := h.res.Resolve(v)
				if err != nil {
					writeError(w, http.StatusBadRequest, fmt.Sprintf("%s/%s: %v", name, k, err))
					return
				}
				sc.Env[k] = rv
			}
			resolved.Servers[name] = sc
		}
		exportCfg = &resolved
	}

	if err := mcpconfig.Export(req.Path, exportCfg); err != nil {
		writeError(w, http.StatusInternalServerError, "export: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exported": true, "secrets": req.Secrets})
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
