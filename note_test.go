package builtin

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoteTool_CreateReadDelete_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	tool, err := NewNoteTool(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create
	args, _ := json.Marshal(map[string]any{
		"action":    "create",
		"title":     "First note",
		"content":   "Hello world",
		"note_type": "general",
		"tags":      []string{"intro", "welcome"},
	})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(out, "note_") {
		t.Errorf("create out = %q, want id starting with note_", out)
	}
	id := out

	// Read
	args, _ = json.Marshal(map[string]any{"action": "read", "id": id})
	out, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, "First note") {
		t.Errorf("read out missing title: %q", out)
	}
	if !strings.Contains(out, "Hello world") {
		t.Errorf("read out missing body: %q", out)
	}

	// Delete
	args, _ = json.Marshal(map[string]any{"action": "delete", "id": id})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Read after delete → error
	args, _ = json.Marshal(map[string]any{"action": "read", "id": id})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Error("read after delete should error")
	}
}

func TestNoteTool_Update_PreservesIDChangesContent(t *testing.T) {
	dir := t.TempDir()
	tool, _ := NewNoteTool(dir)

	args, _ := json.Marshal(map[string]any{
		"action":    "create",
		"title":     "v1",
		"content":   "first",
		"note_type": "general",
	})
	id, _ := tool.Execute(context.Background(), args)

	args, _ = json.Marshal(map[string]any{
		"action":  "update",
		"id":      id,
		"title":   "v2",
		"content": "second",
	})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("update: %v", err)
	}

	args, _ = json.Marshal(map[string]any{"action": "read", "id": id})
	out, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(out, "v2") || !strings.Contains(out, "second") {
		t.Errorf("after update, expected v2/second; got: %q", out)
	}
}

func TestNoteTool_Search_HitsTitleBodyTags(t *testing.T) {
	dir := t.TempDir()
	tool, _ := NewNoteTool(dir)

	create := func(title, body string, tags []string) {
		args, _ := json.Marshal(map[string]any{
			"action":    "create",
			"title":     title,
			"content":   body,
			"note_type": "general",
			"tags":      tags,
		})
		if _, err := tool.Execute(context.Background(), args); err != nil {
			t.Fatal(err)
		}
	}
	create("Apples", "fruit basket", []string{"food"})
	create("Computers", "powerful tool", []string{"tech"})
	create("Pets", "cats and dogs", []string{"animals"})

	// Hit by title
	args, _ := json.Marshal(map[string]any{"action": "search", "query": "apples"})
	out, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(out, "Apples") {
		t.Errorf("search 'apples' should hit Apples title: %q", out)
	}

	// Hit by body
	args, _ = json.Marshal(map[string]any{"action": "search", "query": "powerful"})
	out, _ = tool.Execute(context.Background(), args)
	if !strings.Contains(out, "Computers") {
		t.Errorf("search 'powerful' should hit Computers body: %q", out)
	}

	// Hit by tag
	args, _ = json.Marshal(map[string]any{"action": "search", "query": "animals"})
	out, _ = tool.Execute(context.Background(), args)
	if !strings.Contains(out, "Pets") {
		t.Errorf("search 'animals' should hit Pets via tag: %q", out)
	}
}

func TestNoteTool_List_ReturnsAll(t *testing.T) {
	dir := t.TempDir()
	tool, _ := NewNoteTool(dir)

	for i := 0; i < 3; i++ {
		args, _ := json.Marshal(map[string]any{
			"action":    "create",
			"title":     "title-" + string(rune('A'+i)),
			"content":   "body",
			"note_type": "general",
		})
		tool.Execute(context.Background(), args)
	}

	args, _ := json.Marshal(map[string]any{"action": "list"})
	out, _ := tool.Execute(context.Background(), args)
	if strings.Count(out, "title-") != 3 {
		t.Errorf("list should mention 3 titles, got: %q", out)
	}
}

func TestNoteTool_Summary_CountsByType(t *testing.T) {
	dir := t.TempDir()
	tool, _ := NewNoteTool(dir)

	for _, tp := range []string{"task_state", "task_state", "blocker"} {
		args, _ := json.Marshal(map[string]any{
			"action":    "create",
			"title":     "x",
			"content":   "y",
			"note_type": tp,
		})
		tool.Execute(context.Background(), args)
	}

	args, _ := json.Marshal(map[string]any{"action": "summary"})
	out, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(out, "task_state: 2") {
		t.Errorf("summary should show task_state: 2; got %q", out)
	}
	if !strings.Contains(out, "blocker: 1") {
		t.Errorf("summary should show blocker: 1; got %q", out)
	}
}

func TestNoteTool_BadAction(t *testing.T) {
	dir := t.TempDir()
	tool, _ := NewNoteTool(dir)
	args, _ := json.Marshal(map[string]any{"action": "explode"})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Error("bad action should error")
	}
}

func TestNoteTool_MissingFields(t *testing.T) {
	dir := t.TempDir()
	tool, _ := NewNoteTool(dir)

	// create without title
	args, _ := json.Marshal(map[string]any{"action": "create", "content": "x"})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Error("create without title should error")
	}

	// read without id
	args, _ = json.Marshal(map[string]any{"action": "read"})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Error("read without id should error")
	}
}

func TestNoteTool_FrontmatterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	tool, _ := NewNoteTool(dir)

	args, _ := json.Marshal(map[string]any{
		"action":    "create",
		"title":     "Test: with colon",
		"content":   "Multi\nline\nbody",
		"note_type": "reference",
		"tags":      []string{"a", "b"},
	})
	id, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file exists at expected path
	path := filepath.Join(dir, id+".md")
	if _, err := tool.readNote(path); err != nil {
		t.Errorf("readNote: %v", err)
	}
}

func TestNoteTool_Schema(t *testing.T) {
	tool, _ := NewNoteTool(t.TempDir())
	var m map[string]any
	if err := json.Unmarshal(tool.Schema(), &m); err != nil {
		t.Errorf("Schema not valid JSON: %v", err)
	}
}
