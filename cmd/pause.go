package cmd

import (
	"fmt"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause a VM",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("requires exactly one argument (VM name or ID)")
		}

		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}

		vmID, err := api.GetVMIDByName(computeURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		return api.PauseVM(computeURL, tok.Value, vmID)
	},
}

var unpauseCmd = &cobra.Command{
	Use:   "unpause",
	Short: "Unpause a VM",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("requires exactly one argument (VM name or ID)")
		}

		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}

		vmID, err := api.GetVMIDByName(computeURL, tok.Value, args[0])
		if err != nil {
			return err
		}

		return api.UnpauseVM(computeURL, tok.Value, vmID)
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd)
	rootCmd.AddCommand(unpauseCmd)
}
