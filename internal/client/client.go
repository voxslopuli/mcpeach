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
	Enabled   bool              `json:"enabled,omitempty"`
}

// ServerDetail mirrors the control-plane GET /v0/servers/{name} response.
// Env values are source references (e.g. "keychain:...", "env:..."), never
// resolved secrets.
type ServerDetail struct {
	Name      string            `json:"name"`
	State     string            `json:"state"`
	Transport string            `json:"transport,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Enabled   bool              `json:"enabled"`
	Env       map[string]string `json:"env,omitempty"`
	ToolCount int               `json:"tool_count"`
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

// SecretSource mirrors the control-plane secret reference for one env var.
// Reference holds a source ref (keychain:.../env:.../literal); the value is
// never returned by the API.
type SecretSource struct {
	Name       string `json:"name"`
	Reference  string `json:"reference"`
	Resolvable bool   `json:"resolvable"`
}

// ServerSecrets groups a server's env secret sources.
type ServerSecrets struct {
	Server  string         `json:"server"`
	Sources []SecretSource `json:"sources"`
}

// ListSecrets returns each server's env secret sources (references, never
// values).
func (c *Client) ListSecrets(ctx context.Context) ([]ServerSecrets, error) {
	var resp struct {
		Servers []ServerSecrets `json:"servers"`
	}
	if err := c.do(ctx, http.MethodGet, "/v0/secrets", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Servers, nil
}

// StoreSecret stores (or replaces) a keychain secret for a server env var and
// points the config at it. The value is write-only.
func (c *Client) StoreSecret(ctx context.Context, server, variable, value string) error {
	body, _ := json.Marshal(map[string]string{"value": value})
	return c.do(ctx, http.MethodPost, "/v0/secrets/"+url.PathEscape(server)+"/"+url.PathEscape(variable), bytes.NewReader(body), nil)
}

// DeleteSecret removes a keychain secret and its config reference.
func (c *Client) DeleteSecret(ctx context.Context, server, variable string) error {
	return c.do(ctx, http.MethodDelete, "/v0/secrets/"+url.PathEscape(server)+"/"+url.PathEscape(variable), nil, nil)
}

// GetServer returns the full detail for one server.
func (c *Client) GetServer(ctx context.Context, name string) (ServerDetail, error) {
	var d ServerDetail
	if err := c.do(ctx, http.MethodGet, "/v0/servers/"+url.PathEscape(name), nil, &d); err != nil {
		return ServerDetail{}, err
	}
	return d, nil
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

// UpdateServer applies an edit to an existing server via PUT /v0/servers/{name}.
func (c *Client) UpdateServer(ctx context.Context, name string, req AddServerRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPut, "/v0/servers/"+url.PathEscape(name), bytes.NewReader(body), nil)
}

// DeleteServer removes a server via DELETE /v0/servers/{name}.
func (c *Client) DeleteServer(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/v0/servers/"+url.PathEscape(name), nil, nil)
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
