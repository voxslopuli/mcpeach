package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mcpeach/mcpeach/internal/config"
)

// newTestFinder builds a Finder pointed at an httptest server that returns the
// given chat completion response.
func newTestFinder(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) *Finder {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(respond))
	t.Cleanup(srv.Close)
	return New(config.LLMConfig{
		BaseURL: srv.URL,
		Model:   "test-model",
	})
}

func TestNew(t *testing.T) {
	f := New(config.LLMConfig{BaseURL: "http://x", Model: "m"})
	if f == nil {
		t.Fatal("New returned nil")
	}
}

func TestFindTools(t *testing.T) {
	// The mock returns a JSON list of recommended tools.
	f := newTestFinder(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify the request body has the model and messages.
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("model = %q, want test-model", req.Model)
		}
		if len(req.Messages) == 0 {
			t.Error("no messages in request")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `["github__create_or_update_file", "context7__get-library-docs"]`}},
			},
		})
	})

	tools := []Tool{
		{Name: "github__create_or_update_file", Description: "Create or update a file"},
		{Name: "context7__get-library-docs", Description: "Get library docs"},
		{Name: "other__unrelated", Description: "Unrelated tool"},
	}
	got, err := f.FindTools(context.Background(), "create a file", tools)
	if err != nil {
		t.Fatalf("FindTools: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("FindTools = %d results, want 2", len(got))
	}
	if got[0].Name != "github__create_or_update_file" {
		t.Errorf("result[0] = %q, want github__create_or_update_file", got[0].Name)
	}
}

func TestFindToolsEmpty(t *testing.T) {
	f := newTestFinder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `[]`}},
			},
		})
	})
	got, err := f.FindTools(context.Background(), "nothing", []Tool{{Name: "a", Description: "a"}})
	if err != nil {
		t.Fatalf("FindTools: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FindTools = %d results, want 0", len(got))
	}
}

func TestFindToolsFiltersUnknown(t *testing.T) {
	// The LLM recommends a tool not in the catalog; it should be dropped.
	f := newTestFinder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `["known__tool", "unknown__tool"]`}},
			},
		})
	})
	got, err := f.FindTools(context.Background(), "q", []Tool{{Name: "known__tool", Description: "known"}})
	if err != nil {
		t.Fatalf("FindTools: %v", err)
	}
	if len(got) != 1 || got[0].Name != "known__tool" {
		t.Errorf("FindTools = %v, want [known__tool]", got)
	}
}

func TestFindToolsError(t *testing.T) {
	f := newTestFinder(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := f.FindTools(context.Background(), "q", []Tool{{Name: "a", Description: "a"}}); err == nil {
		t.Fatal("FindTools on 500: want error, got nil")
	}
}

func TestFindToolsBadJSON(t *testing.T) {
	f := newTestFinder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices": [{"message": {"content": "not json"}}]}`))
	})
	if _, err := f.FindTools(context.Background(), "q", []Tool{{Name: "a", Description: "a"}}); err == nil {
		t.Fatal("FindTools with bad JSON: want error, got nil")
	}
}
