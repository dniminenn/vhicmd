package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jessegalley/vhicmd/api"
	"github.com/jessegalley/vhicmd/internal/template"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

// Subcommand: create vm
var createVMCmd = &cobra.Command{
	Use:   "vm",
	Short: "Create a new virtual machine",
	RunE: func(cmd *cobra.Command, args []string) error {
		//----------------------------------------------------------------
		// 1. Validate token endpoints
		//----------------------------------------------------------------
		computeURL, err := validateTokenEndpoint(tok, "compute")
		if err != nil {
			return err
		}
		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}
		imageURL, err := validateTokenEndpoint(tok, "image")
		if err != nil {
			return err
		}

		//----------------------------------------------------------------
		// 2. Gather image and flavor references
		//----------------------------------------------------------------
		imageRef := flagImageRef
		if imageRef == "" {
			imageRef = viper.GetString("image_id")
		}
		flavorRef := flagFlavorRef
		if flavorRef == "" {
			flavorRef = viper.GetString("flavor_id")
		}
		if flavorRef == "" {
			return fmt.Errorf("no flavor specified; provide --flavor or set 'flavor_id' in config")
		}

		//----------------------------------------------------------------
		// 3. Build the networks array (--ports path or --networks path)
		//----------------------------------------------------------------
		var netBytes []byte

		if flagPortCSV != "" {
			// --ports path: use pre-created port IDs directly
			if flagNetworkCSV != "" || flagIPCSV != "" || flagMacAddrCSV != "" {
				return fmt.Errorf("--ports cannot be combined with --networks, --ips, or --macaddr")
			}

			portIDs := strings.Split(flagPortCSV, ",")
			var netSlice []map[string]interface{}
			for _, pid := range portIDs {
				netSlice = append(netSlice, map[string]interface{}{
					"port": strings.TrimSpace(pid),
				})
			}

			var err error
			netBytes, err = json.Marshal(netSlice)
			if err != nil {
				return fmt.Errorf("failed to marshal networks: %v", err)
			}
		} else {
			// --networks path: original flow
			networks := flagNetworkCSV
			if networks == "" {
				networks = viper.GetString("networks")
			}
			if networks == "" {
				return fmt.Errorf("no networks specified; use --networks, --ports, or set 'networks' in config")
			}

			ips := flagIPCSV
			macs := flagMacAddrCSV

			if ips == "" && macs == "" {
				return fmt.Errorf("must specify either --ips or --macs (use 'none' or 'auto')")
			}

			networkIDs := strings.Split(networks, ",")
			var ipAddresses []string
			if ips != "" {
				ipAddresses = strings.Split(ips, ",")
			} else {
				ipAddresses = make([]string, len(networkIDs))
				for i := range ipAddresses {
					ipAddresses[i] = "none"
				}
			}

			var macAddresses []string
			if macs != "" {
				macAddresses = strings.Split(macs, ",")
			} else {
				macAddresses = make([]string, len(networkIDs))
				for i := range macAddresses {
					macAddresses[i] = "none"
				}
			}

			if len(networkIDs) != len(ipAddresses) || len(networkIDs) != len(macAddresses) {
				return fmt.Errorf(
					"number of networks (%d) must match number of IPs (%d) and MACs (%d)",
					len(networkIDs), len(ipAddresses), len(macAddresses),
				)
			}

			if err := validateIPs(ipAddresses); err != nil {
				return err
			}
			for _, m := range macAddresses {
				if err := validateMAC(m); err != nil {
					return err
				}
			}

			for i, netName := range networkIDs {
				nid, err := api.GetNetworkIDByName(networkURL, tok.Value, netName)
				if err == nil {
					networkIDs[i] = nid
				}
			}

			var netSlice []map[string]interface{}
			for i, netID := range networkIDs {
				ipVal := strings.TrimSpace(ipAddresses[i])
				macVal := strings.TrimSpace(macAddresses[i])

				netObj := map[string]interface{}{
					"uuid": netID,
				}

				if strings.ToLower(ipVal) != "none" {
					if strings.ToLower(ipVal) == "auto" {
						// skip => DHCP
					} else {
						netObj["fixed_ip"] = ipVal
					}
					if strings.ToLower(macVal) != "none" && strings.ToLower(macVal) != "auto" {
						return fmt.Errorf("managed NIC cannot have custom MAC: IP=%s MAC=%s", ipVal, macVal)
					}
				} else {
					if strings.ToLower(macVal) == "none" || strings.ToLower(macVal) == "auto" {
						// skip => hypervisor picks MAC
					} else {
						netObj["mac_address"] = macVal
					}
				}

				netSlice = append(netSlice, netObj)
			}

			var err error
			netBytes, err = json.Marshal(netSlice)
			if err != nil {
				return fmt.Errorf("failed to marshal networks: %v", err)
			}
		}

		//----------------------------------------------------------------
		// 4. Resolve image & flavor by name if necessary
		//----------------------------------------------------------------
		if imgID, err := api.GetImageIDByName(imageURL, tok.Value, imageRef); err == nil && imgID != "" {
			imageRef = imgID
		}
		if fid, err := api.GetFlavorIDByName(computeURL, tok.Value, flavorRef); err == nil && fid != "" {
			flavorRef = fid
		}

		//----------------------------------------------------------------
		// 5. Volume size & netboot checks
		//----------------------------------------------------------------
		volumeSize := 10 // default
		if flagVMSize > 0 {
			volumeSize = flagVMSize
		}
		if flagCIData != "" && flagCIDataFile != "" {
			return fmt.Errorf("--ci-data and --ci-data-file are mutually exclusive")
		}
		if (flagCIData != "" || flagCIDataFile != "") && flagUserData == "" {
			return fmt.Errorf("--ci-data/--ci-data-file requires --user-data")
		}

		//----------------------------------------------------------------
		// 6. Create the base VM request
		//----------------------------------------------------------------
		var request api.CreateVMRequest
		request.Server.Name = flagVMName
		request.Server.FlavorRef = flavorRef

		// netboot => skip image
		if flagVMNetboot {
			imageRef = ""
			request.Server.Metadata = map[string]string{
				"network_install": "true",
			}
		}

		// Assign the networks JSON string
		request.Server.Networks = string(netBytes)

		//----------------------------------------------------------------
		// 12. Block device mapping
		//----------------------------------------------------------------
		if imageRef != "" {
			// Use the image => create volume from image
			request.Server.BlockDeviceMappingV2 = []map[string]interface{}{
				{
					"boot_index":            "0",
					"uuid":                  imageRef,
					"source_type":           "image",
					"destination_type":      "volume",
					"volume_size":           volumeSize,
					"delete_on_termination": true,
					"volume_type":           "nvme_ec7_2",
					"disk_bus":              "scsi",
				},
			}

			//------------------------------------------------------------
			// 13. Cloud-init / user data (templating if needed)
			//------------------------------------------------------------
			if flagUserData != "" {
				var userData string
				if flagCIData != "" || flagCIDataFile != "" {
					// Templating path
					var ciDataStr string
					if flagCIDataFile != "" {
						fileBytes, err := os.ReadFile(flagCIDataFile)
						if err != nil {
							return fmt.Errorf("error reading ci-data-file: %v", err)
						}
						ciDataStr = string(fileBytes)
					} else {
						ciDataStr = flagCIData
					}
					ciData, err := template.ParseKeyValueString(ciDataStr)
					if err != nil {
						return fmt.Errorf("error parsing ci-data: %v", err)
					}

					rawUserData, err := readUserDataFile(flagUserData)
					if err != nil {
						return err
					}

					validation := template.ValidateTemplate(rawUserData, ciData)
					if !validation.Valid {
						return fmt.Errorf("template validation failed: missing vars %v", validation.MissingVariables)
					}
					if len(validation.UnusedVariables) > 0 {
						return fmt.Errorf("template validation failed: unused vars %v", validation.UnusedVariables)
					}

					processedUserData := template.ReplaceVariables(rawUserData, ciData)
					userData, err = encodeUserData(processedUserData)
					if err != nil {
						return err
					}

				} else {
					// Plain user-data, no templating
					userData, err = readAndEncodeUserData(flagUserData)
					if err != nil {
						return err
					}
				}

				request.Server.UserData = userData
				request.Server.ConfigDrive = true
			}

		} else {
			// Netboot or no image => create blank volume
			fmt.Printf("Creating blank boot volume for VM %s...\n", flagVMName)
			volRequest := api.CreateVolumeRequest{}
			volRequest.Volume.Name = fmt.Sprintf("%s-boot", flagVMName)
			volRequest.Volume.Size = volumeSize
			volRequest.Volume.Description = "Boot volume for " + flagVMName
			volRequest.Volume.VolumeType = "nvme_ec7_2"

			volResp, err := api.CreateVolume(storageURL, tok.Value, volRequest)
			if err != nil {
				return fmt.Errorf("failed to create blank boot volume: %v", err)
			}
			fmt.Printf("Waiting for volume to become available...\n")

			if err := api.WaitForVolumeStatus(storageURL, tok.Value, volResp.Volume.ID, "available"); err != nil {
				return fmt.Errorf("failed waiting for volume: %v", err)
			}

			if err := api.SetVolumeBootable(storageURL, tok.Value, volResp.Volume.ID, true); err != nil {
				return fmt.Errorf("failed to set bootable flag: %v", err)
			}

			request.Server.BlockDeviceMappingV2 = []map[string]interface{}{
				{
					"boot_index":            "0",
					"uuid":                  volResp.Volume.ID,
					"source_type":           "volume",
					"destination_type":      "volume",
					"delete_on_termination": true,
				},
			}
		}

		//----------------------------------------------------------------
		// 14. Create the VM in one shot (with our networks JSON)
		//----------------------------------------------------------------
		fmt.Printf("Creating VM %s...\n", flagVMName)

		// Create a complete JSON representation of the request
		requestBytes, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %v", err)
		}

		// Fix the networks field directly in the JSON to make it a JSON array instead of a string
		// Only do this if we aren't using the special "none" case
		if request.Server.Networks != "none" {
			networksStr := fmt.Sprintf(`"networks":"%s"`, strings.ReplaceAll(string(netBytes), `"`, `\"`))
			networksJSON := fmt.Sprintf(`"networks":%s`, string(netBytes))
			requestBytes = []byte(strings.Replace(string(requestBytes), networksStr, networksJSON, 1))
		}

		// Call the raw version that won't re-marshal the JSON
		resp, err := api.CreateVMRaw(computeURL, tok.Value, requestBytes)
		if err != nil {
			return fmt.Errorf("failed to create VM: %v", err)
		}

		//----------------------------------------------------------------
		// 15. Wait for VM to become ACTIVE
		//----------------------------------------------------------------
		vmDetails, err := api.WaitForStatus(computeURL, tok.Value, resp.Server.ID, "ACTIVE")
		if err != nil {
			return err
		}

		//----------------------------------------------------------------
		// 16. Prepare output details
		//----------------------------------------------------------------
		details := map[string]interface{}{
			"power_state": getPowerStateString(vmDetails.PowerState),
			"name":        vmDetails.Name,
			"id":          vmDetails.ID,
			"metadata":    vmDetails.Metadata,
		}

		// Build a list of network details from vmDetails.HCIInfo.Network
		netInfo := []map[string]interface{}{}

		for _, iface := range vmDetails.HCIInfo.Network {
			// 1) Use your existing fields: Mac, Network.ID, Network.Label
			netObj := map[string]interface{}{
				"mac":           iface.Mac,
				"network_id":    iface.Network.ID,
				"network_label": iface.Network.Label,
			}

			// 2) Re-marshal 'iface' into a generic map to see if "ips" is present
			rawIface, err := json.Marshal(iface)
			if err == nil {
				// Unmarshal into a generic map
				var generic map[string]interface{}
				if err := json.Unmarshal(rawIface, &generic); err == nil {
					// Attempt to read generic["ips"]
					if val, ok := generic["ips"]; ok {
						// Typically "ips" might be an array of strings
						if arr, ok := val.([]interface{}); ok && len(arr) > 0 {
							ipList := []string{}
							for _, ipVal := range arr {
								if ipStr, ok := ipVal.(string); ok {
									ipList = append(ipList, ipStr)
								}
							}
							// If we actually found some IP strings, store them
							if len(ipList) > 0 {
								netObj["ips"] = ipList
							}
						}
					}
				}
			}

			netInfo = append(netInfo, netObj)
		}

		// If we found NICs, add them to our final details map
		if len(netInfo) > 0 {
			details["network_details"] = netInfo
		}

		//----------------------------------------------------------------
		// 17. Output JSON or YAML
		//----------------------------------------------------------------
		if flagJsonOutput {
			jsonBytes, err := json.MarshalIndent(details, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal VM details to JSON: %v", err)
			}
			fmt.Println(string(jsonBytes))
		} else {
			yamlBytes, err := yaml.Marshal(details)
			if err != nil {
				return fmt.Errorf("failed to marshal VM details to YAML: %v", err)
			}
			fmt.Println(string(yamlBytes))
		}

		//----------------------------------------------------------------
		// 18. Netboot console message if needed
		//----------------------------------------------------------------
		if flagVMNetboot {
			consoleURL := fmt.Sprintf("%s:8800/compute/servers/instances/%s/console", tok.Host, vmDetails.ID)
			fmt.Printf("\nGo to VHI console to complete machine boot/install.\n")
			fmt.Printf("VHI console: %s\n", consoleURL)
		}

		return nil
	},
}

var (
	flagVMName     string
	flagFlavorRef  string
	flagImageRef   string
	flagNetworkCSV string
	flagIPCSV      string
	flagVMSize     int
	flagVMNetboot  bool
	flagUserData   string
	flagMacAddrCSV string
	flagCIData     string
	flagCIDataFile string
	flagPortCSV    string
)
