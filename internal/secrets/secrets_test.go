package secrets

import (
	"testing"
)

// fakeStore is an in-memory keychain store for tests.
type fakeStore struct {
	values map[string]string
}

func (f *fakeStore) Get(service, user string) (string, error) {
	v, ok := f.values[service+"/"+user]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fakeStore) Set(service, user, secret string) error {
	f.values[service+"/"+user] = secret
	return nil
}

func (f *fakeStore) Delete(service, user string) error {
	delete(f.values, service+"/"+user)
	return nil
}

func TestResolvePlain(t *testing.T) {
	r := NewResolver(nil)
	got, err := r.Resolve("hello")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "hello" {
		t.Errorf("Resolve = %q, want hello", got)
	}
}

func TestResolveEnv(t *testing.T) {
	t.Setenv("MY_SECRET", "s3cret")
	r := NewResolver(nil)
	got, err := r.Resolve("env:MY_SECRET")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "s3cret" {
		t.Errorf("Resolve = %q, want s3cret", got)
	}
}

func TestResolveEnvUnset(t *testing.T) {
	r := NewResolver(nil)
	if _, err := r.Resolve("env:DOES_NOT_EXIST_XYZ"); err == nil {
		t.Fatal("Resolve unset env: want error, got nil")
	}
}

func TestResolveKeychain(t *testing.T) {
	store := &fakeStore{values: map[string]string{"mcpeach/github": "ksecret"}}
	r := NewResolver(store)
	got, err := r.Resolve("keychain:mcpeach/github")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "ksecret" {
		t.Errorf("Resolve = %q, want ksecret", got)
	}
}

func TestResolveKeychainNotFound(t *testing.T) {
	store := &fakeStore{values: map[string]string{}}
	r := NewResolver(store)
	if _, err := r.Resolve("keychain:mcpeach/nope"); err == nil {
		t.Fatal("Resolve missing keychain: want error, got nil")
	}
}

func TestResolveKeychainNoStore(t *testing.T) {
	r := NewResolver(nil)
	if _, err := r.Resolve("keychain:mcpeach/github"); err == nil {
		t.Fatal("Resolve keychain with nil store: want error, got nil")
	}
}

func TestIsSecretRef(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"env:FOO", true},
		{"keychain:mcpeach/x", true},
		{"plain", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsSecretRef(tt.val); got != tt.want {
			t.Errorf("IsSecretRef(%q) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestStoreKeychain(t *testing.T) {
	store := &fakeStore{values: map[string]string{}}
	r := NewResolver(store)
	if err := r.Store("mcpeach/github", "newsecret"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := r.Resolve("keychain:mcpeach/github")
	if err != nil {
		t.Fatalf("Resolve after store: %v", err)
	}
	if got != "newsecret" {
		t.Errorf("Resolve = %q, want newsecret", got)
	}
}

func TestStoreKeychainNoStore(t *testing.T) {
	r := NewResolver(nil)
	if err := r.Store("mcpeach/github", "x"); err == nil {
		t.Fatal("Store with nil store: want error, got nil")
	}
}
