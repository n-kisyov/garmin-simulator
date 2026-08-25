package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fitsim",
	Short: "A tool to generate Garmin FIT activity files",
	Long:  `fitsim generates various Garmin activity FIT files (run, cycle, etc) for testing and simulation.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
