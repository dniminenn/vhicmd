package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/cobra"
)

var (
	rebuildImage    string
	rebuildYes      bool
	rebuildTimeout  time.Duration
	rebuildRecreate bool
	rebuildDryRun   bool
)

// Compute microversion that re-images a boot volume on rebuild.
const reimageMicroversion = "2.93"

// The default microversion of every other call (see httpclient.SendRequestWithToken).
const defaultMicroversion = "2.72"

// Below 2.93 a volume-backed VM is deleted and created again from the image with
// the same name, flavor, boot volume size, networks, MACs and fixed IPs.
var rebuildCmd = &cobra.Command{
	Use:   "rebuild <vm-name-or-id> --image <image-name-or-id>",
	Short: "Rebuild a VM from an image (its disk is replaced, its addresses stay)",
	Long: `Rebuild a VM from an image. Everything on its disk is lost.

In place when the compute API accepts microversion 2.93 or the VM boots from a
local disk: the VM keeps its ID, ports and addresses.

A volume-backed VM on an older compute API (or with --recreate) is deleted and
created again from the image with the same name, flavor, boot volume size,
networks, MACs (unmanaged networks) and fixed IPs (managed networks). The VM
gets a new ID and a new boot volume.`,
	Args: cobra.ExactArgs(1),
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
		vm, err := api.GetVMDetails(computeURL, tok.Value, vmID)
		if err != nil {
			return err
		}
		volumeBacked := vm.Flavor.Disk == 0 && len(vm.OSExtendedVolumesVolumesAttached) > 0

		microversion := defaultMicroversion
		recreate := rebuildRecreate
		if !recreate {
			maxVersion, err := api.ComputeMaxMicroversion(computeURL, tok.Value)
			switch {
			case err != nil && volumeBacked:
				return fmt.Errorf("%v; use --recreate to delete and create the VM again", err)
			case err != nil:
				fmt.Printf("%v; a local disk rebuilds at %s\n", err, defaultMicroversion)
			case microversionAtLeast(maxVersion, reimageMicroversion):
				fmt.Printf("Compute API maximum microversion: %s\n", maxVersion)
				microversion = reimageMicroversion
			default:
				fmt.Printf("Compute API maximum microversion: %s\n", maxVersion)
				if volumeBacked {
					recreate = true
					fmt.Printf("A volume-backed VM needs %s to rebuild in place; the VM will be recreated.\n", reimageMicroversion)
				}
			}
		}

		if !recreate {
			fmt.Printf("Plan: rebuild in place (microversion %s)\n", microversion)
			fmt.Printf("  vm:     %s (%s)\n", vm.Name, vmID)
			fmt.Printf("  image:  %s (%s)\n", rebuildImage, imageID)
			fmt.Printf("  ports and addresses stay\n")
			if rebuildDryRun {
				fmt.Println("Dry run: nothing changed.")
				return nil
			}
			if !rebuildYes {
				ok, err := readConfirmation(fmt.Sprintf("Rebuild %s (%s) from %s (%s)? Everything on its disk is lost. [y/N] ", vm.Name, vmID, rebuildImage, imageID))
				if err != nil || !ok {
					return fmt.Errorf("not confirmed")
				}
			}
			fmt.Printf("Rebuilding %s from %s...\n", vmID, imageID)
			if err := api.RebuildVM(computeURL, tok.Value, vmID, imageID, microversion, rebuildTimeout); err != nil {
				return err
			}
			fmt.Printf("VM %s is ACTIVE on image %s\n", vmID, imageID)
			return nil
		}

		storageURL, err := validateTokenEndpoint(tok, "volumev3")
		if err != nil {
			return err
		}
		networkURL, err := validateTokenEndpoint(tok, "network")
		if err != nil {
			return err
		}
		ifaces, err := api.GetVMNetworks(computeURL, tok.Value, vmID)
		if err != nil {
			return err
		}
		var vols []api.VolumeDetail
		for _, v := range vm.OSExtendedVolumesVolumesAttached {
			d, err := api.GetVolumeDetails(storageURL, tok.Value, v.ID)
			if err != nil {
				return err
			}
			vols = append(vols, d)
		}
		plan, err := buildRecreatePlan(vm, ifaces, vols, rebuildImage, imageID)
		if err != nil {
			return err
		}
		fmt.Print(plan.String())
		if rebuildDryRun {
			fmt.Println("Dry run: nothing changed.")
			return nil
		}
		if !rebuildYes {
			ok, err := readConfirmation(fmt.Sprintf("Delete %s (%s) and its boot volume, then create it again from %s? [y/N] ", plan.Name, plan.VMID, rebuildImage))
			if err != nil || !ok {
				return fmt.Errorf("not confirmed")
			}
		}

		fmt.Printf("Deleting %s (%s)...\n", plan.Name, plan.VMID)
		if err := api.DeleteVM(computeURL, tok.Value, plan.VMID); err != nil {
			return err
		}
		if err := api.WaitForVMGone(computeURL, tok.Value, plan.VMID, rebuildTimeout); err != nil {
			return err
		}
		fmt.Printf("VM %s is gone\n", plan.VMID)
		if !plan.DeleteOnTermination {
			fmt.Printf("The old boot volume %s stays (delete_on_termination false)\n", plan.BootVolumeID)
		}

		// A port that was passed to create with --ports survives the delete and keeps its MAC and IP.
		surviving := 0
		for _, n := range plan.NICs {
			ports, err := api.ListPorts(networkURL, tok.Value, map[string]string{"id": n.PortID})
			if err != nil {
				return fmt.Errorf("VM deleted; port %s lookup failed: %v; create it by hand: %s", n.PortID, err, plan.CreateCommand(false))
			}
			if len(ports.Ports) > 0 {
				surviving++
			}
		}
		usePorts := surviving == len(plan.NICs)
		if surviving > 0 && !usePorts {
			return fmt.Errorf("VM deleted; %d of %d ports survived the delete; create it by hand: %s", surviving, len(plan.NICs), plan.CreateCommand(false))
		}
		if usePorts {
			fmt.Println("The ports survived the delete; the new VM uses them.")
		}
		fmt.Printf("Creating %s: %s\n", plan.Name, plan.CreateCommand(usePorts))
		return runCreateVM(plan.CreateOptions(usePorts))
	},
}

// recreateNIC is one port of the VM. IP is empty on an unmanaged network.
type recreateNIC struct {
	PortID string
	NetID  string
	MAC    string
	IP     string
}

// recreatePlan is what a delete and create keeps from the old VM.
type recreatePlan struct {
	VMID, Name, Flavor  string
	Image, ImageID      string
	BootVolumeID        string
	BootVolumeType      string
	Size                int
	DeleteOnTermination bool
	AvailabilityZone    string
	NICs                []recreateNIC
	NotCarried          []string
}

func buildRecreatePlan(vm api.VMDetail, ifaces api.VMNetworkListResponse, vols []api.VolumeDetail, image, imageID string) (recreatePlan, error) {
	p := recreatePlan{VMID: vm.ID, Name: vm.Name, Image: image, ImageID: imageID, AvailabilityZone: vm.AvailabilityZone}
	if p.Name == "" {
		return p, fmt.Errorf("VM %s has no name", vm.ID)
	}
	p.Flavor = vm.Flavor.OriginalName
	if p.Flavor == "" {
		p.Flavor = vm.Flavor.ID
	}
	if p.Flavor == "" {
		return p, fmt.Errorf("VM %s: flavor unknown", vm.ID)
	}
	if vm.Flavor.Disk != 0 {
		return p, fmt.Errorf("VM %s boots from a local disk (flavor disk %d GB); rebuild it in place", vm.ID, vm.Flavor.Disk)
	}
	if len(vm.OSExtendedVolumesVolumesAttached) != 1 || len(vols) != 1 {
		var ids []string
		for _, v := range vm.OSExtendedVolumesVolumesAttached {
			ids = append(ids, v.ID)
		}
		return p, fmt.Errorf("VM %s has %d volumes attached (%s); recreate handles only a single boot volume", vm.ID, len(ids), strings.Join(ids, ", "))
	}
	boot := vols[0]
	if boot.ID != vm.OSExtendedVolumesVolumesAttached[0].ID {
		return p, fmt.Errorf("volume details %s do not match the attached volume %s", boot.ID, vm.OSExtendedVolumesVolumesAttached[0].ID)
	}
	if boot.Size <= 0 {
		return p, fmt.Errorf("boot volume %s has no size", boot.ID)
	}
	p.BootVolumeID = boot.ID
	p.BootVolumeType = boot.VolumeType
	p.Size = boot.Size
	p.DeleteOnTermination = vm.OSExtendedVolumesVolumesAttached[0].DeleteOnTermination

	if len(ifaces.InterfaceAttachments) == 0 {
		return p, fmt.Errorf("VM %s has no network ports", vm.ID)
	}
	for _, a := range ifaces.InterfaceAttachments {
		if a.NetID == "" || a.MacAddr == "" {
			return p, fmt.Errorf("port %s: network or MAC unknown", a.PortID)
		}
		n := recreateNIC{PortID: a.PortID, NetID: a.NetID, MAC: a.MacAddr}
		switch len(a.FixedIPs) {
		case 0:
		case 1:
			n.IP = a.FixedIPs[0].IPAddress
		default:
			return p, fmt.Errorf("port %s has %d fixed IPs; create takes one per network", a.PortID, len(a.FixedIPs))
		}
		p.NICs = append(p.NICs, n)
	}

	if p.BootVolumeType != "" && p.BootVolumeType != "nvme_ec7_2" {
		p.NotCarried = append(p.NotCarried, fmt.Sprintf("boot volume type %s (create uses nvme_ec7_2)", p.BootVolumeType))
	}
	if p.AvailabilityZone != "" {
		p.NotCarried = append(p.NotCarried, fmt.Sprintf("availability zone %s (create has no zone option)", p.AvailabilityZone))
	}
	if len(vm.Metadata) > 0 {
		var keys []string
		for k := range vm.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		p.NotCarried = append(p.NotCarried, "metadata "+strings.Join(keys, ", "))
	}
	seen := map[string]bool{}
	var groups []string
	for _, g := range vm.SecurityGroups {
		if !seen[g.Name] {
			seen[g.Name] = true
			groups = append(groups, g.Name)
		}
	}
	if len(groups) > 0 {
		p.NotCarried = append(p.NotCarried, "security groups "+strings.Join(groups, ", ")+" (create sets none)")
	}
	for _, n := range p.NICs {
		if n.IP != "" {
			p.NotCarried = append(p.NotCarried, fmt.Sprintf("MAC %s on managed network %s (create cannot set a MAC with a fixed IP)", n.MAC, n.NetID))
		}
	}
	return p, nil
}

// CreateOptions maps the plan onto create vm; usePorts reattaches the old ports.
func (p recreatePlan) CreateOptions(usePorts bool) createVMOptions {
	o := createVMOptions{Name: p.Name, Flavor: p.Flavor, Image: p.ImageID, Size: p.Size}
	var ports, nets, ips, macs []string
	for _, n := range p.NICs {
		ports = append(ports, n.PortID)
		nets = append(nets, n.NetID)
		if n.IP == "" {
			ips = append(ips, "none")
			macs = append(macs, n.MAC)
		} else {
			ips = append(ips, n.IP)
			macs = append(macs, "auto")
		}
	}
	if usePorts {
		o.Ports = strings.Join(ports, ",")
		return o
	}
	o.Networks = strings.Join(nets, ",")
	o.IPs = strings.Join(ips, ",")
	o.MACs = strings.Join(macs, ",")
	return o
}

// CreateCommand is the create vm command line that matches CreateOptions.
func (p recreatePlan) CreateCommand(usePorts bool) string {
	o := p.CreateOptions(usePorts)
	cmd := fmt.Sprintf("vhicmd create vm --name %s --flavor %s --image %s --size %d", o.Name, o.Flavor, o.Image, o.Size)
	if usePorts {
		return cmd + " --ports " + o.Ports
	}
	return cmd + fmt.Sprintf(" --networks %s --ips %s --macaddr %s", o.Networks, o.IPs, o.MACs)
}

func (p recreatePlan) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan: delete the VM and create it again\n")
	fmt.Fprintf(&b, "  vm:          %s (%s)\n", p.Name, p.VMID)
	fmt.Fprintf(&b, "  image:       %s (%s)\n", p.Image, p.ImageID)
	fmt.Fprintf(&b, "  flavor:      %s\n", p.Flavor)
	fmt.Fprintf(&b, "  boot volume: %d GB (old %s, type %s, delete_on_termination %t)\n", p.Size, p.BootVolumeID, p.BootVolumeType, p.DeleteOnTermination)
	for i, n := range p.NICs {
		ip := n.IP
		if ip == "" {
			ip = "none (unmanaged)"
		}
		fmt.Fprintf(&b, "  nic %d:       network %s mac %s ip %s (port %s)\n", i, n.NetID, n.MAC, ip, n.PortID)
	}
	for _, s := range p.NotCarried {
		fmt.Fprintf(&b, "  not kept:    %s\n", s)
	}
	fmt.Fprintf(&b, "  create:      %s\n", p.CreateCommand(false))
	return b.String()
}

// microversionAtLeast compares two "major.minor" compute microversions.
func microversionAtLeast(have, want string) bool {
	hMaj, hMin, ok1 := parseMicroversion(have)
	wMaj, wMin, ok2 := parseMicroversion(want)
	if !ok1 || !ok2 {
		return false
	}
	return hMaj > wMaj || (hMaj == wMaj && hMin >= wMin)
}

func parseMicroversion(v string) (int, int, bool) {
	majS, minS, found := strings.Cut(strings.TrimSpace(v), ".")
	if !found {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(majS)
	b, err2 := strconv.Atoi(minS)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return a, b, true
}

func init() {
	rebuildCmd.Flags().StringVar(&rebuildImage, "image", "", "image name or ID")
	rebuildCmd.Flags().BoolVarP(&rebuildYes, "yes", "y", false, "do not ask for confirmation")
	rebuildCmd.Flags().DurationVar(&rebuildTimeout, "timeout", 30*time.Minute, "how long to wait for ACTIVE (in place) or for the delete (recreate)")
	rebuildCmd.Flags().BoolVar(&rebuildRecreate, "recreate", false, "delete the VM and create it again even when it can rebuild in place")
	rebuildCmd.Flags().BoolVar(&rebuildDryRun, "dry-run", false, "print the plan and change nothing")
	rootCmd.AddCommand(rebuildCmd)
}
