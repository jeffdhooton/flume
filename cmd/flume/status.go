package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/jeffdhooton/flume/internal/daemon"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var res daemon.StatusResult
			if err := callDaemon(ctx, "status", nil, &res); err != nil {
				return err
			}
			pretty, _ := cmd.Flags().GetBool("pretty")
			return printJSON(res, pretty)
		},
	}
}
