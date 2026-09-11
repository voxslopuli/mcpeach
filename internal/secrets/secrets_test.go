package secrets

import (
	"strings"
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

func TestResolveEnvMap(t *testing.T) {
	t.Setenv("MY_SECRET", "s3cret")
	r := NewResolver(nil)
	got, err := r.ResolveEnv(map[string]string{"TOKEN": "env:MY_SECRET", "PLAIN": "value"})
	if err != nil {
		t.Fatalf("ResolveEnv: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ResolveEnv = %v, want 2 entries", got)
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e] = true
	}
	if !seen["TOKEN=s3cret"] {
		t.Errorf("ResolveEnv missing TOKEN=s3cret: %v", got)
	}
	if !seen["PLAIN=value"] {
		t.Errorf("ResolveEnv missing PLAIN=value: %v", got)
	}
}

func TestResolveEnvMapError(t *testing.T) {
	r := NewResolver(nil)
	if _, err := r.ResolveEnv(map[string]string{"TOKEN": "env:DOES_NOT_EXIST_XYZ"}); err == nil {
		t.Fatal("ResolveEnv with unset env: want error, got nil")
	}
}

func TestResolveEnvDeterministic(t *testing.T) {
	r := NewResolver(nil)
	env := map[string]string{"Z": "1", "A": "2", "M": "3"}
	got, err := r.ResolveEnv(env)
	if err != nil {
		t.Fatalf("ResolveEnv: %v", err)
	}
	want := []string{"A=2", "M=3", "Z=1"}
	if len(got) != len(want) {
		t.Fatalf("ResolveEnv = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ResolveEnv[%d] = %q, want %q (sorted)", i, got[i], want[i])
		}
	}
}

func TestResolveEnvRejectsInvalidKey(t *testing.T) {
	r := NewResolver(nil)
	tests := []map[string]string{
		{"": "v"},           // empty key
		{"BAD=KEY": "v"},    // contains '='
		{"BAD\x00KEY": "v"}, // contains NUL
		{"BAD\x7fKEY": "v"}, // non-printable ASCII (DEL)
	}
	for _, env := range tests {
		if _, err := r.ResolveEnv(env); err == nil {
			t.Errorf("ResolveEnv(%v): want error, got nil", env)
		}
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

// envMap converts a []string of KEY=value entries into a map for assertions.
func envMap(entries []string) map[string]string {
	m := map[string]string{}
	for _, kv := range entries {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			m[k] = v
		}
	}
	return m
}

func TestMergeEnv(t *testing.T) {
	t.Setenv("INHERITED_VAR", "inherited-value")
	t.Setenv("PATH", "/usr/bin:/bin")

	got := envMap(MergeEnv([]string{"FOO=bar", "PATH=/custom"}))

	if got["INHERITED_VAR"] != "inherited-value" {
		t.Errorf("INHERITED_VAR = %q, want inherited-value (inherited var must survive)", got["INHERITED_VAR"])
	}
	if got["PATH"] != "/custom" {
		t.Errorf("PATH = %q, want /custom (configured key must override inherited)", got["PATH"])
	}
	if got["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar (new configured key must be appended)", got["FOO"])
	}
}

func TestMergeEnvEmptyConfigured(t *testing.T) {
	t.Setenv("KEEP_ME", "yes")

	got := envMap(MergeEnv(nil))

	if got["KEEP_ME"] != "yes" {
		t.Errorf("KEEP_ME = %q, want yes (empty configured must keep all inherited vars)", got["KEEP_ME"])
	}
}

func TestMergeEnvDuplicateKeys(t *testing.T) {
	t.Setenv("DUP", "inherited")

	got := envMap(MergeEnv([]string{"DUP=first", "DUP=second"}))

	if got["DUP"] != "second" {
		t.Errorf("DUP = %q, want second (last configured value must win)", got["DUP"])
	}
}

func TestSplitKeychainRefEmptyParts(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"service/user", false},
		{"/user", true},
		{"service/", true},
		{"/", true},
		{"no-slash", true},
	}
	for _, tt := range tests {
		_, _, err := splitKeychainRef(tt.ref)
		if tt.want && err == nil {
			t.Errorf("splitKeychainRef(%q): want error, got nil", tt.ref)
		}
		if !tt.want && err != nil {
			t.Errorf("splitKeychainRef(%q): want nil, got %v", tt.ref, err)
		}
	}
}

func TestResolveEnvRejectsNULValue(t *testing.T) {
	r := NewResolver(NewKeyringStore())
	_, err := r.ResolveEnv(map[string]string{"BAD": "a\x00b"})
	if err == nil {
		t.Fatal("ResolveEnv with NUL value: want error, got nil")
	}
}

func TestLooksLikeSecretName(t *testing.T) {
	for name, want := range map[string]bool{
		"GITHUB_TOKEN":  true,
		"API_KEY":       true,
		"DB_PASSWORD":   true,
		"PRIVATE_KEY":   true,
		"LOG_LEVEL":     false,
		"PORT":          false,
		"access_token":  true, // case-insensitive
	} {
		if got := LooksLikeSecretName(name); got != want {
			t.Errorf("LooksLikeSecretName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestResolverDelete(t *testing.T) {
	store := &fakeStore{values: map[string]string{"mcpeach/srv/TOKEN": "v"}}
	r := NewResolver(store)
	if err := r.Delete("keychain:mcpeach/srv/TOKEN"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.values["mcpeach/srv/TOKEN"]; ok {
		t.Error("entry still present after Delete")
	}
}

func TestMemoryStore(t *testing.T) {
	ms := NewMemoryStore()
	if _, err := ms.Get("s", "u"); err != ErrNotFound {
		t.Errorf("Get missing = %v, want ErrNotFound", err)
	}
	if err := ms.Set("s", "u", "v"); err != nil {
		t.Fatal(err)
	}
	got, err := ms.Get("s", "u")
	if err != nil || got != "v" {
		t.Errorf("Get = %q, %v", got, err)
	}
	if err := ms.Delete("s", "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Get("s", "u"); err != ErrNotFound {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestNewStoreFromEnv(t *testing.T) {
	t.Setenv("MCPEACH_KEYCHAIN_STORE", "memory")
	if _, ok := NewStoreFromEnv().(*MemoryStore); !ok {
		t.Error("expected MemoryStore when MCPEACH_KEYCHAIN_STORE=memory")
	}
	t.Setenv("MCPEACH_KEYCHAIN_STORE", "")
	if _, ok := NewStoreFromEnv().(*KeyringStore); !ok {
		t.Error("expected KeyringStore by default")
	}
}
