// Package secrets resolves config values that reference environment variables
// (`env:VAR`) or the OS keychain (`keychain:service/user`), so secrets never
// need to be stored in plaintext in the config file.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

// splitKeychainRef parses "service/user" into its parts.
func splitKeychainRef(ref string) (string, string, error) {
	idx := strings.Index(ref, "/")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid keychain ref %q (want service/user)", ref)
	}
	return ref[:idx], ref[idx+1:], nil
}
