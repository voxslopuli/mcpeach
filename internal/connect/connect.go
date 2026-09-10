// Package connect wires an upstream MCP server (stdio, SSE, or streamable-HTTP)
// into an mcp-go client, initializes it, and discovers its tools so the gateway
// can aggregate them.
package connect

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/gateway"
	"github.com/mcpeach/mcpeach/internal/secrets"
)

// Connect establishes a client to the configured server and returns a
// ToolCaller plus the discovered tools. The caller owns closing the client.
// env is passed to stdio subprocesses.
func Connect(ctx context.Context, sc config.ServerConfig, env []string) (gateway.ToolCaller, []mcp.Tool, error) {
	var c *client.Client
	var err error

	switch {
	case sc.Command != "":
		c, err = client.NewStdioMCPClient(sc.Command, secrets.MergeEnv(env), sc.Args...)
	case sc.URL != "":
		switch sc.Transport {
		case "sse":
			c, err = client.NewSSEMCPClient(sc.URL)
		case "", "streamable-http":
			c, err = client.NewStreamableHttpClient(sc.URL)
		default:
			return nil, nil, fmt.Errorf("unknown transport %q", sc.Transport)
		}
	default:
		return nil, nil, fmt.Errorf("server has neither command nor url")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("create client: %w", err)
	}

	if _, err := c.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		_ = c.Close()
		return nil, nil, fmt.Errorf("initialize: %w", err)
	}

	tools, err := DiscoverTools(ctx, c)
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	return c, tools, nil
}

// DiscoverTools lists the tools exposed by a connected client.
func DiscoverTools(ctx context.Context, caller gateway.ToolCaller) ([]mcp.Tool, error) {
	lc, ok := caller.(interface {
		ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	})
	if !ok {
		return nil, fmt.Errorf("caller does not support ListTools")
	}
	res, err := lc.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	return res.Tools, nil
}
