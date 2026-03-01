// Package agent is the public API for embedding agent-core in other Go programs.
// This is what platform-api imports.
//
//	import "github.com/bitop-dev/agent-core/pkg/agent"
//
// This package re-exports the essential types and functions from internal packages,
// providing a stable API surface while keeping implementation details internal.
package agent

// TODO: Re-export Agent, Builder, RunEvent, Config types from internal packages
// This will be finalized once the internal API stabilizes.
