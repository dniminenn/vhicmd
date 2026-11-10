package cmd

import (
	"fmt"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var flagStopYes bool

var startCmd = &cobra.Command{
	Use:   "start <vm-id|vm-name>",
	Short: "Power on a VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		computeURL, vmID, vmName, err := resolveVM(args[0])
		if err != nil {
			return err
		}

		if err := api.StartVM(computeURL, tok.Value, vmID); err != nil {
			return err
		}

		fmt.Printf("Start requested for VM %s (%s)\n", vmName, vmID)
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop <vm-id|vm-name>",
	Short: "Shut down a VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		computeURL, vmID, vmName, err := resolveVM(args[0])
		if err != nil {
			return err
		}

		if !flagStopYes {
			ok, err := readConfirmation(fmt.Sprintf("stop VM %s (%s)? [y/N] ", vmName, vmID))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("stop cancelled")
			}
		}

		if err := api.RequestStopVM(computeURL, tok.Value, vmID); err != nil {
			return err
		}

		fmt.Printf("Stop requested for VM %s (%s)\n", vmName, vmID)
		return nil
	},
}

// resolveVM returns the compute endpoint and the VM's ID and name for an ID or name argument.
func resolveVM(vm string) (computeURL, vmID, vmName string, err error) {
	computeURL, err = validateTokenEndpoint(tok, "compute")
	if err != nil {
		return "", "", "", err
	}

	vmID, err = api.GetVMIDByName(computeURL, tok.Value, vm)
	if err != nil {
		return "", "", "", err
	}
	vmName, err = api.GetVMNameByID(computeURL, tok.Value, vmID)
	if err != nil {
		return "", "", "", fmt.Errorf("VM %s not found: %v", vmID, err)
	}
	return computeURL, vmID, vmName, nil
}

func init() {
	stopCmd.Flags().BoolVarP(&flagStopYes, "yes", "y", false, "Skip the confirmation prompt (for scripts)")
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
}
