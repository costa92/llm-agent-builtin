package builtin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipOnNonUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("terminal tool tests rely on POSIX commands; skipping on windows")
	}
}

func TestTerminalTool_DisabledByDefault(t *testing.T) {
	_, err := NewTerminalTool(TerminalToolConfig{Workspace: t.TempDir()})
	if !errors.Is(err, ErrTerminalDisabled) {
		t.Errorf("err = %v, want ErrTerminalDisabled", err)
	}
}

func TestTerminalTool_RequiresWorkspace(t *testing.T) {
	_, err := NewTerminalTool(TerminalToolConfig{EnableUnsafeExecution: true})
	if err == nil {
		t.Error("expected error when Workspace empty")
	}
}

func TestTerminalTool_LSWorks(t *testing.T) {
	skipOnNonUnix(t)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("a"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("b"), 0644)

	tool, err := NewTerminalTool(TerminalToolConfig{
		Workspace:             dir,
		EnableUnsafeExecution: true,
	})
	if err != nil {
		t.Fatalf("NewTerminalTool: %v", err)
	}
	out, err := tool.Execute(context.Background(), []byte(`{"command":"ls"}`))
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(out, "file1.txt") || !strings.Contains(out, "file2.txt") {
		t.Errorf("ls output missing files: %q", out)
	}
}

func TestTerminalTool_RejectsNonWhitelistedCommand(t *testing.T) {
	skipOnNonUnix(t)
	tool, _ := NewTerminalTool(TerminalToolConfig{
		Workspace:             t.TempDir(),
		EnableUnsafeExecution: true,
	})
	_, err := tool.Execute(context.Background(), []byte(`{"command":"rm -rf /"}`))
	if err == nil || !strings.Contains(err.Error(), "whitelist") {
		t.Errorf("expected whitelist rejection, got %v", err)
	}
}

func TestTerminalTool_RejectsMetaCharacters(t *testing.T) {
	skipOnNonUnix(t)
	tool, _ := NewTerminalTool(TerminalToolConfig{
		Workspace:             t.TempDir(),
		EnableUnsafeExecution: true,
	})

	cases := []string{
		`{"command":"ls; rm -rf /"}`,
		`{"command":"ls && cat /etc/passwd"}`,
		`{"command":"ls | grep secret"}`,
		`{"command":"cat > /tmp/bad"}`,
		`{"command":"echo $(whoami)"}`,
		"{\"command\":\"echo `whoami`\"}",
	}
	for _, args := range cases {
		_, err := tool.Execute(context.Background(), []byte(args))
		if err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("expected forbidden-metachar rejection for %s, got %v", args, err)
		}
	}
}

func TestTerminalTool_RejectsParentPathTraversal(t *testing.T) {
	skipOnNonUnix(t)
	tool, _ := NewTerminalTool(TerminalToolConfig{
		Workspace:             t.TempDir(),
		EnableUnsafeExecution: true,
	})
	_, err := tool.Execute(context.Background(), []byte(`{"command":"cat ../../../etc/passwd"}`))
	if err == nil || !strings.Contains(err.Error(), "..") {
		t.Errorf("expected '..' rejection, got %v", err)
	}
}

func TestTerminalTool_BadJSON(t *testing.T) {
	skipOnNonUnix(t)
	tool, _ := NewTerminalTool(TerminalToolConfig{
		Workspace:             t.TempDir(),
		EnableUnsafeExecution: true,
	})
	_, err := tool.Execute(context.Background(), []byte(`not json`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestTerminalTool_EmptyCommand(t *testing.T) {
	skipOnNonUnix(t)
	tool, _ := NewTerminalTool(TerminalToolConfig{
		Workspace:             t.TempDir(),
		EnableUnsafeExecution: true,
	})
	_, err := tool.Execute(context.Background(), []byte(`{"command":""}`))
	if err == nil {
		t.Error("expected error for empty command")
	}
}

func TestTerminalTool_DefaultsApplied(t *testing.T) {
	tool, err := NewTerminalTool(TerminalToolConfig{
		Workspace:             t.TempDir(),
		EnableUnsafeExecution: true,
	})
	if err != nil {
		t.Fatalf("NewTerminalTool: %v", err)
	}
	if tool.cfg.Timeout == 0 {
		t.Error("Timeout default not applied")
	}
	if tool.cfg.MaxOutputBytes == 0 {
		t.Error("MaxOutputBytes default not applied")
	}
	if len(tool.cfg.AllowedCommands) == 0 {
		t.Error("AllowedCommands default not applied")
	}
}

func TestTerminalTool_NameDescriptionSchema(t *testing.T) {
	tool, _ := NewTerminalTool(TerminalToolConfig{
		Workspace:             t.TempDir(),
		EnableUnsafeExecution: true,
	})
	if tool.Name() != "terminal" {
		t.Errorf("Name = %q, want terminal", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if len(tool.Schema()) == 0 {
		t.Error("Schema empty")
	}
}
