package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/jeffdhooton/flume/internal/store"
)

func requestsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "List captured HTTP requests",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			method, _ := cmd.Flags().GetString("method")
			limit, _ := cmd.Flags().GetInt("limit")

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var res []store.RequestSummary
			if err := callDaemon(ctx, "requests", &store.ListFilter{
				Path:   path,
				Method: method,
				Limit:  limit,
			}, &res); err != nil {
				return err
			}
			pretty, _ := cmd.Flags().GetBool("pretty")
			return printJSON(res, pretty)
		},
	}
	cmd.Flags().String("path", "", "filter by URL path (substring match)")
	cmd.Flags().String("method", "", "filter by HTTP method")
	cmd.Flags().Int("limit", 20, "max results")
	return cmd
}
