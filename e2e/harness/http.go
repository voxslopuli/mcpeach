package harness

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strings"
	"time"
)

// unixClient returns an HTTP client that dials a unix socket.
func unixClient(sock string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
}

// newUnixRequest builds an HTTP request addressed to the unix socket.
func newUnixRequest(method, path, body string) (*http.Request, error) {
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, "http://unix"+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// Contains reports whether s contains substr.
func Contains(s, substr string) bool { return strings.Contains(s, substr) }
