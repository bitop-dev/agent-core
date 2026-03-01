# agent-core

A standalone Go binary for running AI agents from the command line. No database, no web UI, no Docker — just a binary, a YAML config, and an API key.

> **Status**: Early development. Project scaffolding is in place. See the [planning docs](https://github.com/bitop-dev/agent-platform-docs) for the full design.

## Quick Start

```bash
# Build
make build

# Run an agent
./bin/agent-core run --config examples/research-agent.yaml --mission "What are the best Go testing frameworks?"

# Interactive chat
./bin/agent-core chat --config examples/dev-agent.yaml
```

## What It Does

`agent-core` runs an autonomous agent loop:

1. Takes a YAML config (persona, model, skills, tools) and a mission
2. Calls an LLM (Anthropic, OpenAI, Google, Ollama)
3. Executes tool calls (file ops, shell, HTTP, skill tools)
4. Manages context (compaction when the window fills up)
5. Streams results to the terminal in real-time

## Features

- **7 core tools** built in: `bash` (opt-out), `read_file`, `write_file`, `edit_file`, `list_dir`, `grep`, `http_fetch`
- **Skill system** — extend with SKILL.md packages (web_search, github, gitlab, etc.)
- **Multi-provider** — Anthropic, OpenAI, Google, Ollama, OpenAI-compatible
- **Provider reliability** — retry, backoff, API key rotation, model fallback chains
- **Context compaction** — LLM-summarizes old turns when context window fills
- **Loop detection** — catches stuck agents (no-progress, ping-pong, failure streaks)
- **Credential scrubbing** — strips secrets from tool output before sending to LLM
- **Session persistence** — multi-turn chats saved as JSONL files
- **MCP support** — connect to Model Context Protocol tool servers
- **Cost tracking** — per-run token usage and USD cost

## Project Structure

```
cmd/agent-core/     CLI entrypoint
internal/
  agent/            Turn loop, events, context management
  provider/         LLM provider interface + implementations
  tool/             Tool interface, engine, subprocess runner
  tool/builtin/     Core tools (bash, file ops, grep, http)
  skill/            Skill loader (SKILL.md parser, eligibility)
  config/           YAML config parsing + validation
  models/           Model catalog + cost tracking
  observer/         Telemetry (Noop, Log, Cost, Multi)
  session/          JSONL session persistence
  output/           Terminal renderers (text, JSON, quiet)
  mcp/              MCP client (stdio + HTTP/SSE)
pkg/agent/          Public API for embedding in other Go programs
skills/             Bundled skill packages
examples/           Example agent YAML configs
```

## Agent Configuration

```yaml
name: research-agent
provider: anthropic
model: claude-sonnet-4-20250514

system_prompt: |
  You are a research assistant...

skills:
  - web_search:
      backend: ddg
  - summarize

tools:
  core:
    read_file: {}
    http_fetch: {}

max_turns: 15
```

See [`examples/`](examples/) for more.

## Part of the Agent Platform

`agent-core` is the foundation of a larger platform:

| Repo | Purpose |
|---|---|
| **agent-core** (this repo) | Standalone CLI binary + Go library |
| [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) | Architecture, design docs, planning |
| **skills** | Community skill registry (coming soon) |
| **platform-api** | Go API server (coming soon) |
| **platform-web** | Next.js web portal (coming soon) |

## License

TBD
