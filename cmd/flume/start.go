package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jeffdhooton/flume/internal/daemon"
)

func startCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the flume daemon",
		Long: `Start the flumed daemon and reverse proxy. With no flags, flume start
detaches a background process and returns. With --foreground, flumed runs
in the calling shell.

The CLI auto-spawns the daemon on first query, so manual start is only
needed for inspecting logs or running under a supervisor.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			foreground, _ := cmd.Flags().GetBool("foreground")
			port, _ := cmd.Flags().GetInt("port")
			target, _ := cmd.Flags().GetString("target")

			home, err := flumeHome()
			if err != nil {
				return err
			}
			layout := daemon.LayoutFor(home)

			cfg := daemon.DefaultConfig()
			cfg.ProxyPort = port
			cfg.TargetAddr = target

			if foreground {
				d := daemon.New(layout, cfg)
				return d.Run(context.Background())
			}

			if alive, pid := daemon.AliveDaemon(layout); alive {
				fmt.Fprintf(os.Stderr, "flume: daemon already running (pid %d)\n", pid)
				return nil
			}

			// Pass port and target through to the foreground child.
			extra := []string{
				"--port", strconv.Itoa(port),
				"--target", target,
			}
			if err := spawnDaemon(extra...); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "flume: daemon spawned")
			return nil
		},
	}
	cmd.Flags().Bool("foreground", false, "run in the foreground (do not detach)")
	cmd.Flags().Int("port", 8089, "proxy listen port")
	cmd.Flags().String("target", "localhost:8000", "upstream dev server address")
	return cmd
}
