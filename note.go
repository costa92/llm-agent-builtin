package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// NoteTool persists structured notes (YAML frontmatter + Markdown body) to a
// workspace directory. Cross-session memory: notes survive process restart
// while Memory subsystems (Phase 2/3) stay in-process.
//
// File format per note (one .md file per note ID):
//
//	---
//	id: note_20260427_153000_0
//	title: ...
//	type: task_state
//	tags: [a, b]
//	created_at: 2026-04-27T15:30:00Z
//	updated_at: 2026-04-27T15:30:00Z
//	---
//
//	<markdown body>
//
// Frontmatter parser is a minimal hand-rolled YAML subset (key: value, with
// `[a, b]` list form) — keeps the package zero-dep. Round-trips notes we wrote
// ourselves; not a general YAML parser.
type NoteTool struct {
	workspace string

	mu  sync.Mutex // serializes ID generation + write to avoid concurrent collisions
	seq int        // bumped per ID generation within the same second
}

// NewNoteTool constructs a NoteTool. Workspace is created if missing.
func NewNoteTool(workspace string) (*NoteTool, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, errors.New("note: workspace required")
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return nil, fmt.Errorf("note: create workspace: %w", err)
	}
	return &NoteTool{workspace: workspace}, nil
}

// Name implements agents.Tool.
func (*NoteTool) Name() string { return "note" }

// Description implements agents.Tool.
func (*NoteTool) Description() string {
	return "Persistent structured notes (YAML frontmatter + Markdown body) on the local filesystem. Supports create/read/update/search/list/summary/delete actions."
}

// Schema implements agents.Tool.
func (*NoteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["create","read","update","search","list","summary","delete"]},
			"id": {"type": "string"},
			"title": {"type": "string"},
			"content": {"type": "string"},
			"note_type": {"type": "string", "enum": ["task_state","conclusion","blocker","action","reference","general"]},
			"tags": {"type": "array", "items": {"type": "string"}},
			"query": {"type": "string"},
			"limit": {"type": "integer"}
		},
		"required": ["action"]
	}`)
}

type noteArgs struct {
	Action   string   `json:"action"`
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	NoteType string   `json:"note_type"`
	Tags     []string `json:"tags"`
	Query    string   `json:"query"`
	Limit    int      `json:"limit"`
}

// Execute dispatches on action.
func (n *NoteTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p noteArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("note: bad args: %w", err)
	}
	switch p.Action {
	case "create":
		return n.create(p)
	case "read":
		return n.read(p)
	case "update":
		return n.update(p)
	case "search":
		return n.search(p)
	case "list":
		return n.list(p)
	case "summary":
		return n.summary()
	case "delete":
		return n.delete(p)
	default:
		return "", fmt.Errorf("note: unknown action %q", p.Action)
	}
}

// --- core data type --------------------------------------------------------

type Note struct {
	ID        string
	Title     string
	Type      string
	Tags      []string
	CreatedAt time.Time
	UpdatedAt time.Time
	Body      string
}

// --- per-action handlers ---------------------------------------------------

func (n *NoteTool) create(p noteArgs) (string, error) {
	if strings.TrimSpace(p.Title) == "" {
		return "", errors.New("note: title required for create")
	}
	if p.NoteType == "" {
		p.NoteType = "general"
	}
	now := time.Now().UTC()
	id := n.nextID(now)
	note := Note{
		ID:        id,
		Title:     p.Title,
		Type:      p.NoteType,
		Tags:      p.Tags,
		CreatedAt: now,
		UpdatedAt: now,
		Body:      p.Content,
	}
	if err := n.writeNote(note); err != nil {
		return "", err
	}
	return id, nil
}

func (n *NoteTool) read(p noteArgs) (string, error) {
	if p.ID == "" {
		return "", errors.New("note: id required for read")
	}
	path := filepath.Join(n.workspace, p.ID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("note: read %s: %w", p.ID, err)
	}
	return string(data), nil
}

func (n *NoteTool) update(p noteArgs) (string, error) {
	if p.ID == "" {
		return "", errors.New("note: id required for update")
	}
	path := filepath.Join(n.workspace, p.ID+".md")
	existing, err := n.readNote(path)
	if err != nil {
		return "", err
	}
	if p.Title != "" {
		existing.Title = p.Title
	}
	if p.NoteType != "" {
		existing.Type = p.NoteType
	}
	if p.Tags != nil {
		existing.Tags = p.Tags
	}
	if p.Content != "" {
		existing.Body = p.Content
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := n.writeNote(existing); err != nil {
		return "", err
	}
	return existing.ID, nil
}

func (n *NoteTool) search(p noteArgs) (string, error) {
	if strings.TrimSpace(p.Query) == "" {
		return "", errors.New("note: query required for search")
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}
	notes, err := n.listAll()
	if err != nil {
		return "", err
	}
	q := strings.ToLower(p.Query)
	hits := make([]Note, 0)
	for _, note := range notes {
		hay := strings.ToLower(note.Title + " " + note.Body + " " + strings.Join(note.Tags, " "))
		if strings.Contains(hay, q) {
			hits = append(hits, note)
		}
	}
	if len(hits) == 0 {
		return "no results", nil
	}
	if len(hits) > p.Limit {
		hits = hits[:p.Limit]
	}
	return formatNoteList(hits), nil
}

func (n *NoteTool) list(p noteArgs) (string, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	notes, err := n.listAll()
	if err != nil {
		return "", err
	}
	if len(notes) > p.Limit {
		notes = notes[:p.Limit]
	}
	if len(notes) == 0 {
		return "no notes", nil
	}
	return formatNoteList(notes), nil
}

func (n *NoteTool) summary() (string, error) {
	notes, err := n.listAll()
	if err != nil {
		return "", err
	}
	if len(notes) == 0 {
		return "no notes", nil
	}
	counts := make(map[string]int)
	for _, note := range notes {
		counts[note.Type]++
	}
	types := make([]string, 0, len(counts))
	for t := range counts {
		types = append(types, t)
	}
	sort.Strings(types)
	var b strings.Builder
	fmt.Fprintf(&b, "total: %d\n", len(notes))
	for _, t := range types {
		fmt.Fprintf(&b, "  %s: %d\n", t, counts[t])
	}
	return b.String(), nil
}

func (n *NoteTool) delete(p noteArgs) (string, error) {
	if p.ID == "" {
		return "", errors.New("note: id required for delete")
	}
	path := filepath.Join(n.workspace, p.ID+".md")
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("note: delete %s: %w", p.ID, err)
	}
	return "deleted: " + p.ID, nil
}

// --- low-level IO -----------------------------------------------------------

func (n *NoteTool) listAll() ([]Note, error) {
	entries, err := os.ReadDir(n.workspace)
	if err != nil {
		return nil, fmt.Errorf("note: list workspace: %w", err)
	}
	out := make([]Note, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(n.workspace, e.Name())
		note, err := n.readNote(path)
		if err != nil {
			continue // skip malformed files silently — caller sees missing entry, not error
		}
		out = append(out, note)
	}
	// Stable order by CreatedAt (newest first), tiebreaker by ID.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (n *NoteTool) readNote(path string) (Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Note{}, fmt.Errorf("note: read %s: %w", path, err)
	}
	return parseFrontmatter(string(data))
}

func (n *NoteTool) writeNote(note Note) error {
	body := formatFrontmatter(note) + "\n" + note.Body + "\n"
	path := filepath.Join(n.workspace, note.ID+".md")
	return os.WriteFile(path, []byte(body), 0o644)
}

// nextID generates a unique-per-second ID with a monotonic suffix.
func (n *NoteTool) nextID(now time.Time) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	id := fmt.Sprintf("note_%s_%d", now.Format("20060102_150405"), n.seq)
	n.seq++
	return id
}

// --- frontmatter (minimal YAML subset) -------------------------------------

func formatFrontmatter(note Note) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", note.ID)
	fmt.Fprintf(&b, "title: %s\n", note.Title)
	fmt.Fprintf(&b, "type: %s\n", note.Type)
	fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(note.Tags, ", "))
	fmt.Fprintf(&b, "created_at: %s\n", note.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "updated_at: %s\n", note.UpdatedAt.Format(time.RFC3339))
	b.WriteString("---\n")
	return b.String()
}

// parseFrontmatter parses our own write format. Not a general YAML parser.
func parseFrontmatter(s string) (Note, error) {
	const sep = "---"
	if !strings.HasPrefix(s, sep) {
		return Note{}, errors.New("note: missing frontmatter open")
	}
	rest := s[len(sep):]
	rest = strings.TrimLeft(rest, "\n")
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx < 0 {
		return Note{}, errors.New("note: missing frontmatter close")
	}
	header := rest[:closeIdx]
	body := strings.TrimLeft(rest[closeIdx+len("\n---"):], "\n")

	note := Note{}
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimRight(line, "\r ")
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "id":
			note.ID = val
		case "title":
			note.Title = val
		case "type":
			note.Type = val
		case "tags":
			val = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(val, "["), "]"))
			if val == "" {
				note.Tags = nil
			} else {
				parts := strings.Split(val, ",")
				note.Tags = make([]string, 0, len(parts))
				for _, p := range parts {
					if t := strings.TrimSpace(p); t != "" {
						note.Tags = append(note.Tags, t)
					}
				}
			}
		case "created_at":
			t, err := time.Parse(time.RFC3339, val)
			if err == nil {
				note.CreatedAt = t
			}
		case "updated_at":
			t, err := time.Parse(time.RFC3339, val)
			if err == nil {
				note.UpdatedAt = t
			}
		}
	}
	note.Body = body
	return note, nil
}

func formatNoteList(notes []Note) string {
	var b strings.Builder
	for i, note := range notes {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		fmt.Fprintf(&b, "ID: %s\nTitle: %s\nType: %s\nTags: [%s]\n",
			note.ID, note.Title, note.Type, strings.Join(note.Tags, ", "))
	}
	return b.String()
}
