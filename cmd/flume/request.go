package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/jeffdhooton/flume/internal/daemon"
	"github.com/jeffdhooton/flume/internal/store"
)

func requestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "request <id>",
		Short: "Show full detail for a captured request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var res store.Request
			if err := callDaemon(ctx, "request", &daemon.RequestParams{
				ID: args[0],
			}, &res); err != nil {
				return err
			}
			pretty, _ := cmd.Flags().GetBool("pretty")
			return printJSON(res, pretty)
		},
	}
}
