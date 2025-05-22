package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:     "download",
	Aliases: []string{"dl"},
	Short:   "Download resources",
}

var downloadImageCmd = &cobra.Command{
	Use:     "image <image_id> <output_path>",
	Aliases: []string{"img"},
	Short:   "Download an image to local storage",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageID := args[0]
		outputPath := args[1]

		imageURL, err := validateTokenEndpoint(tok, "image")
		if err != nil {
			return err
		}

		// Try to resolve name to ID first
		id, err := api.GetImageIDByName(imageURL, tok.Value, imageID)
		if err == nil {
			imageID = id
		}

		// Create parent directories if they don't exist
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %v", err)
		}

		err = api.DownloadImage(imageURL, tok.Value, imageID, outputPath)
		if err != nil {
			return err
		}

		fmt.Printf("Image downloaded to %s\n", outputPath)
		return nil
	},
}

func init() {
	downloadCmd.AddCommand(downloadImageCmd)
	rootCmd.AddCommand(downloadCmd)
}
