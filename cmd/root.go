package cmd

import (
	"os"

	"github.com/bloodmagesoftware/zet/internal/options"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "zet",
	Short: "SFTP based VCS",
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&options.FlagForce, "force", "f", options.FlagForce, "Enforce a destructive action")
	rootCmd.PersistentFlags().BoolVarP(&options.FlagVerbose, "verbose", "v", options.FlagVerbose, "Write additional output to stdout")
}
