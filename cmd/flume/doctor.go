package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/jeffdhooton/flume/internal/doctor"
)

func doctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check flume installation health",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")

			report := doctor.Run(doctor.Options{
				JSON: jsonOut,
			})

			if jsonOut {
				return doctor.PrintJSON(report)
			}
			doctor.PrintText(report)
			if !report.OK {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output as JSON")
	return cmd
}
