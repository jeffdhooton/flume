package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jeffdhooton/flume/internal/setup"
)

func setupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Register flume as an MCP server with Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			force, _ := cmd.Flags().GetBool("force")
			bin, _ := cmd.Flags().GetString("flume-binary")

			res, err := setup.Install(setup.Options{
				FlumeBinary: bin,
				DryRun:      dryRun,
				Force:       force,
			})
			if err != nil {
				return err
			}

			printSetupResult(res)
			return nil
		},
	}
	cmd.Flags().Bool("dry-run", false, "show what would be done without doing it")
	cmd.Flags().Bool("force", false, "re-register even if already configured")
	cmd.Flags().String("flume-binary", "", "path to flume binary (defaults to current executable)")
	return cmd
}

func printSetupResult(res *setup.Result) {
	fmt.Fprintf(os.Stderr, "flume setup:\n")
	fmt.Fprintf(os.Stderr, "  Skill: %s → %s\n", res.SkillAction, res.SkillPath)
	fmt.Fprintf(os.Stderr, "  MCP server: %s\n", res.MCPAction)
	fmt.Fprintf(os.Stderr, "  Binary: %s\n", res.MCPBinary)
	if !res.ClaudeCLIFound {
		fmt.Fprintf(os.Stderr, "\n  Claude CLI not found. Run this manually:\n")
		fmt.Fprintf(os.Stderr, "    %s\n", res.MCPCommand)
	}
}
