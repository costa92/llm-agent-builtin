package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Doc is a single document searched by MockSearch.
type Doc struct {
	Title string
	Body  string
}

// MockSearch performs case-insensitive substring matching against an in-memory
// document list. NOT a real search engine — intended for learning/demo only.
type MockSearch struct {
	docs []Doc
}

// NewMockSearch returns a MockSearch with the provided docs. If none are
// provided, falls back to a small built-in Go FAQ corpus.
func NewMockSearch(docs ...Doc) *MockSearch {
	if len(docs) == 0 {
		docs = defaultDocs
	}
	return &MockSearch{docs: docs}
}

var defaultDocs = []Doc{
	{Title: "Go modules", Body: "Go modules are the standard dependency management mechanism."},
	{Title: "Goroutines", Body: "Goroutines are lightweight threads managed by the Go runtime."},
	{Title: "Channels", Body: "Channels in Go provide a way to communicate between goroutines."},
	{Title: "Interfaces", Body: "Interfaces in Go are satisfied implicitly through method sets."},
	{Title: "Error handling", Body: "Go uses explicit error values returned from functions."},
}

// Name implements agents.Tool.
func (MockSearch) Name() string { return "search" }

// Description implements agents.Tool.
func (MockSearch) Description() string {
	return "Search a small in-memory document corpus by case-insensitive substring match. Returns top_k matching docs."
}

// Schema implements agents.Tool.
func (MockSearch) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string"},
			"top_k": {"type": "integer", "default": 3}
		},
		"required": ["query"]
	}`)
}

// Execute runs the substring match.
func (s *MockSearch) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("search: bad args: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return "", errors.New("search: query is required")
	}
	if p.TopK <= 0 {
		p.TopK = 3
	}

	q := strings.ToLower(p.Query)
	hits := make([]Doc, 0, len(s.docs))
	for _, d := range s.docs {
		if strings.Contains(strings.ToLower(d.Title+" "+d.Body), q) {
			hits = append(hits, d)
		}
	}
	if len(hits) == 0 {
		return "no results", nil
	}
	if len(hits) > p.TopK {
		hits = hits[:p.TopK]
	}

	var b strings.Builder
	for i, d := range hits {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		fmt.Fprintf(&b, "Title: %s\n%s", d.Title, d.Body)
	}
	return b.String(), nil
}
