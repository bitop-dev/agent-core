package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bitop-dev/agent-core/internal/session"
)

func sessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage chat sessions",
	}

	cmd.AddCommand(sessionsListCmd())
	cmd.AddCommand(sessionsShowCmd())
	cmd.AddCommand(sessionsDeleteCmd())
	cmd.AddCommand(sessionsClearCmd())

	return cmd
}

func sessionsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := session.NewStore(session.DefaultDir())
			if err != nil {
				return err
			}

			infos, err := store.List()
			if err != nil {
				return err
			}

			if len(infos) == 0 {
				fmt.Println("No saved sessions.")
				return nil
			}

			fmt.Printf("%-12s  %-20s  %s\n", "ID", "Modified", "Size")
			fmt.Printf("%-12s  %-20s  %s\n", "---", "---", "---")
			for _, info := range infos {
				fmt.Printf("%-12s  %-20s  %d bytes\n",
					info.ID,
					info.Modified.Format("2006-01-02 15:04:05"),
					info.Size,
				)
			}
			return nil
		},
	}
}

func sessionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [session-id]",
		Short: "Show session details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := session.NewStore(session.DefaultDir())
			if err != nil {
				return err
			}

			sess, err := store.Load(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Session: %s\n", sess.ID)
			fmt.Printf("Created: %s\n", sess.CreatedAt.Format("2006-01-02 15:04:05"))
			if model, ok := sess.Metadata["model"]; ok {
				fmt.Printf("Model:   %s\n", model)
			}
			if agent, ok := sess.Metadata["agent"]; ok && agent != "" {
				fmt.Printf("Agent:   %s\n", agent)
			}
			fmt.Printf("Messages: %d\n\n", len(sess.Messages))

			for i, msg := range sess.Messages {
				role := string(msg.Role)
				text := ""
				for _, b := range msg.Content {
					switch b.Type {
					case "text":
						text = b.Text
					case "tool_call":
						text = fmt.Sprintf("[tool_call: %s]", b.ToolName)
					case "tool_result":
						text = fmt.Sprintf("[tool_result: %s]", truncate(b.Text, 80))
					}
				}
				if len(text) > 120 {
					text = text[:120] + "..."
				}
				fmt.Printf("  [%d] %s: %s\n", i+1, role, text)
			}
			return nil
		},
	}
}

func sessionsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [session-id]",
		Short: "Delete a saved session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := session.NewStore(session.DefaultDir())
			if err != nil {
				return err
			}
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted session: %s\n", args[0])
			return nil
		},
	}
}

func sessionsClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Delete all saved sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := session.NewStore(session.DefaultDir())
			if err != nil {
				return err
			}
			infos, err := store.List()
			if err != nil {
				return err
			}
			for _, info := range infos {
				store.Delete(info.ID)
			}
			fmt.Printf("Deleted %d sessions.\n", len(infos))
			return nil
		},
	}
}
