package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var flagDownloadWait time.Duration

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

var downloadVolumeCmd = &cobra.Command{
	Use:     "volume <volume_id> <output_path>",
	Aliases: []string{"vol"},
	Short:   "Download a volume to local storage",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		volumeID := args[0]
		outputPath := args[1]

		if flagDownloadWait <= 0 {
			return fmt.Errorf("--wait must be a positive duration, for example 30m")
		}

		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}

		imageURL, err := validateTokenEndpoint(tok, "image")
		if err != nil {
			return err
		}

		// Try to resolve name to ID first
		id, err := api.GetVolumeIDByName(storageURL, tok.Value, volumeID)
		if err == nil {
			volumeID = id
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %v", err)
		}

		// Get volume size
		volume, err := api.GetVolumeDetails(storageURL, tok.Value, volumeID)
		if err != nil {
			return fmt.Errorf("failed to get volume details: %v", err)
		}

		var stat syscall.Statfs_t
		if err := syscall.Statfs(filepath.Dir(outputPath), &stat); err != nil {
			return fmt.Errorf("failed to check disk space: %v", err)
		}

		availableSpace := stat.Bavail * uint64(stat.Bsize)
		if availableSpace < uint64(volume.Size*1024*1024*1024) {
			return fmt.Errorf("not enough disk space: need %d GB, have %d GB available",
				volume.Size, availableSpace/1024/1024/1024)
		}

		tempImageName := fmt.Sprintf("temp-download-%s", volumeID)

		fmt.Printf("Converting volume to image...\n")
		resp, err := api.UploadVolumeToImage(storageURL, tok.Value, volumeID, tempImageName)
		if err != nil {
			return fmt.Errorf("failed to convert volume to image: %v", err)
		}

		imageID := resp.OsVolumeUploadImage.ImageID
		fmt.Printf("Image creation started with ID: %s\n", imageID)

		// Delete the temporary image on any failure below
		cleanupTempImage := func() {
			if err := api.DeleteImage(imageURL, tok.Value, imageID); err != nil {
				fmt.Printf("Warning: failed to delete temporary image %s: %v\n", imageID, err)
			}
		}

		fmt.Printf("Waiting for image to become active (up to %s)...\n", flagDownloadWait)
		err = api.WaitForImageActive(imageURL, tok.Value, imageID, flagDownloadWait, 5*time.Second, func(msg string) {
			fmt.Println(msg)
		})
		if err != nil {
			cleanupTempImage()
			return err
		}
		fmt.Printf("Image is now active\n")

		// Download the image
		fmt.Printf("Downloading image...\n")
		err = api.DownloadImage(imageURL, tok.Value, imageID, outputPath)
		if err != nil {
			cleanupTempImage()
			return fmt.Errorf("failed to download image: %v", err)
		}

		// Delete the temporary image
		fmt.Printf("Cleaning up temporary image...\n")
		err = api.DeleteImage(imageURL, tok.Value, imageID)
		if err != nil {
			fmt.Printf("Warning: failed to delete temporary image: %v\n", err)
		}

		fmt.Printf("Volume downloaded to %s\n", outputPath)
		return nil
	},
}

func init() {
	downloadVolumeCmd.Flags().DurationVar(&flagDownloadWait, "wait", 30*time.Minute, "How long to wait for the temporary image to become active")
	downloadCmd.AddCommand(downloadImageCmd)
	downloadCmd.AddCommand(downloadVolumeCmd)
	rootCmd.AddCommand(downloadCmd)
}
