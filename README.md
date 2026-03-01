# agent-core

A standalone Go binary for running AI agents from the command line. No database, no web UI, no Docker required — just a binary, a YAML config, and an API key.

> **Status**: Feature-complete. 88 Go files, ~14K lines, 171 tests across 11 packages.

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
3. Executes tool calls — built-in tools, WASM-sandboxed skill tools, Docker containers, MCP servers
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
| **WASM** | [Wazero](https://wazero.io/) (pure Go, no CGO) | Capability-based | None | Skill tools — default |
| **Container** | Docker / Podman | Full OS-level | Docker daemon | Untrusted code, heavy tasks |
| **Native** | In-process Go | N/A | N/A | 9 built-in tools |

#### WASM Sandbox (Default for Skills)

All community skill tools compile to WebAssembly and run inside Wazero:

- **Zero dependencies** — no Python, no pip, no npm, no shell scripts
- **Capability-based security** — tools can only access explicitly granted resources:
  - `AllowedPaths` / `ReadOnlyPaths` — filesystem access
  - `AllowedHosts` — network access (HTTP through host functions)
  - `EnvVars` — environment variable passthrough
  - `MaxMemoryMB`, `MaxTimeoutSec`, `MaxOutputBytes` — resource limits
- **Host functions** — the `agent_host` module provides HTTP to WASM tools:
  - `http_request(method, url, body)` → `(status, body)` — basic HTTP
  - `http_request_headers(method, url, headers, body)` → `(status, body)` — HTTP with custom headers (for authenticated APIs)
- **Module caching** — compiled modules cached by SHA-256 content hash; ~530ms first call, ~3ms cached (160x speedup)
- **Portable** — .wasm binaries run on any OS/arch where agent-core runs

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

**Live example** — WASM web search:
```bash
$ ./bin/agent-core run -c examples/research-agent.yaml \
    --mission "Search for 'Wazero Go runtime'"
✓ WASM sandbox runtime ready
  ✓ web_search → wasm sandbox
⚙ web_search({"query":"Wazero Go runtime","max_results":1})
✓ web_search (Search results for: Wazero Go runtime  1. wazero: the zero dependency WebAssembly runtime for G…)
--- complete | 2 turns | 3956ms ---
```

#### Container Sandbox

For full OS-level isolation, skills can declare `runtime: container` with a Docker/Podman image:

- **Read-only root filesystem** (`--read-only`)
- **Memory + CPU limits** (`--memory=256m --cpus=1`)
- **Network disabled by default** (enabled if `AllowedHosts` set)
- **No privilege escalation** (`--security-opt=no-new-privileges`)
- **Ephemeral** (`--rm`) — container destroyed after each call
- **Tmpfs /tmp** — writable scratch space (`noexec,nosuid,size=64m`)
- **Auto-detects** Docker or Podman (prefers Podman)

Container skills use the same JSON stdin/stdout protocol as WASM tools. The `Module` field is the image name:

```yaml
# SKILL.md frontmatter for a container skill
---
name: text_transform
runtime: container
image: myregistry/text-tool:latest
---
```

**Live example** — mixed WASM + container in one agent:
```bash
$ ./bin/agent-core run -c /tmp/mixed-agent.yaml \
    --mission "Search for 'Go programming' then uppercase the first result title"
✓ WASM sandbox runtime ready
✓ Container sandbox runtime ready (docker)
  ✓ web_search → wasm sandbox
  ✓ text_transform → container sandbox
⚙ web_search({"query":"Go programming language","max_results":1})
✓ web_search (Search results for: Go programming language  1. The Go Programming Language…)
⚙ text_transform({"text":"The Go Programming Language"})
✓ text_transform (THE GO PROGRAMMING LANGUAGE)
--- complete | 3 turns | 7754ms ---
```

#### Built-in Tool Sandboxing

Core tools also support path-based sandboxing:

- **AllowedPaths / DeniedPaths**: restrict file system access
- **Environment filtering**: only PATH, HOME, TMPDIR passed to tool processes
- **Output truncation**: configurable max output size
- **Timeouts**: per-tool execution limits

### Skill System

Skills extend agents with domain-specific capabilities — instructions and/or sandboxed tools.

#### Skill Types

| Type | Has Tools | Runtime | Example |
|---|---|---|---|
| **WASM tool skill** | .wasm files | WASM sandbox | web_search, github, slack_notify |
| **Container tool skill** | Docker image | Container sandbox | Heavy compute, untrusted code |
| **Instruction-only** | None | N/A | summarize, code_review, write_doc |

#### How It Works

1. Skills declared in agent YAML: `skills: [web_search, summarize]`
2. If not installed locally, auto-fetched from `skill_sources` (git registries)
3. SKILL.md frontmatter parsed for metadata, body injected into system prompt
4. Tool schemas loaded from `tools/*.json`
5. **WASM skills**: executables loaded from `tools/*.wasm`, dispatched through Wazero
6. **Container skills**: image name from SKILL.md `image` field, dispatched through Docker/Podman

#### SKILL.md Format

```yaml
---
name: web_search
version: 2.0.0
description: "Search the web via DuckDuckGo"
author: platform-team
tags: [web, search]
emoji: "🔍"
runtime: wasm          # wasm | container
image: ""              # Docker image (for runtime: container)
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
- **stdio transport**: spawn server as child process
- **HTTP/SSE transport**: connect to running server with auth headers
- **Protocol**: initialize → list_tools → call_tool

### Orchestration

- **`agent_spawn` tool**: LLM can spawn parallel sub-agents for divide-and-conquer
- Parent/child run tracking — child runs linked to parent
- Each sub-agent gets its own context, tools, and turn budget

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

## Examples

The [`examples/`](examples/) directory contains 11 ready-to-use agent configs covering every runtime and feature:

### By Runtime

| Example | Runtime | What It Demonstrates |
|---|---|---|
| [`minimal-agent.yaml`](examples/minimal-agent.yaml) | None | Simplest possible agent — just a model and a prompt |
| [`native-tools-agent.yaml`](examples/native-tools-agent.yaml) | Native | All 9 built-in tools, no skills or sandbox |
| [`research-agent.yaml`](examples/research-agent.yaml) | WASM | Web search + fetch via Wazero sandbox |
| [`dev-agent.yaml`](examples/dev-agent.yaml) | WASM | GitHub skill + shell + code review |
| [`standup-bot.yaml`](examples/standup-bot.yaml) | WASM | GitHub + Slack notify + report generation |
| [`slack-notifier-agent.yaml`](examples/slack-notifier-agent.yaml) | WASM | Web search → Slack webhook posting |
| [`container-agent.yaml`](examples/container-agent.yaml) | Container | Docker-based text transform tool |
| [`mixed-runtime-agent.yaml`](examples/mixed-runtime-agent.yaml) | WASM + Container | Both runtimes in one agent |
| [`orchestrator-agent.yaml`](examples/orchestrator-agent.yaml) | WASM | `agent_spawn` for parallel sub-agents |
| [`mcp-agent.yaml`](examples/mcp-agent.yaml) | MCP | External tool servers (HTTP/SSE + stdio) |
| [`ollama-agent.yaml`](examples/ollama-agent.yaml) | Native | Fully offline with local Ollama model |

### Minimal Agent (no skills, no sandbox)

```yaml
name: minimal
model: gpt-4o
system_prompt: |
  You are a helpful assistant.
max_turns: 5
```

### WASM Skill Agent (web search)

```yaml
name: researcher
model: claude-sonnet-4-20250514
skills:
  - web_search
  - web_fetch
  - summarize
sandbox:
  mode: wasm
  allowed_hosts: ["html.duckduckgo.com", "*"]
  max_timeout_sec: 30
tools:
  core:
    read_file: {}
    grep: {}
```

### Container Skill Agent (Docker)

```yaml
name: container-agent
model: gpt-4o
skills:
  - text_transform     # runtime: container, image: myimage:latest
sandbox:
  mode: container
  max_timeout_sec: 30
```

### Mixed Runtime Agent (WASM + Container)

```yaml
name: mixed
model: gpt-4o
skills:
  - web_search          # → WASM sandbox (declared in SKILL.md)
  - text_transform      # → Docker container (declared in SKILL.md)
sandbox:
  mode: wasm
  allowed_hosts: ["html.duckduckgo.com"]
```

### Orchestrator Agent (parallel sub-agents)

```yaml
name: orchestrator
model: claude-sonnet-4-20250514
skills: [web_search, summarize]
tools:
  core:
    agent_spawn: {}     # the orchestration tool
    read_file: {}
max_turns: 20
```

### Full Configuration Reference

```yaml
name: my-agent                      # required
description: "What it does"         # optional
provider: anthropic                 # openai | anthropic | ollama
model: claude-sonnet-4-20250514     # any supported model

system_prompt: |                    # agent persona / instructions
  You are a helpful assistant.

skills:                             # skill names to load
  - web_search
  - github
  - summarize

sandbox:                            # WASM/container sandbox config
  mode: wasm                        # wasm | container
  allowed_hosts:                    # network access whitelist
    - api.github.com
    - "*"                           # wildcard = any host
  allowed_paths:                    # filesystem write access
    - /home/user/project
  read_only_paths:                  # filesystem read access
    - /etc
  env_vars:                         # env vars passed to WASM/container
    GITHUB_TOKEN: ${GITHUB_TOKEN}
  max_timeout_sec: 30               # per-tool timeout
  max_memory_mb: 256                # container memory limit
  max_output_bytes: 1048576         # 1MB output cap

tools:
  core:                             # built-in tool config
    bash:
      timeout_seconds: 60
      working_dir: /tmp
      allowed_paths: [/tmp]
      denied_paths: [/tmp/.env]
    read_file: {}
    write_file: {}
    edit_file: {}
    list_dir: {}
    grep: {}
    http_fetch: {}
    tasks: {}
    agent_spawn: {}

mcp:                                # external MCP tool servers
  servers:
    - name: my-server
      transport: http               # http | stdio
      url: https://mcp.example.com
      headers:
        API_KEY: ${MCP_API_KEY}
    - name: local-server
      transport: stdio
      command: ["npx", "my-mcp-server"]

max_turns: 20                       # turn limit
timeout_seconds: 300                # total run timeout

context:
  strategy: compaction              # compaction | none
  compaction_threshold: 0.8         # trigger at 80% of context window

approval:
  policy: list                      # full | list | none
  required_tools: [bash]            # which tools need approval

output:
  stream: true
  format: text                      # text | json | jsonl
  color: auto                       # auto | always | never
```

---

## Writing WASM Tool Skills

Tools are written in Go and compiled to WebAssembly:

```go
package main

import (
    "encoding/json"
    "os"
    "github.com/bitop-dev/agent-core/pkg/hostcall"
)

func main() {
    // Read JSON from stdin
    var input struct {
        Name      string          `json:"name"`
        Arguments json.RawMessage `json:"arguments"`
    }
    json.NewDecoder(os.Stdin).Decode(&input)

    // Make HTTP request through host function
    body, status := hostcall.HTTPGet("https://example.com/api")

    // Or with custom headers (for authenticated APIs)
    body, status = hostcall.HTTPRequestWithHeaders(
        "GET", "https://api.github.com/repos/golang/go",
        "Authorization: Bearer token123\nAccept: application/json",
        "",
    )

    // Write JSON result to stdout
    json.NewEncoder(os.Stdout).Encode(map[string]any{
        "result": string(body),
        "status": status,
    })
}
```

Compile:
```bash
GOOS=wasip1 GOARCH=wasm go build -o tools/my_tool.wasm .
```

The `pkg/hostcall` package provides `//go:wasmimport` bindings:
- `HTTPGet(url)` / `HTTPPost(url, body)` — basic HTTP
- `HTTPRequestWithHeaders(method, url, headers, body)` — full HTTP with custom headers

See [`BLDER_DOCS/wasm-tool-guide.md`](https://github.com/bitop-dev/agent-platform-docs/blob/main/BLDER_DOCS/wasm-tool-guide.md) in the docs repo for the complete authoring guide.

---

## Writing Container Tool Skills

For tools that need full OS access, use container skills:

```go
// main.go — reads JSON from stdin, writes JSON to stdout
package main

import (
    "encoding/json"
    "os"
)

func main() {
    var input struct {
        Name      string          `json:"name"`
        Arguments json.RawMessage `json:"arguments"`
    }
    json.NewDecoder(os.Stdin).Decode(&input)

    // Do whatever you need — full OS, network, etc.
    result := doWork(input.Arguments)

    json.NewEncoder(os.Stdout).Encode(result)
}
```

```dockerfile
# Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -ldflags="-s -w" -o tool .

FROM alpine:3.21
COPY --from=builder /app/tool /usr/local/bin/tool
ENTRYPOINT ["tool"]
```

```yaml
# SKILL.md
---
name: my_container_tool
runtime: container
image: myregistry/my-tool:latest
---
```

The container runs with `--read-only --no-new-privileges --memory=256m --cpus=1 --network=none` by default.

---

## Project Structure

```
cmd/agent-core/          CLI entrypoint (cobra commands)
internal/
  agent/                 Turn loop, events, context management
  provider/              LLM providers (OpenAI, Anthropic, Reliable)
  tool/                  Tool interface, engine, sandboxed tool
    builtin/             9 core tool implementations
  sandbox/               WASM and container runtimes
    testdata/            WASM tool source, container Dockerfile, test data
  skill/                 Skill loader + remote registry
  config/                YAML config parsing + validation
  mcp/                   MCP client (stdio + HTTP transports)
  models/                Model catalog
  observer/              Telemetry interface
  session/               JSONL session persistence
  output/                Terminal renderers (text, JSON, JSONL)
pkg/
  agent/                 Public API for embedding agent-core in Go programs
  hostcall/              WASM guest bindings (//go:wasmimport for tool authors)
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

// Sandboxed skill registration (WASM + container)
reg := agent.NewSandboxRegistry()
wasmRT, _ := agent.NewWASMRuntime(ctx)
containerRT, _ := agent.NewContainerRuntime()
reg.Register(wasmRT)
reg.Register(containerRT)

caps := agent.SandboxCapabilities{
    AllowedHosts: []string{"html.duckduckgo.com"},
}
skills := agent.RegisterSkillToolsSandboxed(engine, reg, []string{"web_search"}, "wasm", caps)
```

---

## Testing

```bash
go test ./... -count=1        # All 171 tests across 11 packages
go test ./... -race            # With race detector

# Sandbox-specific tests
go test ./internal/sandbox/... -v       # WASM + container runtimes
go test ./internal/sandbox/e2e/ -v      # Full skill E2E (load → register → execute)

# Container E2E (requires Docker + test image)
docker build -t agent-core-test-tool:latest internal/sandbox/testdata/container_tool/
go test ./internal/sandbox/ -v -run TestContainerRuntime
```

---

## Part of the Agent Platform

| Repo | Purpose | Status |
|---|---|---|
| **agent-core** (this repo) | Standalone CLI + Go library | ✅ 171 tests, 45 commits |
| [agent-platform-api](https://github.com/bitop-dev/agent-platform-api) | Go Fiber REST API | ✅ 22 tests, 24 commits |
| [agent-platform-web](https://github.com/bitop-dev/agent-platform-web) | React web portal | ✅ 14 pages, 18 commits |
| [agent-platform-skills](https://github.com/bitop-dev/agent-platform-skills) | Community skill registry | ✅ 10 skills (4 WASM + 6 instruction) |
| [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) | Architecture & planning | ✅ Comprehensive |

---

## License

MIT
