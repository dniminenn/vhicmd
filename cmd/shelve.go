package cmd

import (
	"fmt"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var flagShelveYes bool

var shelveCmd = &cobra.Command{
	Use:   "shelve <vm-id|vm-name>",
	Short: "Shelve a VM",
	Long:  "Shelve a VM: it is shut down and its compute resources are released; disks, ports and addresses are kept.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		computeURL, vmID, vmName, err := resolveVM(args[0])
		if err != nil {
			return err
		}

		if !flagShelveYes {
			ok, err := readConfirmation(fmt.Sprintf("shelve VM %s (%s)? [y/N] ", vmName, vmID))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("shelve cancelled")
			}
		}

		if err := api.ShelveVM(computeURL, tok.Value, vmID); err != nil {
			return err
		}

		fmt.Printf("Shelve requested for VM %s (%s)\n", vmName, vmID)
		return nil
	},
}

var unshelveCmd = &cobra.Command{
	Use:   "unshelve <vm-id|vm-name>",
	Short: "Unshelve a VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		computeURL, vmID, vmName, err := resolveVM(args[0])
		if err != nil {
			return err
		}

		if err := api.UnshelveVM(computeURL, tok.Value, vmID); err != nil {
			return err
		}

		fmt.Printf("Unshelve requested for VM %s (%s)\n", vmName, vmID)
		return nil
	},
}

func init() {
	shelveCmd.Flags().BoolVarP(&flagShelveYes, "yes", "y", false, "Skip the confirmation prompt (for scripts)")
	rootCmd.AddCommand(shelveCmd)
	rootCmd.AddCommand(unshelveCmd)
}
