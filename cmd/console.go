package cmd

import (
	"fmt"
	"strings"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var flagConsoleLines int

var consoleCmd = &cobra.Command{
	Use:     "console <vm-id|vm-name>",
	Aliases: []string{"console-log"},
	Short:   "Print a VM's console log",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConsoleLines < 0 {
			return fmt.Errorf("--lines must not be negative")
		}

		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}

		vmID, err := api.GetVMIDByName(computeURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		output, err := api.GetConsoleOutput(computeURL, tok.Value, vmID, flagConsoleLines)
		if err != nil {
			return err
		}

		fmt.Print(output)
		if output != "" && !strings.HasSuffix(output, "\n") {
			fmt.Println()
		}
		return nil
	},
}

func init() {
	consoleCmd.Flags().IntVarP(&flagConsoleLines, "lines", "n", 0, "Print only the last N lines (default: the whole log)")
	rootCmd.AddCommand(consoleCmd)
}
