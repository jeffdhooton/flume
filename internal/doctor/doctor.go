// Package doctor runs read-only health checks for the flume installation.
package doctor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jeffdhooton/flume/internal/daemon"
)

// Status is the check result.
type Status int

const (
	Pass Status = iota
	Warn
	Fail
)

func (s Status) String() string {
	switch s {
	case Pass:
		return "PASS"
	case Warn:
		return "WARN"
	case Fail:
		return "FAIL"
	}
	return "?"
}

func (s Status) Symbol() string {
	switch s {
	case Pass:
		return "ok"
	case Warn:
		return "!!"
	case Fail:
		return "XX"
	}
	return "??"
}

// Check is one health check result.
type Check struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Status   Status `json:"status"`
	Message  string `json:"message,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

// Report is the full doctor output.
type Report struct {
	Checks []Check `json:"checks"`
	OK     bool    `json:"ok"`
}

// Options controls the doctor run.
type Options struct {
	JSON bool
	Fix  bool
}

// Run executes all health checks.
func Run(opts Options) *Report {
	home, _ := os.UserHomeDir()
	flumeHome := filepath.Join(home, ".flume")
	layout := daemon.LayoutFor(flumeHome)

	r := &Report{}

	// Environment checks.
	r.check("environment", "flume binary", func() (Status, string, string) {
		exe, err := os.Executable()
		if err != nil {
			return Fail, "cannot locate flume binary", ""
		}
		return Pass, exe, ""
	})

	r.check("environment", "~/.flume writable", func() (Status, string, string) {
		if err := os.MkdirAll(flumeHome, 0o755); err != nil {
			return Fail, err.Error(), "mkdir -p " + flumeHome
		}
		return Pass, flumeHome, ""
	})

	// Daemon checks.
	r.check("daemon", "daemon running", func() (Status, string, string) {
		alive, pid := daemon.AliveDaemon(layout)
		if !alive {
			return Warn, "not running", "flume start"
		}
		return Pass, fmt.Sprintf("pid %d", pid), ""
	})

	r.check("daemon", "socket exists", func() (Status, string, string) {
		if _, err := os.Stat(layout.SocketPath); err != nil {
			return Warn, "no socket", "flume start"
		}
		return Pass, layout.SocketPath, ""
	})

	// Proxy check.
	r.check("proxy", "proxy port reachable", func() (Status, string, string) {
		conn, err := net.DialTimeout("tcp", "localhost:8089", 500*time.Millisecond)
		if err != nil {
			return Warn, "proxy not listening on :8089", "flume start --port 8089"
		}
		_ = conn.Close()
		return Pass, "localhost:8089", ""
	})

	// Claude Code integration.
	r.check("claude", "claude CLI on PATH", func() (Status, string, string) {
		path, err := exec.LookPath("claude")
		if err != nil {
			return Warn, "claude not found", "install Claude Code CLI"
		}
		return Pass, path, ""
	})

	r.check("claude", "MCP registration", func() (Status, string, string) {
		claudeBin, err := exec.LookPath("claude")
		if err != nil {
			return Warn, "cannot check (claude not on PATH)", ""
		}
		cmd := exec.Command(claudeBin, "mcp", "get", "flume")
		out, err := cmd.CombinedOutput()
		if err != nil || len(out) == 0 {
			return Warn, "flume not registered as MCP server", "flume setup"
		}
		return Pass, "registered", ""
	})

	// Compute overall status.
	r.OK = true
	for _, c := range r.Checks {
		if c.Status == Fail {
			r.OK = false
			break
		}
	}

	return r
}

func (r *Report) check(category, name string, fn func() (Status, string, string)) {
	status, msg, fix := fn()
	r.Checks = append(r.Checks, Check{
		Category: category,
		Name:     name,
		Status:   status,
		Message:  msg,
		Fix:      fix,
	})
}

// PrintText prints a human-readable checklist to stderr.
func PrintText(r *Report) {
	lastCat := ""
	for _, c := range r.Checks {
		if c.Category != lastCat {
			if lastCat != "" {
				fmt.Fprintln(os.Stderr)
			}
			fmt.Fprintf(os.Stderr, "  %s\n", c.Category)
			lastCat = c.Category
		}
		fmt.Fprintf(os.Stderr, "    [%s] %s: %s\n", c.Status.Symbol(), c.Name, c.Message)
		if c.Fix != "" && c.Status != Pass {
			fmt.Fprintf(os.Stderr, "          fix: %s\n", c.Fix)
		}
	}
	fmt.Fprintln(os.Stderr)
	if r.OK {
		fmt.Fprintln(os.Stderr, "  all checks passed")
	} else {
		fmt.Fprintln(os.Stderr, "  some checks failed")
	}
}

// PrintJSON prints machine-readable output to stdout.
func PrintJSON(r *Report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
