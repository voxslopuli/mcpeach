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

// Resolver resolves secret references against the environment and keychain.
type Resolver struct {
	store Store
}

// NewResolver builds a Resolver. store may be nil (then keychain: refs fail).
func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

// IsSecretRef reports whether a value is a secret reference (env: or keychain:).
func IsSecretRef(val string) bool {
	return strings.HasPrefix(val, "env:") || strings.HasPrefix(val, "keychain:")
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

// Store stores a secret in the keychain under "service/user".
func (r *Resolver) Store(ref string, secret string) error {
	if r.store == nil {
		return errors.New("keychain store not available")
	}
	service, user, err := splitKeychainRef(ref)
	if err != nil {
		return err
	}
	return r.store.Set(service, user, secret)
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
