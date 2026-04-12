package main

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jeffdhooton/flume/internal/daemon"
)

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running flume daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := flumeHome()
			if err != nil {
				return err
			}
			layout := daemon.LayoutFor(home)

			alive, pid := daemon.AliveDaemon(layout)
			if !alive {
				fmt.Fprintln(os.Stderr, "flume: no daemon running")
				return nil
			}

			// Try clean shutdown via RPC; fall back to SIGTERM.
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := callDaemon(ctx, "shutdown", nil, nil); err != nil {
				if pid > 0 {
					_ = syscall.Kill(pid, syscall.SIGTERM)
				}
			}

			// Wait for socket to disappear.
			deadline := time.Now().Add(daemon.DefaultShutdownGrace)
			for time.Now().Before(deadline) {
				if alive, _ := daemon.AliveDaemon(layout); !alive {
					fmt.Fprintln(os.Stderr, "flume: daemon stopped")
					return nil
				}
				time.Sleep(50 * time.Millisecond)
			}

			// Force-kill after grace.
			if pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			fmt.Fprintln(os.Stderr, "flume: daemon force-killed after grace period")
			return nil
		},
	}
}
