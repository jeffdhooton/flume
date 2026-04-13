// Package setup installs the flume Claude Code integration: writes the
// embedded SKILL.md to ~/.claude/skills/flume/SKILL.md and registers flume as
// an MCP server via `claude mcp add`.
//
// Design notes:
//
//   - The SKILL.md is version-controlled at internal/setup/SKILL.md and
//     embedded via go:embed so upgrading flume automatically upgrades the
//     skill. Users who want to customize it can edit the installed copy;
//     `flume setup --force` will overwrite it again.
//
//   - MCP registration is done by shelling out to `claude mcp add`, which is
//     Claude Code's official CLI for managing MCP servers. This is MUCH safer
//     than hand-editing ~/.claude.json.
package setup

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed SKILL.md
var embeddedSkill []byte

// Options controls the install behavior.
type Options struct {
	FlumeBinary string // absolute path to the flume binary; defaults to os.Executable()
	DryRun      bool
	Force       bool
}

// Result summarizes what Install did.
type Result struct {
	SkillPath      string // absolute path to the installed SKILL.md
	SkillAction    string // "written" | "unchanged" | "dry-run"
	MCPAction      string // "registered" | "replaced" | "unchanged" | "dry-run" | "manual"
	MCPCommand     string // the `claude mcp add ...` command we ran (or would run)
	MCPBinary      string
	ClaudeCLIFound bool
}

// Install performs the full Claude Code integration: SKILL.md + MCP server.
func Install(opts Options) (*Result, error) {
	claudeHome, err := claudeHomeDir()
	if err != nil {
		return nil, err
	}
	res := &Result{
		SkillPath: filepath.Join(claudeHome, "skills", "flume", "SKILL.md"),
	}

	if err := installSkill(opts, res); err != nil {
		return res, fmt.Errorf("install skill: %w", err)
	}

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

// claudeHomeDir returns ~/.claude, creating it if missing.
func claudeHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create claude home: %w", err)
	}
	return dir, nil
}

// ---------------- SKILL.md ----------------

func installSkill(opts Options, res *Result) error {
	dir := filepath.Dir(res.SkillPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	existing, readErr := os.ReadFile(res.SkillPath)
	if readErr == nil && bytesEqual(existing, embeddedSkill) && !opts.Force {
		res.SkillAction = "unchanged"
		return nil
	}

	if opts.DryRun {
		res.SkillAction = "dry-run"
		return nil
	}

	// Atomic replace: write to temp sibling, rename over the target.
	tmp, err := os.CreateTemp(dir, "SKILL.md.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(embeddedSkill); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, res.SkillPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	res.SkillAction = "written"
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// EmbeddedSkill returns the embedded SKILL.md bytes for tests and tooling.
func EmbeddedSkill() []byte { return embeddedSkill }
