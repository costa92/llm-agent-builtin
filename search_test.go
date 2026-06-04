package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMockSearch_DefaultDocs_HitsByQuery(t *testing.T) {
	s := NewMockSearch()
	args, _ := json.Marshal(map[string]any{"query": "Go", "top_k": 3})
	out, err := s.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Go") {
		t.Errorf("output should contain Go: %q", out)
	}
}

func TestMockSearch_TopK_Truncates(t *testing.T) {
	docs := []Doc{
		{Title: "a", Body: "the cat sat"},
		{Title: "b", Body: "the cat ran"},
		{Title: "c", Body: "the cat ate"},
	}
	s := NewMockSearch(docs...)
	args, _ := json.Marshal(map[string]any{"query": "cat", "top_k": 2})
	out, err := s.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "Title:") != 2 {
		t.Errorf("expected 2 hits, got %q", out)
	}
}

func TestMockSearch_NoMatch(t *testing.T) {
	s := NewMockSearch(Doc{Title: "a", Body: "hello"})
	args, _ := json.Marshal(map[string]any{"query": "zzz", "top_k": 5})
	out, _ := s.Execute(context.Background(), args)
	if !strings.Contains(out, "no results") {
		t.Errorf("output = %q", out)
	}
}

func TestMockSearch_SchemaIsValidJSON(t *testing.T) {
	s := NewMockSearch()
	var m map[string]any
	if err := json.Unmarshal(s.Schema(), &m); err != nil {
		t.Errorf("Schema not valid JSON: %v", err)
	}
}
