package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bitop-dev/agent-core/internal/config"
	"github.com/bitop-dev/agent-core/internal/mcp"
)

func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server management",
	}

	cmd.AddCommand(mcpTestCmd())
	return cmd
}

func mcpTestCmd() *cobra.Command {
	var (
		transport string
		command   string
		url       string
		headers   []string
		name      string
	)

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test connection to an MCP server",
		Long: `Connect to an MCP server, perform the handshake, and list available tools.

Examples:
  # Test a stdio server (local process)
  agent-core mcp test --transport stdio --command "npx @tanstack/cli mcp" --name tanstack

  # Test an HTTP server
  agent-core mcp test --transport http --url https://mcp.context7.com/mcp --header "CONTEXT7_API_KEY=..." --name context7`,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := config.MCPServer{
				Name:      name,
				Transport: transport,
				URL:       url,
			}

			if command != "" {
				srv.Command = strings.Fields(command)
			}

			if len(headers) > 0 {
				srv.Headers = make(map[string]string)
				for _, h := range headers {
					k, v, ok := strings.Cut(h, "=")
					if !ok {
						return fmt.Errorf("invalid header %q (expected KEY=VALUE)", h)
					}
					srv.Headers[k] = v
				}
			}

			if transport == "" {
				if len(srv.Command) > 0 {
					srv.Transport = "stdio"
				} else if srv.URL != "" {
					srv.Transport = "http"
				} else {
					return fmt.Errorf("specify --transport, --command, or --url")
				}
			}

			fmt.Fprintf(os.Stderr, "Connecting to MCP server %q (%s)...\n", srv.Name, srv.Transport)

			client, err := mcp.Connect(srv)
			if err != nil {
				return fmt.Errorf("connect failed: %w", err)
			}
			defer client.Close()

			tools := client.Tools()
			fmt.Fprintf(os.Stderr, "✓ Connected — %d tool(s) available:\n\n", len(tools))

			for _, t := range tools {
				desc := t.Description
				if len(desc) > 80 {
					desc = desc[:80] + "…"
				}
				fmt.Printf("  %-40s %s\n", t.Name, desc)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&transport, "transport", "t", "", "Transport: stdio, http, sse")
	cmd.Flags().StringVar(&command, "command", "", "Command to spawn (stdio transport)")
	cmd.Flags().StringVar(&url, "url", "", "Server URL (http/sse transport)")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "HTTP headers (KEY=VALUE)")
	cmd.Flags().StringVarP(&name, "name", "n", "test", "Server name")

	return cmd
}
