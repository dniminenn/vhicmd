package cmd

import (
	"fmt"
	"strings"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var (
	flagConsoleLines int
	flagConsoleURL   bool
)

var consoleCmd = &cobra.Command{
	Use:     "console <vm-id|vm-name>",
	Aliases: []string{"console-log"},
	Short:   "Print a VM's console log, or its console URL",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConsoleLines < 0 {
			return fmt.Errorf("--lines must not be negative")
		}
		if flagConsoleURL && cmd.Flags().Changed("lines") {
			return fmt.Errorf("--lines applies to the console log, not to --url")
		}

		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}

		vmID, err := api.GetVMIDByName(computeURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		if flagConsoleURL {
			consoleURL, err := api.GetConsoleURL(computeURL, tok.Value, vmID)
			if err != nil {
				return err
			}
			fmt.Println(consoleURL)
			return nil
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
	consoleCmd.Flags().BoolVar(&flagConsoleURL, "url", false, "Print the URL of the interactive (noVNC) console instead of the log")
	rootCmd.AddCommand(consoleCmd)
}
