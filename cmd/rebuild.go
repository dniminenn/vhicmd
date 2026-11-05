package cmd

import (
	"fmt"
	"time"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var (
	rebuildImage   string
	rebuildYes     bool
	rebuildTimeout time.Duration
)

// The VM keeps its ports and addresses; its disk becomes the image (a boot volume is re-imaged).
var rebuildCmd = &cobra.Command{
	Use:   "rebuild <vm-name-or-id> --image <image-name-or-id>",
	Short: "Rebuild a VM from an image (its disk is replaced, its addresses stay)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if rebuildImage == "" {
			return fmt.Errorf("--image is required")
		}
		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}
		imageURL, err := validateTokenEndpoint(tok, "image")
		if err != nil {
			return err
		}
		vmID, err := api.GetVMIDByName(computeURL, tok.Value, args[0])
		if err != nil {
			return fmt.Errorf("VM %s: %v", args[0], err)
		}
		imageID, err := api.GetImageIDByName(imageURL, tok.Value, rebuildImage)
		if err != nil {
			return fmt.Errorf("image %s: %v", rebuildImage, err)
		}
		if !rebuildYes {
			ok, err := readConfirmation(fmt.Sprintf("Rebuild %s (%s) from %s (%s)? Everything on its disk is lost. [y/N] ", args[0], vmID, rebuildImage, imageID))
			if err != nil || !ok {
				return fmt.Errorf("not confirmed")
			}
		}
		fmt.Printf("Rebuilding %s from %s...\n", vmID, imageID)
		if err := api.RebuildVM(computeURL, tok.Value, vmID, imageID, rebuildTimeout); err != nil {
			return err
		}
		fmt.Printf("VM %s is ACTIVE on image %s\n", vmID, imageID)
		return nil
	},
}

func init() {
	rebuildCmd.Flags().StringVar(&rebuildImage, "image", "", "image name or ID")
	rebuildCmd.Flags().BoolVarP(&rebuildYes, "yes", "y", false, "do not ask for confirmation")
	rebuildCmd.Flags().DurationVar(&rebuildTimeout, "timeout", 30*time.Minute, "how long to wait for ACTIVE")
	rootCmd.AddCommand(rebuildCmd)
}
