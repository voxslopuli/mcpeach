// Package secrets resolves config values that reference environment variables
// (`env:VAR`) or the OS keychain (`keychain:service/user`), so secrets never
// need to be stored in plaintext in the config file.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrNotFound is returned when a keychain entry does not exist.
var ErrNotFound = errors.New("keychain entry not found")

// Store is the keychain interface. It is satisfied by the OS keychain via
// go-keyring, and by an in-memory fake in tests.
type Store interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

// KeyringStore is a Store backed by the OS keychain via go-keyring.
type KeyringStore struct{}

// NewKeyringStore returns a Store backed by the OS keychain.
func NewKeyringStore() *KeyringStore {
	return &KeyringStore{}
}

// Get fetches a secret from the OS keychain.
func (k *KeyringStore) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

// Set stores a secret in the OS keychain.
func (k *KeyringStore) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

// Delete removes a secret from the OS keychain.
func (k *KeyringStore) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// MemoryStore is an in-memory Store used for tests and the E2E suite (so no
// test touches the real OS keychain). It is safe for concurrent use.
type MemoryStore struct {
	mu     sync.Mutex
	values map[string]string
}

// NewMemoryStore returns an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{values: map[string]string{}}
}

// Get fetches a secret from the in-memory map.
func (m *MemoryStore) Get(service, user string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.values[service+"/"+user]; ok {
		return v, nil
	}
	return "", ErrNotFound
}

// Set stores a secret in the in-memory map.
func (m *MemoryStore) Set(service, user, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[service+"/"+user] = secret
	return nil
}

// Delete removes a secret from the in-memory map.
func (m *MemoryStore) Delete(service, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, service+"/"+user)
	return nil
}

// NewStoreFromEnv returns a Store selected by the MCPEACH_KEYCHAIN_STORE env
// var: "memory" uses an in-memory store (for tests/E2E), anything else uses
// the OS keychain.
func NewStoreFromEnv() Store {
	if os.Getenv("MCPEACH_KEYCHAIN_STORE") == "memory" {
		return NewMemoryStore()
	}
	return NewKeyringStore()
}

// Resolver resolves secret references against the environment and keychain.
type Resolver struct {
	store Store
}

// NewResolver builds a Resolver. store may be nil (then keychain: refs fail).
func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

// SetStore swaps the keychain store. Used by tests to inject an in-memory
// fake; production never calls it.
func (r *Resolver) SetStore(s Store) {
	r.store = s
}

// IsSecretRef reports whether a value is a secret reference (env: or keychain:).
func IsSecretRef(val string) bool {
	return strings.HasPrefix(val, "env:") || strings.HasPrefix(val, "keychain:")
}

// secretNameHints are substrings that strongly suggest an env variable name
// carries a secret value.
var secretNameHints = []string{
	"TOKEN", "SECRET", "PASSWORD", "PASSWD", "API_KEY", "PRIVATE_KEY",
	"CREDENTIAL", "AUTH", "APIKEY", "ACCESS_KEY", "SECRET_KEY",
}

// LooksLikeSecretName reports whether an environment variable name is likely
// to hold a secret (e.g. contains TOKEN/API_KEY). Used to warn when a literal
// (plaintext) value is chosen for a sensitive-looking variable.
func LooksLikeSecretName(name string) bool {
	up := strings.ToUpper(name)
	for _, h := range secretNameHints {
		if strings.Contains(up, h) {
			return true
		}
	}
	return false
}

// Resolve resolves a config value through the chain:
//  1. keychain:ref → fetch from the OS keychain.
//  2. env:VAR → read from the process environment.
//  3. plain value → returned as-is.
func (r *Resolver) Resolve(val string) (string, error) {
	switch {
	case strings.HasPrefix(val, "keychain:"):
		if r.store == nil {
			return "", errors.New("keychain store not available")
		}
		service, user, err := splitKeychainRef(val[len("keychain:"):])
		if err != nil {
			return "", err
		}
		secret, err := r.store.Get(service, user)
		if err != nil {
			return "", fmt.Errorf("keychain %s/%s: %w", service, user, err)
		}
		return secret, nil
	case strings.HasPrefix(val, "env:"):
		name := val[len("env:"):]
		v, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("environment variable %q is not set", name)
		}
		return v, nil
	default:
		return val, nil
	}
}

// Store stores a secret in the keychain under "service/user". ref may be a
// bare "service/user" or a full "keychain:service/user" reference.
func (r *Resolver) Store(ref string, secret string) error {
	service, user, err := r.keychainParts(ref)
	if err != nil {
		return err
	}
	return r.store.Set(service, user, secret)
}

// Delete removes a secret from the keychain under "service/user".
func (r *Resolver) Delete(ref string) error {
	service, user, err := r.keychainParts(ref)
	if err != nil {
		return err
	}
	return r.store.Delete(service, user)
}

// keychainParts validates that a keychain store is available and splits a
// "service/user" (or "keychain:service/user") reference into its parts.
func (r *Resolver) keychainParts(ref string) (service, user string, err error) {
	if r.store == nil {
		return "", "", errors.New("keychain store not available")
	}
	return splitKeychainRef(strings.TrimPrefix(ref, "keychain:"))
}

// ResolveEnv resolves a map of environment variables into a []string of
// "KEY=value" entries, resolving each value through the secret chain. Keys are
// returned in sorted order for determinism, and each key is validated so a
// malformed key (empty, containing '=', NUL, or non-printable ASCII) cannot be
// silently dropped or mangled by the subprocess environment.
func (r *Resolver) ResolveEnv(env map[string]string) ([]string, error) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env))
	for _, k := range keys {
		if err := validateEnvKey(k); err != nil {
			return nil, err
		}
		resolved, err := r.Resolve(env[k])
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", k, err)
		}
		// A NUL byte in a value truncates it at the OS boundary when the
		// subprocess is spawned; reject it rather than fail obscurely.
		if strings.ContainsRune(resolved, '\x00') {
			return nil, fmt.Errorf("env %s: resolved value contains NUL byte", k)
		}
		out = append(out, k+"="+resolved)
	}
	return out, nil
}

// validateEnvKey rejects keys that would be dropped or mangled by the
// subprocess environment: empty, containing '=', containing NUL, or outside
// printable ASCII.
func validateEnvKey(k string) error {
	if k == "" {
		return errors.New("env key is empty")
	}
	if strings.ContainsAny(k, "=\x00") {
		return fmt.Errorf("env key %q contains '=' or NUL", k)
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x20 || k[i] > 0x7e {
			return fmt.Errorf("env key %q contains non-printable ASCII", k)
		}
	}
	return nil
}

// MergeEnv merges the resolved configured environment over the process
// environment. Configured keys replace inherited values; unconfigured keys
// keep their inherited value.
func MergeEnv(configured []string) []string {
	// Build a map of configured KEY=value entries (last wins).
	overrides := map[string]string{}
	order := []string{}
	for _, kv := range configured {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, exists := overrides[k]; !exists {
			order = append(order, k)
		}
		overrides[k] = v
	}
	// Start from the inherited environment, replacing configured keys.
	out := []string{}
	seen := map[string]bool{}
	for _, kv := range os.Environ() {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if v, isOverride := overrides[k]; isOverride {
			out = append(out, k+"="+v)
			seen[k] = true
		} else {
			out = append(out, kv)
		}
	}
	// Append any configured keys not present in the inherited env.
	for _, k := range order {
		if !seen[k] {
			out = append(out, k+"="+overrides[k])
		}
	}
	return out
}

// splitKeychainRef parses "service/user" into its parts, requiring both.
func splitKeychainRef(ref string) (string, string, error) {
	idx := strings.Index(ref, "/")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid keychain ref %q (want service/user)", ref)
	}
	service, user := ref[:idx], ref[idx+1:]
	if service == "" || user == "" {
		return "", "", fmt.Errorf("invalid keychain ref %q (service and user must be non-empty)", ref)
	}
	return service, user, nil
}
