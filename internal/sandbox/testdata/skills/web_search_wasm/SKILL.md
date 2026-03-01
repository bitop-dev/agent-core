---
name: web_search_wasm
version: "1.0.0"
description: "Search the web using DuckDuckGo. WASM-sandboxed — no Python, no pip, no dependencies."
author: bitop-dev
tags: [search, web, duckduckgo, wasm]
emoji: "🔍"
runtime: wasm
---

# Web Search (WASM)

Search the web using DuckDuckGo's HTML endpoint. Results include title, URL, and snippet.

This tool runs as a sandboxed WebAssembly module — no external dependencies needed.
Network access is gated by the sandbox's AllowedHosts policy.

## Usage

The agent can call `web_search_wasm` with a search query:

```json
{"query": "golang wasm runtime", "max_results": 5}
```

## Capabilities Required

- **Network**: `html.duckduckgo.com` must be in AllowedHosts
