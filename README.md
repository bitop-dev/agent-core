# agent-core

A standalone Go binary for running AI agents from the command line. No database, no web UI, no Docker — just a binary, a YAML config, and an API key.

> **Status**: Phase 1 complete + WASM sandbox system. 100+ files, ~15K lines, 130+ tests.

---

## Quick Start

```bash
# Build
go build -o bin/agent-core ./cmd/agent-core/

# Run a one-shot mission
export OPENAI_API_KEY=sk-...
./bin/agent-core run -c examples/research-agent.yaml \
  --mission "What are the top Go testing frameworks in 2026?"

# Interactive multi-turn chat
./bin/agent-core chat -c examples/dev-agent.yaml

# List tools configured for an agent
./bin/agent-core tools -c examples/dev-agent.yaml

# Validate a config file
./bin/agent-core validate examples/research-agent.yaml
```

---

## What It Does

`agent-core` runs an autonomous agent loop:

1. Takes a YAML config (persona, model, skills, tools) and a mission
2. Calls an LLM (Anthropic, OpenAI, Ollama, or any OpenAI-compatible endpoint)
3. Executes tool calls (file ops, shell, HTTP, WASM skill tools, MCP servers)
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
- **API key rotation** on 429
- Configurable attempts per level

### Core Tools (9 built-in)

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
| `agent_spawn` | Spawn parallel sub-agents | On |

### Sandbox System

agent-core supports three sandboxing runtimes for executing skill tools:

| Runtime | Backend | Isolation | Dependencies | Best For |
|---|---|---|---|---|
| **WASM** | Wazero (pure Go) | Capability-based | None | Skill tools, file ops |
| **Container** | Docker / Podman | Full OS-level | Docker daemon | Untrusted code, heavy tasks |
| **Subprocess** | Raw OS process | Minimal | Language runtime | Legacy/dev |

#### WASM Sandbox (Default for Skills)

All community skill tools are compiled to WebAssembly and run inside Wazero:

- **Zero dependencies** — no Python, no pip, no npm, no shell scripts
- **Capability-based security** — tools can only access explicitly granted resources:
  - **AllowedPaths** / **ReadOnlyPaths** — filesystem access
  - **AllowedHosts** — network access (HTTP through host functions)
  - **EnvVars** — environment variable passthrough
- **Host functions** — the `agent_host` module provides HTTP to WASM tools, gated by AllowedHosts
- **~500ms per invocation** (includes compile; cacheable for repeat calls)
- **Portable** — .wasm binaries run on any OS where agent-core runs

```yaml
# Agent YAML sandbox config
sandbox:
  mode: wasm
  allowed_hosts:
    - html.duckduckgo.com
    - api.github.com
  allowed_paths:
    - /home/user/project
  read_only_paths:
    - /etc
  max_timeout_sec: 30
```

#### Container Sandbox

For full isolation, tools can run in Docker/Podman containers:

- Read-only root filesystem
- Memory + CPU limits
- Network disabled by default (opt-in per AllowedHosts)
- `--security-opt=no-new-privileges`
- Auto-detects Docker or Podman

#### Built-in Tool Sandboxing

Core tools also support path-based sandboxing:

- **AllowedPaths / DeniedPaths**: restrict file system access
- **Environment filtering**: only PATH, HOME, TMPDIR passed to subprocesses
- **Output truncation**: configurable max output size
- **Timeouts**: per-tool execution limits

### Skill System

Skills extend agents with domain-specific capabilities — instructions and/or WASM-sandboxed tools.

#### Skill Types

| Type | Has Tools | Runtime | Example |
|---|---|---|---|
| **Tool skill** | Yes (.wasm) | WASM sandbox | web_search, github, slack_notify |
| **Instruction-only** | No | None | summarize, code_review, write_doc |

#### How It Works

1. Skills are declared in the agent YAML: `skills: [web_search, summarize]`
2. If not installed locally, auto-fetched from `skill_sources` (git registries)
3. SKILL.md frontmatter parsed for metadata, body injected into system prompt
4. Tool schemas loaded from `tools/*.json`, executables from `tools/*.wasm`
5. Tools registered in the engine, dispatched through the sandbox runtime

#### SKILL.md Format

```yaml
---
name: web_search
version: 2.0.0
description: "Search the web via DuckDuckGo"
author: platform-team
tags: [web, search]
emoji: "🔍"
runtime: wasm          # wasm | container | subprocess
config:
  max_results:
    type: integer
    default: 10
---

# Instructions (injected into system prompt)

Search the web and return titles, URLs, and snippets...
```

#### Skill CLI

```bash
# Browse available skills from registries
agent-core skill search

# Install from default registry
agent-core skill install web_search

# Install from a custom source
agent-core skill install my_skill --source github.com/yourname/your-skills

# Manage installed skills
agent-core skill list
agent-core skill show web_search
agent-core skill update web_search
agent-core skill remove web_search

# Validate a skill directory
agent-core skill test ./my-skill/
```

### MCP Support

Model Context Protocol for external tool servers:
- **stdio transport**: spawn server as subprocess
- **HTTP/SSE transport**: connect to running server with auth headers
- **Protocol**: initialize → list_tools → call_tool

### Context Management

- **Proactive compaction**: triggers when history exceeds 80% of context window
- **Reactive compaction**: triggers on ContextFull error from provider
- **LLM-summarize**: preserves last 20 messages, summarizes middle section

### Safety Features

| Feature | Description |
|---|---|
| **Loop detection** | 3 strategies: no-progress, ping-pong, failure streak |
| **Credential scrubbing** | Regex-based, applied before entering history |
| **Approval manager** | Full autonomy (default) or supervised mode |
| **Safety heartbeat** | Re-injects constraints every N turns |
| **Deferred-action detection** | Catches unfulfilled promises |

---

## Agent Configuration

```yaml
name: research-agent
model: gpt-4o

system_prompt: |
  You are a research assistant.

skills:
  - web_search
  - web_fetch
  - summarize

sandbox:
  mode: wasm
  allowed_hosts:
    - html.duckduckgo.com
    - "*"

tools:
  core:
    read_file: {}
    list_dir: {}
    grep: {}
    # bash: not listed = disabled

max_turns: 20
timeout_seconds: 300
```

---

## Project Structure

```
cmd/agent-core/          CLI entrypoint (cobra commands)
internal/
  agent/                 Turn loop, events, context management
  provider/              LLM providers (OpenAI, Anthropic, Reliable)
  tool/                  Tool interface, engine, subprocess, sandboxed tool
    builtin/             9 core tool implementations
  sandbox/               WASM, container, subprocess runtimes
    testdata/tools/      WASM tool source code (Go → .wasm)
    testdata/hostcall/   Go bindings for WASM host functions
  skill/                 Skill loader + remote registry
  config/                YAML config parsing + validation
  mcp/                   MCP client (stdio + HTTP transports)
  models/                Model catalog
  observer/              Telemetry interface
  session/               JSONL session persistence
  output/                Terminal renderers (text, JSON, JSONL)
pkg/agent/               Public API for embedding
```

---

## Public API (`pkg/agent`)

For embedding agent-core in other Go programs:

```go
import "github.com/bitop-dev/agent-core/pkg/agent"

// Quick one-shot
result, err := agent.QuickRun(ctx, provider, "gpt-4o", "What is 2+2?")

// Full builder
a, _ := agent.NewBuilder().
    WithConfig(cfg).
    WithProvider(provider).
    WithTools(agent.NewToolEngine()).
    Build()

events, _ := a.Run(ctx, "Analyze this codebase")

// Sandboxed skill registration
reg := agent.NewSandboxRegistry()
wasmRT, _ := agent.NewWASMRuntime(ctx)
reg.Register(wasmRT)

caps := agent.SandboxCapabilities{
    AllowedHosts: []string{"html.duckduckgo.com"},
}
skills := agent.RegisterSkillToolsSandboxed(engine, reg, []string{"web_search"}, "wasm", caps)
```

---

## Testing

```bash
go test ./... -count=1       # All 130+ tests
go test ./... -race           # With race detector

# Sandbox-specific tests
go test ./internal/sandbox/... -v     # WASM, filesystem, HTTP host functions
go test ./internal/sandbox/e2e/ -v    # Full skill E2E (load → register → execute)
```

---

## Part of the Agent Platform

| Repo | Purpose | Status |
|---|---|---|
| **agent-core** (this repo) | Standalone CLI + Go library | ✅ 130+ tests |
| [agent-platform-api](https://github.com/bitop-dev/agent-platform-api) | Go Fiber REST API | ✅ 22 tests |
| [agent-platform-web](https://github.com/bitop-dev/agent-platform-web) | React web portal | ✅ 13 pages |
| [agent-platform-skills](https://github.com/bitop-dev/agent-platform-skills) | Community skill registry | ✅ 10 skills (4 WASM + 6 instruction) |
| [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) | Architecture & planning | ✅ Comprehensive |

---

## License

MIT
