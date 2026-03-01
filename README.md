# agent-core

A standalone Go binary for running AI agents from the command line. No database, no web UI, no Docker — just a binary, a YAML config, and an API key.

> **Status**: Phase 1 complete — fully functional agent runtime with tools, skills, MCP, and safety features. 69 files, ~10K lines, 111 tests, 23 commits.

---

## Quick Start

```bash
# Build
make build

# Run a one-shot mission
export ANTHROPIC_API_KEY=sk-...
./bin/agent-core run --config examples/research-agent.yaml \
  --mission "What are the top Go testing frameworks in 2026?"

# Interactive multi-turn chat
./bin/agent-core chat --config examples/dev-agent.yaml

# Pipe input
echo "Summarize this directory" | ./bin/agent-core run --config examples/dev-agent.yaml

# List tools configured for an agent
./bin/agent-core tools --config examples/dev-agent.yaml

# Validate a config file
./bin/agent-core validate --config examples/research-agent.yaml
```

---

## What It Does

`agent-core` runs an autonomous agent loop:

1. Takes a YAML config (persona, model, skills, tools) and a mission
2. Calls an LLM (Anthropic, OpenAI, Ollama, or any OpenAI-compatible endpoint)
3. Executes tool calls (file ops, shell, HTTP, skill tools, MCP servers)
4. Manages context (compaction when the window fills up)
5. Streams results to the terminal in real-time
6. Detects stuck loops, scrubs credentials, and enforces safety limits

---

## Features

### LLM Providers

| Provider | Models | Auto-detected |
|---|---|---|
| **OpenAI Chat Completions** | gpt-4o, gpt-4.1, gpt-5 | Default for all models |
| **Anthropic Messages** | claude-sonnet-4.6, claude-opus-4 | Model name contains claude/sonnet/opus/haiku |
| **OpenAI Responses** | Same as Chat Completions | Explicit `--responses` flag |
| **Ollama** | Any local model | `OLLAMA_HOST` env var or explicit config |

All providers support:
- **SSE streaming** with tool call accumulation
- **`KeyRotatable` interface** for runtime API key rotation
- **Error classification**: Retryable (5xx), RateLimited (429), NonRetryable (4xx), ContextFull

### ReliableProvider

Wraps any provider with production-grade reliability:
- **3-level failover**: retry same → rotate API key → fall back to alternate model
- **Exponential backoff** with jitter
- **API key rotation** on 429 (applies rotated key to all providers in chain)
- Configurable attempts per level

### Core Tools (8 built-in)

| Tool | Description | Default |
|---|---|---|
| `bash` | Run shell commands | **Opt-out** (on by default) |
| `read_file` | Read file with offset/limit | On |
| `write_file` | Write or overwrite files | On |
| `edit_file` | Surgical text find-and-replace | On |
| `list_dir` | List directory with metadata | On |
| `grep` | Regex search with context lines | On |
| `http_fetch` | HTTP GET/POST requests | On |
| `tasks` | Session-scoped task checklist | On |

### Tool Sandboxing

- **AllowedPaths / DeniedPaths**: restrict file system access (deny overrides allow)
- **Environment filtering**: only PATH, HOME, TMPDIR passed to subprocesses
- **Output truncation**: configurable max output size
- **Timeouts**: per-tool execution limits

### Skill System

Skills extend agents with domain-specific capabilities:

- **SKILL.md format**: YAML frontmatter (metadata) + markdown body (injected into system prompt)
- **Subprocess tools**: communicate via stdin/stdout JSON, language-agnostic
- **Eligibility checks**: verify required binaries/env vars before loading
- **Tiers**: bundled (compiled in) → local (`~/.agent-core/skills/`) → community (git URL)
- **Per-agent config**: human controls `config`, LLM controls `arguments`

### MCP Support

Model Context Protocol for external tool servers:
- **stdio transport**: spawn server as subprocess
- **HTTP/SSE transport**: connect to running server with auth headers
- **Protocol**: initialize → list_tools → call_tool
- **Adapter**: converts MCP tools to agent-core Tool interface

### Context Management

- **Proactive compaction**: triggers when history exceeds 80% of context window
- **Reactive compaction**: triggers on ContextFull error from provider
- **LLM-summarize**: preserves last 20 messages, summarizes middle section
- **Tool boundary guard**: never splits history mid tool-call/result sequence

### Safety Features

| Feature | Description |
|---|---|
| **Loop detection** | 3 strategies: no-progress, ping-pong, failure streak. Two-phase: warn → hard stop |
| **Credential scrubbing** | Regex-based, applied before entering conversation history |
| **Approval manager** | Full autonomy (default) or supervised mode with CLI prompts |
| **Safety heartbeat** | Re-injects safety constraints every N turns |
| **Deferred-action detection** | Catches unfulfilled promises ("I'll do that next") |

### Session Persistence

- **JSONL format** at `~/.agent-core/sessions/{id}.jsonl`
- Save/load/list/delete sessions
- Resume multi-turn conversations with `--session`

### Output Formats

- **Text** (default): streaming terminal output with color
- **JSON**: structured output for piping
- **JSONL**: newline-delimited events for streaming consumers

---

## CLI Commands

```
agent-core [command] [flags]

Commands:
  run          Run an agent with a mission (non-interactive)
  chat         Interactive multi-turn chat (readline REPL, slash commands)
  tools        List tools configured for an agent
  skill        Skill management (list, install, remove, new, test, audit)
  mcp          MCP server test command
  sessions     Session management (list, show, clear)
  validate     Validate agent config file
  version      Show version info
```

---

## Agent Configuration

```yaml
name: research-agent
description: "Researches topics and produces summaries"

provider: anthropic
model: claude-sonnet-4-20250514

system_prompt: |
  You are a research assistant. When given a topic, search the web,
  read relevant sources, and produce a clear, cited summary.

skills:
  - web_search:
      backend: ddg
      max_results: 10
  - web_fetch
  - summarize

tools:
  core:
    read_file: {}
    list_dir: {}
    grep: {}
    http_fetch: {}
    # bash: not listed = disabled for this agent

max_turns: 20
timeout_seconds: 300

# Optional sandbox
sandbox:
  allowed_paths: ["/home/user/projects"]
  denied_paths: ["/etc", "/var"]

# Optional MCP servers
mcp:
  servers:
    - name: postgres
      transport: stdio
      command: ["uvx", "mcp-server-postgres", "postgresql://localhost/mydb"]
```

Example configs in [`examples/`](examples/): dev-agent, research-agent, standup-bot, mcp-agent, ollama-agent, sandboxed-agent.

---

## Project Structure

```
cmd/agent-core/         CLI entrypoint (cobra commands)
internal/
  agent/                Turn loop, events, context management
    agent.go            Agent struct, Run() entry point
    loop.go             Main turn loop
    compact.go          Context compaction
    detection.go        Loop detection (3 strategies)
    scrub.go            Credential scrubbing
    approval.go         Approval manager
    heartbeat.go        Safety heartbeat
    deferred.go         Deferred-action detection
    events.go           RunEvent types
    context.go          Context window management
  provider/             LLM provider interface + implementations
    provider.go         Provider/KeyRotatable interfaces
    openai.go           OpenAI Chat Completions (SSE streaming)
    anthropic.go        Anthropic Messages (SSE streaming)
    openai_responses.go OpenAI Responses API
    reliable.go         ReliableProvider (retry/backoff/failover)
    errors.go           Error classification (4 classes)
  tool/                 Tool interface, engine, subprocess runner
    tool.go             Tool interface + ToolEngine
    engine.go           Parallel dispatch
    subprocess.go       Subprocess runner (stdin/stdout JSON)
    sandbox.go          Path/env sandboxing
    builtin/            8 core tool implementations
  skill/                Skill loader
    skill.go            Skill types
    loader.go           ParseSkillMD, LoadAll, CheckEligibility
  config/               YAML config parsing + validation
  models/               Model catalog (12 models, context windows, pricing)
  observer/             Telemetry interface (Noop, Log, Cost, Multi)
  session/              JSONL session persistence
  output/               Terminal renderers (text, JSON, JSONL)
  mcp/                  MCP client
    client.go           MCP client (initialize, list_tools, call_tool)
    transport.go        stdio + HTTP/SSE transports
    adapter.go          MCP → Tool interface adapter
    protocol.go         MCP JSON-RPC types
pkg/agent/              Public API for embedding
  agent.go              Builder pattern, QuickRun, provider factories
examples/               Example YAML configs
```

---

## Public API (`pkg/agent`)

For embedding agent-core in other Go programs (e.g., platform-api):

```go
import "github.com/bitop-dev/agent-core/pkg/agent"

// Quick one-shot run
result, err := agent.QuickRun(ctx, agent.QuickRunOptions{
    Model:        "gpt-4o",
    APIKey:       os.Getenv("OPENAI_API_KEY"),
    SystemPrompt: "You are a helpful assistant.",
    Mission:      "What is 2+2?",
})

// Full builder pattern
a := agent.NewBuilder().
    WithModel("claude-sonnet-4-20250514").
    WithAPIKey(key).
    WithSystemPrompt(prompt).
    WithTools(agent.NewToolEngine(nil)).
    Build()

result, err := a.Run(ctx, "Analyze this codebase")
```

---

## Testing

```bash
make test        # Run all 111 tests
make test-race   # With race detector
make lint        # golangci-lint
```

Tests cover: providers (error classification, reliable provider), agent (compaction, loop detection, scrubbing, approval, heartbeat, deferred), tools (sandbox), skills (loader, eligibility), MCP (protocol), output (renderers), pkg/agent (public API).

---

## Part of the Agent Platform

| Repo | Purpose | Status |
|---|---|---|
| **agent-core** (this repo) | Standalone CLI + Go library | ✅ 111 tests |
| [agent-platform-api](https://github.com/bitop-dev/agent-platform-api) | REST API server | ✅ 22 tests |
| [agent-platform-web](https://github.com/bitop-dev/agent-platform-web) | Next.js web portal | ✅ 12 pages |
| [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) | Architecture & planning | ✅ Comprehensive |
| **skills** | Community skill registry | 🔜 Coming soon |

---

## License

TBD
