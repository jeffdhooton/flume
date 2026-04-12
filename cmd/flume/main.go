// Command flume is the runtime visibility CLI for AI agents.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// Version is set by ldflags during release builds.
var Version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "flume",
		Short: "Runtime visibility daemon for AI agents",
		Long: `flume is a runtime visibility daemon for AI agents. It captures HTTP
requests and responses from your dev server via a reverse proxy and
exposes them as millisecond-latency MCP queries — replacing the
guess-log-reproduce-read debugging cycle.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().Bool("pretty", false, "pretty-print JSON output")

	root.AddCommand(versionCmd())
	root.AddCommand(startCmd())
	root.AddCommand(stopCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(requestsCmd())
	root.AddCommand(requestCmd())
	root.AddCommand(mcpCmd())
	root.AddCommand(setupCmd())
	root.AddCommand(doctorCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "flume:", err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print flume version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("flume", Version)
			return nil
		},
	}
}

// flumeHome returns ~/.flume, creating it if missing.
func flumeHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	dir := filepath.Join(home, ".flume")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create flume home: %w", err)
	}
	return dir, nil
}

func printJSON(v any, pretty bool) error {
	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}
