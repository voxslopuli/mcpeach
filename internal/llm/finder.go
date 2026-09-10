// Package llm provides an OpenAI-compatible client and a tool-finder that
// recommends which MCP tools to enable for a given natural-language request.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mcpeach/mcpeach/internal/config"
	openai "github.com/sashabaranov/go-openai"
)

// Tool is a canonical MCP tool with its description.
type Tool struct {
	Name        string
	Description string
}

// Finder recommends tools from a catalog using an OpenAI-compatible endpoint.
type Finder struct {
	client *openai.Client
	model  string
}

// New builds a Finder from LLM config.
func New(cfg config.LLMConfig) *Finder {
	clientCfg := openai.DefaultConfig(cfg.APIKey)
	clientCfg.BaseURL = cfg.BaseURL
	client := openai.NewClientWithConfig(clientCfg)
	return &Finder{client: client, model: cfg.Model}
}

// FindTools asks the LLM which tools from the catalog best match the query,
// returning only tools that exist in the catalog.
func (f *Finder) FindTools(ctx context.Context, query string, catalog []Tool) ([]Tool, error) {
	prompt := buildPrompt(query, catalog)
	resp, err := f.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: f.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	content := resp.Choices[0].Message.Content

	// Parse the JSON array of tool names.
	var names []string
	if err := json.Unmarshal([]byte(content), &names); err != nil {
		return nil, fmt.Errorf("parse tool list: %w", err)
	}

	// Filter to tools that exist in the catalog.
	known := map[string]Tool{}
	for _, t := range catalog {
		known[t.Name] = t
	}
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		if t, ok := known[n]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

const systemPrompt = `You are a tool-selection assistant. Given a user request and a catalog of available MCP tools (each named <server>__<tool>), return a JSON array of the tool names that would best help accomplish the request. Return only tools from the catalog. If none apply, return []. Respond with ONLY the JSON array, no prose.`

// buildPrompt formats the catalog and query for the LLM.
func buildPrompt(query string, catalog []Tool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "User request: %s\n\nAvailable tools:\n", query)
	for _, t := range catalog {
		fmt.Fprintf(&b, "- %s: %s\n", t.Name, t.Description)
	}
	return b.String()
}
