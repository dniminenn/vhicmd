package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// =========================
// USAGE EXPLANATIONS
// =========================
// On a host with access to the VHI API as well as the VMDK stores mounted,
// the 'migrate vm' command can be used to migrate a VM from a VMDK image.
// The VMDK image is uploaded to the VHI API as a temporary image, then a VM
// is created with the image as the root volume. The VM is then attached to
// the specified networks with the specified MAC addresses.
//
// TO PREVENT COLLISIONS:
// Ensure the vSphere VM is powered off before migration, or use the --shutdown
// flag to shut down the VM after migration.

// 'migrate' parent command
var migrateCmd = &cobra.Command{
	Use:     "migrate",
	Aliases: []string{"mig"},
	Short:   "Migrate resources from VMWare to VHI",
}

// 'migrate vm' subcommand
var migrateVMCmd = &cobra.Command{
	Use:   "vm",
	Short: "Migrate a virtual machine from a VMWare VMDK",
	Long: `Example:
  vhicmd migrate vm \
    --name MyVM \
    --vmdk /path/to/disk.vmdk \
    --flavor myflavor \
    --networks netA,netB,netC \
    --mac auto,bb:bb:bb:bb:bb:bb,auto \
    --size 20 \
    --shutdown`,
	RunE: func(cmd *cobra.Command, args []string) error {
		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}
		imageURL, err := validateTokenEndpoint(tok, "image")
		if err != nil {
			return err
		}
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}

		if migrateFlagVMName == "" {
			return fmt.Errorf("must provide --name for the VM")
		}
		if migrateFlagVMDKPath == "" {
			return fmt.Errorf("must provide --vmdk /path/to/image for migration")
		}

		// validate disk bus
		if migrateFlagDiskBus != "sata" && migrateFlagDiskBus != "scsi" && migrateFlagDiskBus != "virtio" {
			return fmt.Errorf("disk bus must be one of: sata, scsi, virtio")
		}

		flavorRef := migrateFlagFlavorRef
		if flavorRef == "" {
			flavorRef = viper.GetString("flavor_id")
		}
		if flavorRef == "" {
			return fmt.Errorf("no flavor specified; provide --flavor or set 'flavor_id' in config")
		}

		networks := migrateFlagNetworkCSV
		if networks == "" {
			networks = viper.GetString("networks")
		}
		if networks == "" {
			return fmt.Errorf("no networks specified; provide --networks or set 'networks' in config")
		}

		macs := migrateFlagMacAddrCSV
		networkIDs := strings.Split(networks, ",")
		macAddresses := strings.Split(macs, ",")
		if len(networkIDs) != len(macAddresses) {
			return fmt.Errorf("the number of networks must match the number of MAC addresses")
		}
		for _, mac := range macAddresses {
			if err := validateMacAddr(mac); err != nil {
				return fmt.Errorf("invalid MAC address: %s", err)
			}
		}

		fid, err := api.GetFlavorIDByName(computeURL, tok.Value, flavorRef)
		if err == nil && fid != "" {
			flavorRef = fid
		}

		// --- BEGIN SKETCHY STUFF ---
		// Wake up NFS
		// ---------------------------
		if strings.HasPrefix(migrateFlagVMDKPath, "/mnt/vmdk/") {
			cmd := exec.Command("dd", "if="+migrateFlagVMDKPath, "of=/dev/null", "bs=1M", "count=1", "status=progress")
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
				if state == "D" && strings.Contains(cmdline, "dd") && strings.Contains(cmdline, migrateFlagVMDKPath) {
					cmd.Process.Signal(syscall.SIGKILL)
					cmd.Wait()

					// Quick retry
					retryCmd := exec.Command("dd", "if="+migrateFlagVMDKPath, "of=/dev/null", "bs=1M", "count=1")
					retryCmd.Run()
				}
			}
		}
		// --- END SKETCHY STUFF ---
		// ---------------------------

		fmt.Printf("Creating temporary image for VM '%s'...\n", migrateFlagVMName)

		file, err := os.Open(migrateFlagVMDKPath)
		if err != nil {
			return fmt.Errorf("failed to open image file: %v", err)
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file: %v", err)
		}

		fmt.Printf("Starting upload of %s (%d MB)\n", migrateFlagVMDKPath, info.Size()/1024/1024)

		imgReq := api.CreateImageRequest{
			Name:         fmt.Sprintf("Migrated-%s", migrateFlagVMName),
			ContainerFmt: "bare",
			DiskFmt:      "vmdk",
			Visibility:   "shared",
		}

		imageID, err := api.CreateAndUploadImage(imageURL, tok.Value, imgReq, file)
		if err != nil {
			return fmt.Errorf("failed to create/upload image: %v", err)
		}

		var imageSize int64
		for i := 0; i < 3; i++ {
			imageSize, err = api.GetImageSize(imageURL, tok.Value, imageID)
			if err == nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
		if err != nil {
			return fmt.Errorf("failed to get image size after retries: %v", err)
		}

		imageSizeGB := int64(0)
		if migrateFlagVMSize == 0 {
			// round up to the nearest GB
			imageSizeGB = (imageSize + 1024*1024*1024 - 1) / (1024 * 1024 * 1024)
		} else {
			imageSizeGB = migrateFlagVMSize
		}

		fmt.Printf("\nImage created: %s\n", imageID)

		if migrateFlagI440fx {
			fmt.Printf("Setting i440fx machine type for image %s...\n", imageID)
			if err := api.SetImageI440fx(imageURL, tok.Value, imageID); err != nil {
				return fmt.Errorf("failed to set i440fx machine type: %v", err)
			}
		}

		// Handle secondary disk if specified
		var secondaryVolumeID string
		if migrateFlagSecondaryVMDK != "" {
			storageURL, err := validateTokenEndpoint(tok, "volumev3")
			if err != nil {
				return err
			}

			fmt.Printf("Creating temporary image for secondary disk...\n")

			file, err := os.Open(migrateFlagSecondaryVMDK)
			if err != nil {
				return fmt.Errorf("failed to open secondary VMDK file: %v", err)
			}
			defer file.Close()

			info, err := file.Stat()
			if err != nil {
				return fmt.Errorf("failed to stat secondary VMDK file: %v", err)
			}

			fmt.Printf("Starting upload of secondary disk %s (%d MB)\n", migrateFlagSecondaryVMDK, info.Size()/1024/1024)

			imgReq := api.CreateImageRequest{
				Name:         fmt.Sprintf("Secondary-%s", migrateFlagVMName),
				ContainerFmt: "bare",
				DiskFmt:      "vmdk",
				Visibility:   "shared",
			}

			secondaryImageID, err := api.CreateAndUploadImage(imageURL, tok.Value, imgReq, file)
			if err != nil {
				return fmt.Errorf("failed to create/upload secondary image: %v", err)
			}

			fmt.Printf("Creating volume from secondary image...\n")
			volumeResp, err := api.CreateVolumeFromImage(storageURL, tok.Value, secondaryImageID, fmt.Sprintf("secondary-%s", migrateFlagVMName))
			if err != nil {
				return fmt.Errorf("failed to create volume from secondary image: %v", err)
			}

			secondaryVolumeID = volumeResp.Volume.ID

			fmt.Printf("Waiting for secondary volume to become available...\n")
			err = api.WaitForVolumeStatus(storageURL, tok.Value, secondaryVolumeID, "available")
			if err != nil {
				return fmt.Errorf("failed waiting for secondary volume to become available: %v", err)
			}

			fmt.Printf("Deleting temporary secondary image %s...\n", secondaryImageID)
			for i := 0; i < 3; i++ {
				err = api.DeleteImage(imageURL, tok.Value, secondaryImageID)
				if err == nil {
					break
				}
				time.Sleep(5 * time.Second)
			}
			if err != nil {
				return fmt.Errorf("failed to delete temporary secondary image after retries: %v", err)
			}
		}

		vmReq := api.CreateVMRequest{}
		vmReq.Server.Name = migrateFlagVMName
		vmReq.Server.FlavorRef = flavorRef
		vmReq.Server.ImageRef = imageID
		vmReq.Server.Networks = "none"

		// Force SATA block device
		// NOTE: This is a bit of a hack to force the use of SATA for the root volume
		// so udev in the VM uses /dev/sda instead of /dev/vda, as with VMWare.
		mapping := map[string]interface{}{
			"boot_index":            0,
			"uuid":                  imageID,
			"source_type":           "image",
			"destination_type":      "volume",
			"volume_size":           imageSizeGB,
			"delete_on_termination": true,
			"disk_bus":              migrateFlagDiskBus,
			"volume_type":           "nvme_ec7_2",
		}
		vmReq.Server.BlockDeviceMappingV2 = []map[string]interface{}{mapping}

		fmt.Printf("Creating VM '%s'...\n", migrateFlagVMName)
		vmResp, err := api.CreateVM(computeURL, tok.Value, vmReq)
		if err != nil {
			return fmt.Errorf("failed to create VM: %v", err)
		}

		// Wait for ACTIVE
		vmDetails, err := api.WaitForStatus(computeURL, tok.Value, vmResp.Server.ID, "ACTIVE")
		if err != nil {
			return fmt.Errorf("failed waiting for VM to become ACTIVE: %v", err)
		}

		// Attach secondary volume if we have one
		if secondaryVolumeID != "" {
			fmt.Printf("Attaching secondary volume to VM...\n")
			err = api.AttachVolume(computeURL, tok.Value, vmDetails.ID, secondaryVolumeID)
			if err != nil {
				return fmt.Errorf("failed to attach secondary volume: %v", err)
			}
		}

		netInfo := make([]map[string]interface{}, 0)
		for i, netNameOrID := range networkIDs {
			netNameOrID = strings.TrimSpace(netNameOrID)
			macAddr := strings.TrimSpace(macAddresses[i])

			// If the user specified "auto", then omit the mac_addr field by setting it to empty.
			if strings.ToLower(macAddr) == "auto" {
				macAddr = ""
			}

			// Try to resolve network name->ID
			netID, err := api.GetNetworkIDByName(networkURL, tok.Value, netNameOrID)
			if err == nil && netID != "" {
				netNameOrID = netID
			}

			fmt.Printf("Attaching network '%s' to VM '%s' with MAC '%s'...\n",
				netNameOrID, vmDetails.ID, macAddr)

			// Create a port, using the MAC address for unmanaged networks
			portResp, err := api.CreatePort(networkURL, tok.Value, netNameOrID, macAddr)
			if err != nil {
				return fmt.Errorf("failed to create port on network %s: %v", netNameOrID, err)
			}

			// Attach the port to the VM (unchanged)
			_, err = api.AttachNetworkToVM(networkURL, computeURL, tok.Value, vmDetails.ID, "", portResp.Port.ID, nil)
			if err != nil {
				return fmt.Errorf("failed to attach port '%s' to VM '%s': %v", portResp.Port.ID, vmDetails.ID, err)
			}

			// Optionally, add the network info to your summary.
			netInfo = append(netInfo, map[string]interface{}{
				"network_id":  netNameOrID,
				"mac_address": portResp.Port.MACAddress,
			})
		}

		api.RebootVM(computeURL, tok.Value, vmDetails.ID, "HARD")

		// -- Not very reliable if the VM is hung since
		// this only sends a soft os-stop signal, it takes
		// ~5 minutes if acpid is not running in the VM.
		if migrateFlagShutdown {
			fmt.Printf("Shutting down VM '%s'...\n", vmDetails.ID)
			if err := api.StopVM(computeURL, tok.Value, vmDetails.ID); err != nil {
				return fmt.Errorf("failed to shut down VM: %v", err)
			}
		}

		fmt.Printf("Deleting temporary image %s...\n", imageID)
		for i := 0; i < 3; i++ {
			err = api.DeleteImage(imageURL, tok.Value, imageID)
			if err == nil {
				break
			}
			time.Sleep(5 * time.Second)
		}
		if err != nil {
			return fmt.Errorf("failed to delete temporary image after retries: %v", err)
		}

		summary := map[string]interface{}{
			"vm_id":   vmDetails.ID,
			"vm_name": vmDetails.Name,
			"power_state": fmt.Sprintf("%d (%s)",
				vmDetails.PowerState,
				getPowerStateString(vmDetails.PowerState)),
			"networks": netInfo,
		}

		data, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(data))

		return nil
	},
}

// migrateFindCmd is the 'migrate find' subcommand
var migrateFindCmd = &cobra.Command{
	Use:   "find <pattern>",
	Short: "Find a VMDK file matching the pattern in /mnt/vmdk",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		single := false
		if migrateFindVMDKSingle {
			single = true
		}

		pattern := args[0]

		fmt.Printf("Searching for VMDK files matching '%s' in /mnt/vmdk...\n", pattern)
		start := time.Now()

		var matches []string
		var err error

		if single {
			match, verr := findSingleVMDK(pattern)
			err = verr
			matches = []string{match}
		} else {
			matches, err = findVMDKs(pattern)
		}
		duration := time.Since(start)

		fmt.Printf("\nSearch completed in %s\n", duration)

		if err != nil {
			return err
		}

		if len(matches) == 0 {
			fmt.Println("No matching VMDK files found.")
			return nil
		}

		fmt.Println("\nMatching VMDK files:")
		for _, match := range matches {
			fmt.Println(match)
		}

		return nil
	},
}

// Flags for migrate vm
var (
	migrateFlagVMName        string
	migrateFlagVMDKPath      string
	migrateFlagFlavorRef     string
	migrateFlagNetworkCSV    string
	migrateFlagMacAddrCSV    string
	migrateFlagVMSize        int64
	migrateFlagDiskBus       string
	migrateFlagShutdown      bool
	migrateFindVMDKSingle    bool
	migrateFlagI440fx        bool
	migrateFlagSecondaryVMDK string
)

func init() {
	migrateVMCmd.Flags().StringVar(&migrateFlagVMName, "name", "", "Name of the VM")
	migrateVMCmd.Flags().StringVar(&migrateFlagVMDKPath, "vmdk", "", "Local path to VMDK file")
	migrateVMCmd.Flags().StringVar(&migrateFlagFlavorRef, "flavor", "", "Flavor name or ID")
	migrateVMCmd.Flags().StringVar(&migrateFlagNetworkCSV, "networks", "", "Comma-separated network names/IDs")
	migrateVMCmd.Flags().StringVar(&migrateFlagMacAddrCSV, "mac", "", "Comma-separated MAC addresses (one per network)")
	migrateVMCmd.Flags().Int64Var(&migrateFlagVMSize, "size", 0, "Optional: size in GB if extending the image")
	migrateVMCmd.Flags().StringVar(&migrateFlagDiskBus, "disk-bus", "scsi", "Disk bus for the root volume, default: scsi")
	migrateVMCmd.Flags().BoolVar(&migrateFlagShutdown, "shutdown", false, "Shut down the new VM after creation")
	migrateVMCmd.Flags().BoolVar(&migrateFlagI440fx, "i440fx", false, "Set i440fx machine type for the image (centos 6)")
	migrateVMCmd.Flags().StringVar(&migrateFlagSecondaryVMDK, "secondary-vmdk", "", "Local path to secondary VMDK file to attach as additional volume")
	migrateFindCmd.Flags().BoolVar(&migrateFindVMDKSingle, "single", false, "Find a single VMDK file")

	migrateCmd.AddCommand(migrateVMCmd)
	migrateCmd.AddCommand(migrateFindCmd)

	rootCmd.AddCommand(migrateCmd)
}
