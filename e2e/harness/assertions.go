package harness

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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

// AssertAPIRetry polls a control-plane endpoint until the response contains
// all the wanted substrings (or a deadline), tolerating transient delays such
// as a form submission settling under load.
func AssertAPIRetry(t *testing.T, d *Daemon, method, path, body string, wantStatus int, wantSubstrings ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, resp := d.API(method, path, body)
		if status == wantStatus {
			ok := true
			for _, sub := range wantSubstrings {
				if !strings.Contains(resp, sub) {
					ok = false
					break
				}
			}
			if ok {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	status, resp := d.API(method, path, body)
	t.Fatalf("%s %s: never matched after retry (status %d, body %s)", method, path, status, resp)
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

// AssertToolRegistered asserts the fake server's echo tool is exposed.
func AssertToolRegistered(t *testing.T, d *Daemon) {
	t.Helper()
	AssertAPI(t, d, "GET", "/v0/tools", "", 200, "fake__echo")
}

// AssertNoTools asserts no tools are exposed (server stopped).
func AssertNoTools(t *testing.T, d *Daemon) {
	t.Helper()
	AssertAPI(t, d, "GET", "/v0/tools", "", 200)
}

// JSONBody decodes a JSON response body into out.
func JSONBody(t *testing.T, body string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), out); err != nil {
		t.Fatalf("decode JSON: %v (body %s)", err, body)
	}
}
