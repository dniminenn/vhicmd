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
		return runCreateVM(createVMOptions{
			Name:       flagVMName,
			Flavor:     flagFlavorRef,
			Image:      flagImageRef,
			Networks:   flagNetworkCSV,
			IPs:        flagIPCSV,
			MACs:       flagMacAddrCSV,
			Ports:      flagPortCSV,
			Size:       flagVMSize,
			Volume:     flagVMVolume,
			VolumeType: flagVMVolumeType,
			Netboot:    flagVMNetboot,
			UserData:   flagUserData,
			CIData:     flagCIData,
			CIDataFile: flagCIDataFile,
		})
	},
}

// createVMOptions holds the create vm flags; rebuild fills them from an existing VM.
type createVMOptions struct {
	Name, Flavor, Image          string
	Networks, IPs, MACs, Ports   string
	Size                         int
	Volume, VolumeType           string
	Netboot                      bool
	UserData, CIData, CIDataFile string
}

// defaultVolumeType is the type of a new boot volume when neither --volume-type nor the
// volume_type setting names one.
const defaultVolumeType = "nvme_ec7_2"

// bootVolumeType picks the type of a new boot volume: the flag, the setting, the default.
func bootVolumeType(flag string) string {
	if flag != "" {
		return flag
	}
	if t := viper.GetString("volume_type"); t != "" {
		return t
	}
	return defaultVolumeType
}

// checkBootVolumeOptions rejects the flags that make no sense with --volume.
func checkBootVolumeOptions(o createVMOptions) error {
	if o.Volume == "" {
		return nil
	}
	switch {
	case o.Image != "":
		return fmt.Errorf("--volume and --image are mutually exclusive: the VM boots from the volume as it is")
	case o.Netboot:
		return fmt.Errorf("--volume and --netboot are mutually exclusive")
	case o.Size > 0:
		return fmt.Errorf("--size does not apply to --volume: the volume keeps its size")
	case o.VolumeType != "":
		return fmt.Errorf("--volume-type does not apply to --volume: the volume keeps its type")
	}
	return nil
}

// checkBootVolume says why a volume cannot be the boot disk of a new VM, if it cannot.
func checkBootVolume(v api.VolumeDetail) error {
	if v.Status != "available" {
		return fmt.Errorf("volume %s is %s; it must be available (detached) to boot a VM from it", v.ID, v.Status)
	}
	if v.Bootable != "true" {
		return fmt.Errorf("volume %s is not bootable; run: vhicmd bootable %s true", v.ID, v.ID)
	}
	return nil
}

// buildUserData returns the base64 user data of the VM, templated when ci-data is given.
func buildUserData(o createVMOptions) (string, error) {
	if o.CIData == "" && o.CIDataFile == "" {
		return readAndEncodeUserData(o.UserData)
	}

	var ciDataStr string
	if o.CIDataFile != "" {
		fileBytes, err := os.ReadFile(o.CIDataFile)
		if err != nil {
			return "", fmt.Errorf("error reading ci-data-file: %v", err)
		}
		ciDataStr = string(fileBytes)
	} else {
		ciDataStr = o.CIData
	}
	ciData, err := template.ParseKeyValueString(ciDataStr)
	if err != nil {
		return "", fmt.Errorf("error parsing ci-data: %v", err)
	}

	rawUserData, err := readUserDataFile(o.UserData)
	if err != nil {
		return "", err
	}

	validation := template.ValidateTemplate(rawUserData, ciData)
	if !validation.Valid {
		return "", fmt.Errorf("template validation failed: missing vars %v", validation.MissingVariables)
	}
	if len(validation.UnusedVariables) > 0 {
		return "", fmt.Errorf("template validation failed: unused vars %v", validation.UnusedVariables)
	}

	return encodeUserData(template.ReplaceVariables(rawUserData, ciData))
}

func runCreateVM(o createVMOptions) error {
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

	if err := checkBootVolumeOptions(o); err != nil {
		return err
	}

	//----------------------------------------------------------------
	// 2. Gather image and flavor references
	//----------------------------------------------------------------
	imageRef := o.Image
	if imageRef == "" && o.Volume == "" {
		imageRef = viper.GetString("image_id")
	}
	flavorRef := o.Flavor
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
	networks := o.Networks
	if networks == "" {
		networks = viper.GetString("networks")
	}
	// --networks none: a VM with no network interface at all.
	noNetwork := o.Ports == "" && strings.EqualFold(strings.TrimSpace(networks), "none")

	if o.Ports != "" {
		// --ports path: use pre-created port IDs directly
		if o.Networks != "" || o.IPs != "" || o.MACs != "" {
			return fmt.Errorf("--ports cannot be combined with --networks, --ips, or --macaddr")
		}

		portIDs := strings.Split(o.Ports, ",")
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
	} else if noNetwork {
		if o.IPs != "" || o.MACs != "" {
			return fmt.Errorf("--networks none cannot be combined with --ips or --macaddr")
		}
	} else {
		// --networks path: original flow
		if networks == "" {
			return fmt.Errorf("no networks specified; use --networks, --ports, or set 'networks' in config")
		}

		ips := o.IPs
		macs := o.MACs

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
			nid, err := api.GetNetworkIDByName(networkURL, tok.Value, strings.TrimSpace(netName))
			if err != nil {
				return fmt.Errorf("network %q: %v", strings.TrimSpace(netName), err)
			}
			networkIDs[i] = nid
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
	if imageRef != "" {
		if imgID, err := api.GetImageIDByName(imageURL, tok.Value, imageRef); err == nil && imgID != "" {
			imageRef = imgID
		}
	}
	if fid, err := api.GetFlavorIDByName(computeURL, tok.Value, flavorRef); err == nil && fid != "" {
		flavorRef = fid
	}

	//----------------------------------------------------------------
	// 5. Volume size & netboot checks
	//----------------------------------------------------------------
	volumeSize := 10 // default
	if o.Size > 0 {
		volumeSize = o.Size
	}
	if o.CIData != "" && o.CIDataFile != "" {
		return fmt.Errorf("--ci-data and --ci-data-file are mutually exclusive")
	}
	if (o.CIData != "" || o.CIDataFile != "") && o.UserData == "" {
		return fmt.Errorf("--ci-data/--ci-data-file requires --user-data")
	}

	//----------------------------------------------------------------
	// 6. Create the base VM request
	//----------------------------------------------------------------
	var request api.CreateVMRequest
	request.Server.Name = o.Name
	request.Server.FlavorRef = flavorRef

	// netboot => skip image
	if o.Netboot {
		imageRef = ""
		request.Server.Metadata = map[string]string{
			"network_install": "true",
		}
	}

	// Assign the networks JSON string
	request.Server.Networks = string(netBytes)
	if noNetwork {
		request.Server.Networks = "none"
	}

	//----------------------------------------------------------------
	// 12. Cloud-init / user data (templating if needed)
	//----------------------------------------------------------------
	if o.UserData != "" {
		userData, err := buildUserData(o)
		if err != nil {
			return err
		}
		request.Server.UserData = userData
		request.Server.ConfigDrive = true
	}

	//----------------------------------------------------------------
	// 13. Block device mapping
	//----------------------------------------------------------------
	switch {
	case o.Volume != "":
		// Boot from an existing volume, which outlives the VM.
		volumeID, err := api.GetVolumeIDByName(storageURL, tok.Value, o.Volume)
		if err != nil {
			return fmt.Errorf("volume %q: %v", o.Volume, err)
		}
		volume, err := api.GetVolumeDetails(storageURL, tok.Value, volumeID)
		if err != nil {
			return err
		}
		if err := checkBootVolume(volume); err != nil {
			return err
		}
		request.Server.BlockDeviceMappingV2 = []map[string]interface{}{
			{
				"boot_index":            "0",
				"uuid":                  volumeID,
				"source_type":           "volume",
				"destination_type":      "volume",
				"delete_on_termination": false,
			},
		}

	case imageRef != "":
		// Use the image => create volume from image
		request.Server.BlockDeviceMappingV2 = []map[string]interface{}{
			{
				"boot_index":            "0",
				"uuid":                  imageRef,
				"source_type":           "image",
				"destination_type":      "volume",
				"volume_size":           volumeSize,
				"delete_on_termination": true,
				"volume_type":           bootVolumeType(o.VolumeType),
				"disk_bus":              "scsi",
			},
		}

	default:
		// Netboot or no image => create blank volume
		fmt.Fprintf(os.Stderr, "Creating blank boot volume for VM %s...\n", o.Name)
		volRequest := api.CreateVolumeRequest{}
		volRequest.Volume.Name = fmt.Sprintf("%s-boot", o.Name)
		volRequest.Volume.Size = volumeSize
		volRequest.Volume.Description = "Boot volume for " + o.Name
		volRequest.Volume.VolumeType = bootVolumeType(o.VolumeType)

		volResp, err := api.CreateVolume(storageURL, tok.Value, volRequest)
		if err != nil {
			return fmt.Errorf("failed to create blank boot volume: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Waiting for volume to become available...\n")

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
	fmt.Fprintf(os.Stderr, "Creating VM %s...\n", o.Name)

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
	if o.Netboot {
		consoleURL := fmt.Sprintf("%s:8800/compute/servers/instances/%s/console", tok.Host, vmDetails.ID)
		fmt.Fprintf(os.Stderr, "\nGo to VHI console to complete machine boot/install.\n")
		fmt.Fprintf(os.Stderr, "VHI console: %s\n", consoleURL)
	}

	return nil
}

var (
	flagVMName       string
	flagFlavorRef    string
	flagImageRef     string
	flagNetworkCSV   string
	flagIPCSV        string
	flagVMSize       int
	flagVMNetboot    bool
	flagUserData     string
	flagMacAddrCSV   string
	flagCIData       string
	flagCIDataFile   string
	flagPortCSV      string
	flagVMVolume     string
	flagVMVolumeType string
)
