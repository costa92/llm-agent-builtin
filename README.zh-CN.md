[English](./README.md) | [简体中文](./README.zh-CN.md)

# llm-agent-builtin

为 [llm-agent 生态](https://github.com/costa92/llm-agent-ecosystem)提供的开箱即用、（几乎）零依赖的 `agents.Tool` 实现。

[llm-agent 生态](https://github.com/costa92/llm-agent-ecosystem)的一部分。在 Phase A 期间从核心 `llm-agent` 框架中抽取出来，使内置工具库可以独立于智能体运行时演进。

## 安装

```bash
go get github.com/costa92/llm-agent-builtin
```

```go
import builtin "github.com/costa92/llm-agent-builtin"
```

本 module 只暴露单个包 `builtin`。它提供的每个工具都满足来自 [`github.com/costa92/llm-agent-contract/agents`](https://github.com/costa92/llm-agent-contract) 的 `agents.Tool` 接口，因此可以直接接入任何接受工具的智能体（例如 `ReActAgent` 或 `FunctionCallAgent`）。

## 提供的能力

所有工具都实现了四方法的 `agents.Tool` 契约：`Name() string`、`Description() string`、`Schema() json.RawMessage` 以及 `Execute(ctx, args) (string, error)`。

| 工具 | 构造函数 | `Name()` | 用途 | 依赖 |
|---|---|---|---|---|
| `Calculator` | `NewCalculator()` | `calculator` | 通过手写的 Pratt parser 求值算术表达式（`+ - * /`、括号、一元负号、整数与小数）。 | 仅标准库 |
| `MockSearch` | `NewMockSearch(docs ...Doc)` | `search` | 对内存中的 `[]Doc` 做大小写不敏感的子串匹配；返回 top-k 命中。未提供文档时回退到一份内置的小型 Go FAQ 语料。**不是**真正的搜索引擎——仅供测试和演示使用。 | 仅标准库 |
| `NoteTool` | `NewNoteTool(workspace string)` | `note` | 持久化结构化笔记（YAML frontmatter + Markdown 正文），每条笔记一个 `.md` 文件，存放于一个 workspace 目录。动作（action）：`create`、`read`、`update`、`search`、`list`、`summary`、`delete`。可在进程重启后存活。 | 仅标准库 |
| `TerminalTool` | `NewTerminalTool(cfg TerminalToolConfig)` | `terminal` | 在沙箱目录中运行一组精选的只读 POSIX 命令。**默认禁用**——见下文「安全」。 | 导入 `llm-agent-contract/agents` |

配套的导出类型：

- `Doc{ Title, Body string }` —— `MockSearch` 检索的单篇文档。
- `Note{ ID, Title, Type string; Tags []string; CreatedAt, UpdatedAt time.Time; Body string }` —— 已持久化笔记在内存中的形态。
- `TerminalToolConfig{ Workspace string; AllowedCommands []string; Timeout time.Duration; MaxOutputBytes int; EnableUnsafeExecution bool }` —— `TerminalTool` 的配置。
- `ErrTerminalDisabled` —— 当 `EnableUnsafeExecution` 不为 `true` 时由 `NewTerminalTool` 返回。

### `TerminalTool` 安全

`TerminalTool` 刻意做了严格限制。除非将 `EnableUnsafeExecution` 设为 `true`，否则 `NewTerminalTool` 会返回 `ErrTerminalDisabled`。即便已启用，以下四道防护仍然强制生效：

1. **白名单** —— 命令的二进制名必须在 `AllowedCommands` 中（默认是一组保守的只读集合：`ls cat head tail find grep wc sort uniq cut awk sed pwd file stat du df`）；命令中的 shell 元字符（分号、`&&`、`||`、管道、重定向、反引号、`$(`、`$`）会被拒绝。
2. **沙箱** —— 工作目录被锁定到 `Workspace`；包含 `..` 的参数会被拒绝。
3. **超时** —— 进程在 `Timeout`（默认 30s）后被取消。
4. **输出上限** —— 输出在 `MaxOutputBytes`（默认 10MB）处被截断，并附带 `[truncated]` 标记。

它刻意不支持管道、重定向、后台作业、AND/OR 串联、命令替换、环境变量替换或子 shell。

## 用法

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

要把工具注册到智能体上，将构造好的工具（它们已经满足 `agents.Tool`）传给智能体接受的工具列表即可：

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

## 与生态的关系

`llm-agent-builtin` 是在 **Phase A 抽取**期间从核心 `llm-agent` 框架的内置工具包中抽取出来，并打上 `v0.1.0` tag。它仅依赖智能体契约（`github.com/costa92/llm-agent-contract/agents`）以获取 `Tool` 接口——而不依赖智能体运行时——因此工具库与运行时可以各自独立地进行版本管理。任何依赖 `llm-agent-contract` 的消费方都能使用这些工具，而无需引入完整的框架。

## 开发

```bash
GOWORK=off go vet ./...
GOWORK=off go build ./...
GOWORK=off go test ./... -count=1
```

`GOWORK=off` 是必需的：伞形仓库的 `go.work` 排除了这个独立的兄弟仓，因此 Go 命令必须选择关闭工作区模式。
