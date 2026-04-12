// Package setup registers flume as an MCP server with Claude Code.
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options controls the install behavior.
type Options struct {
	FlumeBinary string // absolute path to the flume binary; defaults to os.Executable()
	DryRun      bool
	Force       bool
}

// Result summarizes what Install did.
type Result struct {
	MCPAction      string // "registered" | "replaced" | "unchanged" | "dry-run" | "manual"
	MCPCommand     string // the `claude mcp add ...` command we ran (or would run)
	MCPBinary      string
	ClaudeCLIFound bool
}

// Install registers flume as an MCP server via `claude mcp add`.
func Install(opts Options) (*Result, error) {
	res := &Result{}

	bin := opts.FlumeBinary
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate flume binary: %w", err)
		}
		bin = exe
	}
	if abs, err := filepath.Abs(bin); err == nil {
		bin = abs
	}
	res.MCPBinary = bin

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		res.ClaudeCLIFound = false
		res.MCPCommand = fmt.Sprintf("claude mcp add --scope user --transport stdio flume -- %q mcp", bin)
		res.MCPAction = "manual"
		return res, nil
	}
	res.ClaudeCLIFound = true
	res.MCPCommand = fmt.Sprintf("%s mcp add --scope user --transport stdio flume -- %q mcp", claudeBin, bin)

	// Check current registration.
	current, currentErr := runClaudeMCP(claudeBin, "get", "flume")
	hasFlume := currentErr == nil && len(current) > 0
	commandMatches := hasFlume && strings.Contains(current, bin) && strings.Contains(current, " mcp")

	if commandMatches && !opts.Force {
		res.MCPAction = "unchanged"
		return res, nil
	}

	if opts.DryRun {
		if hasFlume {
			res.MCPAction = "replaced (dry-run)"
		} else {
			res.MCPAction = "registered (dry-run)"
		}
		return res, nil
	}

	if hasFlume {
		if _, err := runClaudeMCP(claudeBin, "remove", "flume"); err != nil {
			return res, fmt.Errorf("remove existing flume entry: %w", err)
		}
	}

	cmd := exec.Command(claudeBin, "mcp", "add", "--scope", "user", "--transport", "stdio", "flume", "--", bin, "mcp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return res, fmt.Errorf("claude mcp add: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if hasFlume {
		res.MCPAction = "replaced"
	} else {
		res.MCPAction = "registered"
	}
	return res, nil
}

func runClaudeMCP(claudeBin string, args ...string) (string, error) {
	cmd := exec.Command(claudeBin, append([]string{"mcp"}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
