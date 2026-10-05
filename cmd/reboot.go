package cmd

import (
	"fmt"
	"strings"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var flagRebootYes bool

var rebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot a virtual machine",
}

var hardRebootCmd = &cobra.Command{
	Use:   "hard <vm-id|vm-name>",
	Short: "Perform a hard reboot on a VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return rebootVM(args[0], "HARD")
	},
}

var softRebootCmd = &cobra.Command{
	Use:   "soft <vm-id|vm-name>",
	Short: "Perform a soft reboot on a VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return rebootVM(args[0], "SOFT")
	},
}

// rebootVM resolves the VM, shows its name and ID, and asks before it reboots unless --yes is set.
func rebootVM(vm, kind string) error {
	computeURL, err := validateTokenEndpoint(tok, "compute")
	if err != nil {
		return err
	}

	vmID, err := api.GetVMIDByName(computeURL, tok.Value, vm)
	if err != nil {
		return err
	}
	vmName, err := api.GetVMNameByID(computeURL, tok.Value, vmID)
	if err != nil {
		return fmt.Errorf("VM %s not found: %v", vmID, err)
	}

	label := strings.ToLower(kind)
	if !flagRebootYes {
		ok, err := readConfirmation(fmt.Sprintf("%s reboot VM %s (%s)? [y/N] ", label, vmName, vmID))
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("reboot cancelled")
		}
	}

	if err := api.RebootVM(computeURL, tok.Value, vmID, kind); err != nil {
		return err
	}

	fmt.Printf("%s reboot initiated for VM %s (%s)\n", strings.ToUpper(label[:1])+label[1:], vmName, vmID)
	return nil
}

func init() {
	rebootCmd.PersistentFlags().BoolVarP(&flagRebootYes, "yes", "y", false, "Skip the confirmation prompt (for scripts)")
	rebootCmd.AddCommand(hardRebootCmd)
	rebootCmd.AddCommand(softRebootCmd)
	rootCmd.AddCommand(rebootCmd)
}
