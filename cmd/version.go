package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"     // set by the release build
	buildTime = "unknown" // set by Makefile
	goversion = runtime.Version()
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"v"},
	Short:   "Print build information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\nBuild: %s\nGo: %s\n", version, buildTime, goversion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
