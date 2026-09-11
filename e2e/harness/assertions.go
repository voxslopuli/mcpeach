package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

// AssertAPI asserts a control-plane response status and that the body contains
// the given substrings.
func AssertAPI(t *testing.T, d *Daemon, method, path, body string, wantStatus int, wantSubstrings ...string) {
	t.Helper()
	status, resp := d.API(method, path, body)
	if status != wantStatus {
		t.Fatalf("%s %s: status = %d, want %d (body %s)", method, path, status, wantStatus, resp)
	}
	for _, sub := range wantSubstrings {
		if !strings.Contains(resp, sub) {
			t.Errorf("%s %s: body missing %q: %s", method, path, sub, resp)
		}
	}
}

// AssertNoSecret asserts that a string does not contain any of the given
// secret values.
func AssertNoSecret(t *testing.T, haystack string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(haystack, s) {
			t.Errorf("secret %q leaked into output", s)
		}
	}
}

// JSONBody decodes a JSON response body into out.
func JSONBody(t *testing.T, body string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), out); err != nil {
		t.Fatalf("decode JSON: %v (body %s)", err, body)
	}
}
