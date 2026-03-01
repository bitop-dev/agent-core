# AGENTS.md

Instructions for AI coding agents working with this codebase.

## Project Overview

`agent-core` is a standalone Go binary for running AI agents. It's the foundation of the Agent Platform — see the [planning docs](https://github.com/bitop-dev/agent-platform-docs) for the full architecture.

## Module Structure

```
github.com/bitop-dev/agent-core

cmd/agent-core/        CLI entrypoint (cobra)
internal/
  agent/               Turn loop, events, context management, loop detection
  provider/            LLM provider interface + implementations (Anthropic, OpenAI, etc.)
  tool/                Tool interface, ToolEngine, subprocess runner, sandbox
  tool/builtin/        Core tools: bash, read_file, write_file, edit_file, list_dir, grep, http_fetch, tasks
  mcp/                 MCP client (stdio + HTTP/SSE transports)
  models/              Model catalog (context windows, costs) + CostTracker
  observer/            Observer interface: Noop, Log, Cost, Multi
  session/             Session persistence (JSONL files)
  skill/               Skill loader (SKILL.md parser, eligibility, snapshot)
  config/              AgentConfig YAML parsing + validation
  output/              Renderers: text (colored), JSON, JSONL, quiet
pkg/agent/             Public API for embedding (what platform-api imports)
skills/                Bundled skill packages
examples/              Example agent YAML configs
```

## Key Interfaces

- `provider.Provider` — LLM provider (Complete method returns streaming events)
- `tool.Tool` — Any callable tool (Definition + Execute)
- `tool.Engine` — Registers tools, dispatches calls in parallel
- `observer.Observer` — Receives telemetry events during runs
- `output.Renderer` — Renders RunEvents to terminal/file

## Conventions

- No global state — everything injected via constructors
- `context.Context` on all blocking operations
- Builder pattern for Agent construction
- Errors return `error`, not panic
- Internal packages for implementation, `pkg/` for public API

## Build

```bash
make build    # → bin/agent-core
make test     # run all tests
make lint     # golangci-lint
```

## Planning Docs

Full design documentation: https://github.com/bitop-dev/agent-platform-docs/tree/main/BLDER_DOCS
