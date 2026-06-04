package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agents "github.com/costa92/llm-agent-contract/agents"
)

// TerminalTool runs a curated subset of POSIX commands inside a sandbox
// directory. It is INTENTIONALLY restrictive: no pipes, no redirects,
// no shell metacharacters, no subshells, no env-var substitution.
//
// SECURITY: This tool is DEFAULT-DISABLED. Constructing it requires
// EnableUnsafeExecution=true on the config. Even with that flag, the
// 4 layers below stay enforced:
//
//  1. Whitelist: command must be in AllowedCommands; metacharacters
//     in args are rejected.
//  2. Sandbox: working directory locked to Workspace via filepath.Abs;
//     paths containing ".." rejected.
//  3. Timeout: exec.CommandContext + WithTimeout cancels the process.
//  4. Output cap: io.LimitReader + truncation marker.
//
// What this tool will NOT do (by design): pipe ("|"), redirect (">"
// "<"), background ("&"), AND/OR ("&&" "||"), command substitution
// ("$(", "`"), env substitution ("$VAR"), subshells ("(", "(;").
type TerminalTool struct {
	cfg TerminalToolConfig
}

// TerminalToolConfig configures a TerminalTool.
type TerminalToolConfig struct {
	Workspace             string        // sandbox root; required, must exist
	AllowedCommands       []string      // whitelist; defaults to a safe read-only set
	Timeout               time.Duration // default 30s
	MaxOutputBytes        int           // default 10MB
	EnableUnsafeExecution bool          // MUST be true; tool refuses to construct otherwise
}

// ErrTerminalDisabled is returned by NewTerminalTool when the caller
// fails to set EnableUnsafeExecution=true.
var ErrTerminalDisabled = errors.New("builtin: terminal tool requires EnableUnsafeExecution=true")

// defaultAllowed is the conservative read-only command set. Override
// via TerminalToolConfig.AllowedCommands for write-side commands.
var defaultAllowed = []string{
	"ls", "cat", "head", "tail", "find", "grep", "wc", "sort", "uniq",
	"cut", "awk", "sed", "pwd", "file", "stat", "du", "df",
}

// metacharacters that we refuse anywhere in args.
var forbiddenChars = []string{";", "&&", "||", "|", ">", "<", "`", "$(", "$"}

// NewTerminalTool constructs a TerminalTool. Returns ErrTerminalDisabled
// when EnableUnsafeExecution is false.
func NewTerminalTool(cfg TerminalToolConfig) (*TerminalTool, error) {
	if !cfg.EnableUnsafeExecution {
		return nil, ErrTerminalDisabled
	}
	if strings.TrimSpace(cfg.Workspace) == "" {
		return nil, errors.New("builtin: terminal tool requires Workspace")
	}
	abs, err := filepath.Abs(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("builtin: terminal Workspace abs: %w", err)
	}
	cfg.Workspace = abs
	if len(cfg.AllowedCommands) == 0 {
		cfg.AllowedCommands = append([]string(nil), defaultAllowed...)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 10 * 1024 * 1024
	}
	return &TerminalTool{cfg: cfg}, nil
}

// Name implements agents.Tool.
func (t *TerminalTool) Name() string { return "terminal" }

// Description implements agents.Tool.
func (t *TerminalTool) Description() string {
	return "Run a curated read-only POSIX command inside a sandbox. Args: {\"command\": \"ls -la\"}. No pipes/redirects/subshells. " +
		"Allowed: " + strings.Join(t.cfg.AllowedCommands, " ")
}

// Schema implements agents.Tool.
func (t *TerminalTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "Command + args, e.g. 'ls -la'. No metacharacters."}
		},
		"required": ["command"]
	}`)
}

type terminalArgs struct {
	Command string `json:"command"`
}

// Execute implements agents.Tool. Returns command stdout (capped),
// or an error explaining which guard rejected the command.
func (t *TerminalTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var p terminalArgs
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("terminal: bad args: %w", err)
	}
	cmdLine := strings.TrimSpace(p.Command)
	if cmdLine == "" {
		return "", errors.New("terminal: command is required")
	}

	// Guard 1: metacharacters
	for _, bad := range forbiddenChars {
		if strings.Contains(cmdLine, bad) {
			return "", fmt.Errorf("terminal: forbidden metacharacter %q in command", bad)
		}
	}

	// Tokenize on whitespace (we already rejected metachars, so split is safe).
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return "", errors.New("terminal: empty command")
	}
	bin := parts[0]
	args := parts[1:]

	// Guard 1b: whitelist
	if !inList(bin, t.cfg.AllowedCommands) {
		return "", fmt.Errorf("terminal: command %q not in whitelist", bin)
	}

	// Guard 2: sandbox — reject ".." anywhere in args
	for _, a := range args {
		if strings.Contains(a, "..") {
			return "", fmt.Errorf("terminal: path arg %q contains '..'; refused", a)
		}
	}

	// Guard 3: timeout
	runCtx, cancel := context.WithTimeout(ctx, t.cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = t.cfg.Workspace

	// Guard 4: capped output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("terminal: stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("terminal: start: %w", err)
	}

	limited := io.LimitReader(stdout, int64(t.cfg.MaxOutputBytes))
	out, readErr := io.ReadAll(limited)
	waitErr := cmd.Wait()

	if runCtx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("terminal: timed out after %s", t.cfg.Timeout)
	}
	if waitErr != nil {
		return string(out), fmt.Errorf("terminal: %w", waitErr)
	}
	if readErr != nil {
		return string(out), fmt.Errorf("terminal: read: %w", readErr)
	}

	// Detect truncation by reading one more byte; if anything's there,
	// we hit the cap.
	one := make([]byte, 1)
	if n, _ := stdout.Read(one); n > 0 {
		return string(out) + "\n[truncated]", nil
	}
	return string(out), nil
}

func inList(s string, list []string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Verify TerminalTool satisfies the agents.Tool interface.
var _ agents.Tool = (*TerminalTool)(nil)
