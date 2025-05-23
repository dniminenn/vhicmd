package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create resources like VMs or volumes",
}

// Subcommand: create Image (and upload data)
var createImageCmd = &cobra.Command{
	Use:     "image",
	Aliases: []string{"img"},
	Short:   "Create a new image",
	Long:    "Create a new image (.qcow2, .raw, .vmdk, .iso) or from a VM instance",
	RunE: func(cmd *cobra.Command, args []string) error {
		imageURL, err := validateTokenEndpoint(tok, "image")
		if err != nil {
			return err
		}

		// Check if we're creating from an instance
		// Check if we're creating from an instance
		if flagInstanceID != "" {
			computeURL, err := validateTokenEndpoint(tok, "compute")
			if err != nil {
				return err
			}

			storageURL, err := validateTokenEndpoint(tok, "volumev3")
			if err != nil {
				return err
			}

			// Verify the instance exists
			id, err := api.GetVMIDByName(computeURL, tok.Value, flagInstanceID)
			if err == nil {
				flagInstanceID = id
			} else {
				return fmt.Errorf("instance not found: %v", err)
			}

			// If no name provided, use instance name + timestamp
			if flagImageName == "" {
				vmName, err := api.GetVMNameByID(computeURL, tok.Value, flagInstanceID)
				if err != nil {
					return fmt.Errorf("failed to get VM name: %v", err)
				}
				flagImageName = fmt.Sprintf("%s-%s", vmName, time.Now().Format("20060102-150405"))
			}

			// Get the boot volume ID from the VM
			fmt.Printf("Getting boot volume for instance %s...\n", flagInstanceID)
			volumeID, err := api.GetVMBootVolume(computeURL, tok.Value, flagInstanceID)
			if err != nil {
				return fmt.Errorf("failed to get boot volume: %v", err)
			}
			fmt.Printf("Boot volume ID: %s\n", volumeID)

			// Upload the volume to create an image
			fmt.Printf("Creating image from volume...\n")
			resp, err := api.UploadVolumeToImage(storageURL, tok.Value, volumeID, flagImageName)
			if err != nil {
				return fmt.Errorf("failed to create image from volume: %v", err)
			}

			imageID := resp.OsVolumeUploadImage.ImageID
			fmt.Printf("Image creation started with ID: %s\n", imageID)

			// Verify that we have a valid image ID
			if imageID == "" {
				return fmt.Errorf("failed to get a valid image ID from volume upload")
			}

			fmt.Printf("Waiting for image (ID: %s) to become active...\n", imageID)

			// Monitor image status until it's active
			maxAttempts := 60 // 5 minutes with 5 second intervals
			var lastStatus string
			for attempt := 0; attempt < maxAttempts; attempt++ {
				// List all images and find the one with our ID
				images, err := api.ListImages(imageURL, tok.Value, map[string]string{"id": imageID})
				if err != nil {
					fmt.Printf("Warning: Error checking image status: %v (retrying...)\n", err)
					time.Sleep(5 * time.Second)
					continue
				}

				var found bool
				var status string

				for _, img := range images.Images {
					if img.ID == imageID {
						found = true
						status = img.Status
						break
					}
				}

				if !found {
					if lastStatus != "not_found" {
						fmt.Printf("Image not found yet, waiting...\n")
						lastStatus = "not_found"
					}
					time.Sleep(5 * time.Second)
					continue
				}

				if status != lastStatus {
					fmt.Printf("Image status: %s\n", status)
					lastStatus = status
				}

				if status == "active" {
					fmt.Printf("Image is now active\n")
					break
				} else if status == "error" {
					return fmt.Errorf("image creation failed with status: error")
				}

				if attempt == maxAttempts-1 {
					return fmt.Errorf("timed out waiting for image to become active")
				}

				time.Sleep(5 * time.Second)
			}

			fmt.Printf("Image created successfully: ID: %s, Name: %s\n", imageID, flagImageName)
			return nil
		}

		// If not creating from instance, proceed with regular image creation from file
		// Validate file exists
		if _, err := os.Stat(flagImageFile); os.IsNotExist(err) {
			return fmt.Errorf("image file not found: %s", flagImageFile)
		}

		// Determine format from flag or file extension if not specified
		format := flagDiskFormat
		if format == "" {
			ext := strings.ToLower(filepath.Ext(flagImageFile))
			switch ext {
			case ".qcow2":
				format = "qcow2"
			case ".raw":
				format = "raw"
			case ".vmdk":
				format = "vmdk"
			case ".iso":
				format = "iso"
			default:
				return fmt.Errorf("unsupported image format %s, must specify --format flag", ext)
			}
		}

		switch format {
		case "qcow2", "raw", "vmdk", "iso":
			// Valid formats
		default:
			return fmt.Errorf("unsupported format %s, must be qcow2, raw, vmdk or iso", format)
		}

		// --- Begin really dumb stuff ---------------------------
		// This is a hack to warm up the NFS on the VMDK stores in
		// /mnt/vmdk, have no clue why this is necessary
		// -------------------------------------------------------
		if format == "vmdk" && strings.HasPrefix(flagImageFile, "/mnt/vmdk/") {
			cmd := exec.Command("dd", "if="+flagImageFile, "of=/dev/null", "bs=1M", "count=1", "status=progress")
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start warmup read: %v", err)
			}

			time.Sleep(2 * time.Second)

			psCmd := exec.Command("ps", "-p", fmt.Sprintf("%d", cmd.Process.Pid), "-o", "state=,cmd=")
			output, err := psCmd.Output()
			if err != nil {
				cmd.Process.Kill()
				return fmt.Errorf("failed to check process state: %v", err)
			}

			parts := strings.Fields(string(output))
			if len(parts) >= 2 {
				state := parts[0]
				cmdline := strings.Join(parts[1:], " ")

				// Kill if stuck
				if state == "D" && strings.Contains(cmdline, "dd") && strings.Contains(cmdline, flagImageFile) {
					cmd.Process.Signal(syscall.SIGKILL)
					cmd.Wait()

					// Quick retry
					retryCmd := exec.Command("dd", "if="+flagImageFile, "of=/dev/null", "bs=1M", "count=1")
					retryCmd.Run()
				}
			}
		}
		// -------------------------------
		// --- /End really dumb stuff ----
		// -------------------------------

		file, err := os.Open(flagImageFile)
		if err != nil {
			return fmt.Errorf("failed to open image file: %v", err)
		}
		defer file.Close()

		name := flagImageName
		if name == "" {
			name = fmt.Sprintf("%s-%s", filepath.Base(flagImageFile), time.Now().Format("20060102-150405"))
		}

		// Get file size for progress display
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file: %v", err)
		}

		fmt.Printf("Starting upload of %s (%d MB)\n", flagImageFile, info.Size()/1024/1024)

		req := api.CreateImageRequest{
			Name:         name,
			ContainerFmt: "bare",
			DiskFmt:      format,
			Visibility:   "shared",
		}

		imageID, err := api.CreateAndUploadImage(imageURL, tok.Value, req, file)
		if err != nil {
			return fmt.Errorf("failed to create/upload image: %v", err)
		}

		fmt.Printf("Image created: ID: %s, Name: %s\n", imageID, name)
		return nil
	},
}

// Subcommand: create volume
var createVolumeCmd = &cobra.Command{
	Use:     "volume",
	Aliases: []string{"vol"},
	Short:   "Create a new storage volume",
	RunE: func(cmd *cobra.Command, args []string) error {
		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}

		if flagVolumeImage != "" {
			imageURL, err := validateTokenEndpoint(tok, "image")
			if err != nil {
				return err
			}

			imageID, err := api.GetImageIDByName(imageURL, tok.Value, flagVolumeImage)
			if err != nil {
				return fmt.Errorf("failed to get image ID: %v", err)
			}

			imageSize, err := api.GetImageSize(imageURL, tok.Value, imageID)
			if err != nil {
				return fmt.Errorf("failed to get image size: %v", err)
			}
			// Round up to nearest GB
			volumeSize := int((imageSize + 1024*1024*1024 - 1) / (1024 * 1024 * 1024))

			resp, err := api.CreateVolumeFromImage(storageURL, tok.Value, imageID, flagVolumeName, int64(volumeSize))
			if err != nil {
				return err
			}
			fmt.Printf("Volume created from image: ID: %s, Name: %s, Size: %d GB\n", resp.Volume.ID, resp.Volume.Name, resp.Volume.Size)
			return nil
		}

		var request api.CreateVolumeRequest
		request.Volume.Name = flagVolumeName
		request.Volume.Size = flagVolumeSize
		request.Volume.Description = flagVolumeDescription
		request.Volume.VolumeType = flagVolumeType

		resp, err := api.CreateVolume(storageURL, tok.Value, request)
		if err != nil {
			return err
		}

		fmt.Printf("Volume created: ID: %s, Name: %s, Size: %d GB\n", resp.Volume.ID, resp.Volume.Name, resp.Volume.Size)
		return nil
	},
}

var createPortCmd = &cobra.Command{
	Use:     "port",
	Aliases: []string{"nic", "interface"},
	Short:   "Create a network port",
	RunE: func(cmd *cobra.Command, args []string) error {
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		// Check required network flag
		networkID := flagPortNetwork
		if networkID == "" {
			return fmt.Errorf("network is required: specify with --network flag")
		}

		if err := validateMAC(flagPortMAC); err != nil {
			return err
		}

		// Check if network exists by name first
		fmt.Printf("Checking network ID for %s\n", networkID)
		netID, err := api.GetNetworkIDByName(networkURL, tok.Value, networkID)
		if err == nil {
			fmt.Printf("Network found: %s\n", netID)
			networkID = netID
		} else {
			fmt.Printf("Network ID not found by name, using as-is: %s\n", err)
		}

		// Create port
		resp, err := api.CreatePort(networkURL, tok.Value, networkID, flagPortMAC)
		if err != nil {
			return fmt.Errorf("failed to create port: %v", err)
		}

		if flagJsonOutput {
			b, _ := json.MarshalIndent(resp, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Printf("Port created successfully:\n")
			fmt.Printf("  ID: %s\n", resp.Port.ID)
			fmt.Printf("  MAC: %s\n", resp.Port.MACAddress)
			fmt.Printf("  Network: %s\n", resp.Port.NetworkID)
			fmt.Printf("  Status: %s\n", resp.Port.Status)
		}

		return nil
	},
}

var (
	flagVolumeName        string
	flagVolumeSize        int
	flagVolumeDescription string
	flagVolumeType        string
	flagVolumeImage       string
	flagImageFile         string
	flagImageName         string
	flagDiskFormat        string
	flagPortNetwork       string
	flagPortMAC           string
	flagInstanceID        string
	flagDeleteSnapshot    bool
)

func init() {
	// Flags for create vm
	createVMCmd.Flags().StringVar(&flagVMName, "name", "", "Name of the virtual machine")
	createVMCmd.Flags().StringVar(&flagFlavorRef, "flavor", "", "Flavor ID for the virtual machine")
	createVMCmd.Flags().StringVar(&flagImageRef, "image", "", "Image ID for the virtual machine")
	createVMCmd.Flags().StringVar(&flagNetworkCSV, "networks", "", "Comma-separated list of network UUIDs")
	createVMCmd.Flags().StringVar(&flagIPCSV, "ips", "", "Comma-separated list of IP addresses ('none' for unmanaged network)")
	createVMCmd.Flags().BoolVar(&flagJsonOutput, "json", false, "Output in JSON format (default: YAML)")
	createVMCmd.Flags().IntVar(&flagVMSize, "size", 0, "Size in GB of boot volume")
	createVMCmd.Flags().BoolVar(&flagVMNetboot, "netboot", false, "Enable network boot with blank volume (deprecated, use --image)")
	createVMCmd.Flags().StringVar(&flagUserData, "user-data", "", "User script, bash, YAML (file path), use with --ci-data for templating, eg. {{%variable%}}")
	createVMCmd.Flags().StringVar(&flagMacAddrCSV, "macaddr", "", "Comma-separated list of MAC addresses ('auto' is valid value)")
	createVMCmd.Flags().StringVar(&flagCIData, "ci-data", "", "Template variables for cloud-init in format key:value,key:value")

	// Bind flags to viper
	viper.BindPFlag("flavor_id", createVMCmd.Flags().Lookup("flavor"))
	viper.BindPFlag("image_id", createVMCmd.Flags().Lookup("image"))
	viper.BindPFlag("networks", createVMCmd.Flags().Lookup("networks"))

	createVMCmd.MarkFlagRequired("name")

	// Flags for create volume
	createVolumeCmd.Flags().StringVar(&flagVolumeName, "name", "", "Name of the volume")
	createVolumeCmd.Flags().IntVar(&flagVolumeSize, "size", 0, "Size of the volume in GB (not needed when creating from image)")
	createVolumeCmd.Flags().StringVar(&flagVolumeDescription, "description", "", "Description of the volume")
	createVolumeCmd.Flags().StringVar(&flagVolumeType, "type", "nvme_ec7_2", "Type of the volume: nvme_ec7_2, replica3")
	createVolumeCmd.Flags().StringVar(&flagVolumeImage, "image", "", "Image ID to create volume from")

	createVolumeCmd.MarkFlagRequired("name")
	// Only require size if not creating from image
	if flagVolumeImage == "" {
		createVolumeCmd.MarkFlagRequired("size")
	}

	// Flags for create image
	createImageCmd.Flags().StringVar(&flagImageFile, "file", "", "Path to the image file")
	createImageCmd.Flags().StringVar(&flagImageName, "name", "", "Name of the image")
	createImageCmd.Flags().StringVar(&flagDiskFormat, "format", "", "Disk format (qcow2, raw, vmdk, iso)")
	createImageCmd.Flags().StringVar(&flagInstanceID, "instance", "", "VM instance ID or name to snapshot")
	createImageCmd.Flags().BoolVar(&flagDeleteSnapshot, "delete-snapshot", true, "Delete snapshot after creating template")

	// Flags for create port
	createPortCmd.Flags().StringVar(&flagPortNetwork, "network", "", "Network ID or name")
	createPortCmd.Flags().StringVar(&flagPortMAC, "mac", "", "MAC address")

	// Add subcommands to the parent create command
	createCmd.AddCommand(createVMCmd)
	createCmd.AddCommand(createVolumeCmd)
	createCmd.AddCommand(createImageCmd)
	createCmd.AddCommand(createPortCmd)

	// Add the create command to the root command
	rootCmd.AddCommand(createCmd)
}
