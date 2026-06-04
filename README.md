[English](./README.md) | [简体中文](./README.zh-CN.md)

# llm-agent-builtin

Ready-to-use, (almost) dependency-free `agents.Tool` implementations for the [llm-agent ecosystem](https://github.com/costa92/llm-agent-ecosystem).

Part of the [llm-agent ecosystem](https://github.com/costa92/llm-agent-ecosystem). Extracted from the core `llm-agent` framework during Phase A so the built-in tool library can evolve independently of the agent runtime.

## Install

```bash
go get github.com/costa92/llm-agent-builtin
```

```go
import builtin "github.com/costa92/llm-agent-builtin"
```

The module exposes a single package, `builtin`. Every tool it provides satisfies the `agents.Tool` interface from [`github.com/costa92/llm-agent-contract/agents`](https://github.com/costa92/llm-agent-contract), so they plug straight into any agent (e.g. a `ReActAgent` or `FunctionCallAgent`) that accepts tools.

## What it provides

All tools implement the four-method `agents.Tool` contract: `Name() string`, `Description() string`, `Schema() json.RawMessage`, and `Execute(ctx, args) (string, error)`.

| Tool | Constructor | `Name()` | Purpose | Dependencies |
|---|---|---|---|---|
| `Calculator` | `NewCalculator()` | `calculator` | Evaluates arithmetic expressions (`+ - * /`, parentheses, unary minus, integers and decimals) via a hand-rolled Pratt parser. | stdlib only |
| `MockSearch` | `NewMockSearch(docs ...Doc)` | `search` | Case-insensitive substring match over an in-memory `[]Doc`; returns the top-k hits. Falls back to a small built-in Go FAQ corpus when no docs are given. **Not** a real search engine — for tests and demos. | stdlib only |
| `NoteTool` | `NewNoteTool(workspace string)` | `note` | Persistent structured notes (YAML frontmatter + Markdown body), one `.md` file per note in a workspace directory. Actions: `create`, `read`, `update`, `search`, `list`, `summary`, `delete`. Survives process restart. | stdlib only |
| `TerminalTool` | `NewTerminalTool(cfg TerminalToolConfig)` | `terminal` | Runs a curated subset of read-only POSIX commands inside a sandbox directory. **Default-disabled** — see Security below. | imports `llm-agent-contract/agents` |

Supporting exported types:

- `Doc{ Title, Body string }` — a single document searched by `MockSearch`.
- `Note{ ID, Title, Type string; Tags []string; CreatedAt, UpdatedAt time.Time; Body string }` — the in-memory shape of a persisted note.
- `TerminalToolConfig{ Workspace string; AllowedCommands []string; Timeout time.Duration; MaxOutputBytes int; EnableUnsafeExecution bool }` — `TerminalTool` configuration.
- `ErrTerminalDisabled` — returned by `NewTerminalTool` when `EnableUnsafeExecution` is not `true`.

### `TerminalTool` security

`TerminalTool` is intentionally restrictive. `NewTerminalTool` returns `ErrTerminalDisabled` unless `EnableUnsafeExecution` is set to `true`. Even when enabled, four guards stay enforced:

1. **Whitelist** — the command binary must be in `AllowedCommands` (defaults to a conservative read-only set: `ls cat head tail find grep wc sort uniq cut awk sed pwd file stat du df`); shell metacharacters (semicolon, `&&`, `||`, pipe, redirects, backtick, `$(`, `$`) in the command are rejected.
2. **Sandbox** — the working directory is locked to `Workspace`; args containing `..` are refused.
3. **Timeout** — the process is cancelled after `Timeout` (default 30s).
4. **Output cap** — output is truncated at `MaxOutputBytes` (default 10MB) with a `[truncated]` marker.

It deliberately does not support pipes, redirects, background jobs, AND/OR chains, command substitution, env-var substitution, or subshells.

## Usage

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	builtin "github.com/costa92/llm-agent-builtin"
)

func main() {
	calc := builtin.NewCalculator()

	// Tools take their args as raw JSON, matching the agents.Tool contract.
	args, _ := json.Marshal(map[string]string{"expr": "2 * (3 + 4)"})
	out, err := calc.Execute(context.Background(), args)
	if err != nil {
		panic(err)
	}
	fmt.Println(out) // 14
}
```

To register tools with an agent, pass the constructed tools (which already satisfy `agents.Tool`) to whatever tool list your agent accepts:

```go
calc := builtin.NewCalculator()
search := builtin.NewMockSearch() // built-in Go FAQ corpus

note, err := builtin.NewNoteTool("/tmp/agent-notes")
if err != nil {
	// handle error
}

term, err := builtin.NewTerminalTool(builtin.TerminalToolConfig{
	Workspace:             "/tmp/agent-sandbox",
	EnableUnsafeExecution: true, // required; otherwise ErrTerminalDisabled
})
if err != nil {
	// handle error
}

tools := []agents.Tool{calc, search, note, term}
```

## Relationship to the ecosystem

`llm-agent-builtin` was extracted from the core `llm-agent` framework's built-in tool package during the **Phase A extraction**, and tagged `v0.1.0`. It depends only on the agent contract (`github.com/costa92/llm-agent-contract/agents`) for the `Tool` interface — not on the agent runtime — so the tool library and the runtime can version independently. Any consumer that depends on `llm-agent-contract` can use these tools without pulling in the full framework.

## Development

```bash
GOWORK=off go vet ./...
GOWORK=off go build ./...
GOWORK=off go test ./... -count=1
```

`GOWORK=off` is required: the umbrella `go.work` excludes this standalone sibling, so Go commands must opt out of workspace mode.
