// Package client is the control-plane client used by the TUI (and future web
// client) to talk to the mcpeach daemon over its unix-socket HTTP API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Client talks to the mcpeach control-plane API.
type Client struct {
	base string
	http *http.Client
}

// New builds a client for the given base URL using the default transport
// (e.g. an httptest server URL). For a unix-socket API, use NewUnix.
func New(base string) *Client {
	return &Client{base: strings.TrimSuffix(base, "/"), http: &http.Client{}}
}

// NewUnix builds a client that talks to a unix-socket HTTP API. The base URL
// should be "http://unix" and the socket path is dialed directly.
func NewUnix(sock string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}
	return &Client{base: "http://unix", http: &http.Client{Transport: transport}}
}

// ServerInfo mirrors the control-plane ServerInfo.
type ServerInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// AddServerRequest is the POST /v0/servers request body.
type AddServerRequest struct {
	Name      string            `json:"name"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Transport string            `json:"transport,omitempty"`
}

// ListServers returns the configured servers.
func (c *Client) ListServers(ctx context.Context) ([]ServerInfo, error) {
	var resp struct {
		Servers []ServerInfo `json:"servers"`
	}
	if err := c.do(ctx, http.MethodGet, "/v0/servers", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Servers, nil
}

// ListTools returns the aggregated tool names.
func (c *Client) ListTools(ctx context.Context) ([]string, error) {
	var resp struct {
		Tools []string `json:"tools"`
	}
	if err := c.do(ctx, http.MethodGet, "/v0/tools", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

// ServerLogs returns the captured log lines for a server.
func (c *Client) ServerLogs(ctx context.Context, name string) ([]string, error) {
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := c.do(ctx, http.MethodGet, "/v0/servers/"+url.PathEscape(name)+"/logs", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Lines, nil
}

// StartServer starts a server by name.
func (c *Client) StartServer(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v0/servers/"+url.PathEscape(name)+"/start", nil, nil)
}

// StopServer stops a server by name.
func (c *Client) StopServer(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/v0/servers/"+url.PathEscape(name)+"/stop", nil, nil)
}

// AddServer adds a new server to the config.
func (c *Client) AddServer(ctx context.Context, req AddServerRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/v0/servers", bytes.NewReader(body), nil)
}

// do performs an HTTP request and decodes the JSON response, returning an
// error for non-2xx statuses.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error != "" {
			return fmt.Errorf("%s %s: %s", method, path, e.Error)
		}
		return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
