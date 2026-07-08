package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jessegalley/vhicmd/api"
)

// Flags for `create image --from-image ...` (grow an LV inside an existing image).
var (
	flagFromImage   string
	flagGrowSize    string
	flagExpandLV    string
	flagExpandPart  string
	flagGrowMinDisk int
	flagGrowVis     string
	flagWorkDir     string
	flagKeepTemp    bool
)

func init() {
	createImageCmd.Flags().StringVar(&flagFromImage, "from-image", "", "Source image (ID or name) to build a larger copy from by expanding an LV")
	createImageCmd.Flags().StringVar(&flagGrowSize, "grow", "", "Amount to add to the disk and target LV, e.g. 20G, 25G, 500M (binary units)")
	createImageCmd.Flags().StringVar(&flagExpandLV, "expand-lv", "/dev/rootvg/varlv", "Logical volume to expand into the new space")
	createImageCmd.Flags().StringVar(&flagExpandPart, "expand-part", "", "Partition holding the LVM PV to expand (default: auto-detect the VG's PV)")
	createImageCmd.Flags().IntVar(&flagGrowMinDisk, "min-disk", 0, "min_disk (GB) for the new image (default: auto = ceil of new virtual size)")
	createImageCmd.Flags().StringVar(&flagGrowVis, "visibility", "shared", "Visibility of the new image: shared, private, public, community")
	createImageCmd.Flags().StringVar(&flagWorkDir, "work-dir", "", "Directory for temporary download/build files (default: a temp dir; needs a few GB free)")
	createImageCmd.Flags().BoolVar(&flagKeepTemp, "keep-temp", false, "Keep temporary download/build files instead of deleting them")
}

// runGrowImageFromImage downloads an existing qcow2 image, uses virt-resize to
// grow a logical volume (default /dev/rootvg/varlv) by --grow, and uploads the
// result as a new image. It shells out to qemu-img and virt-resize (libguestfs).
func runGrowImageFromImage(imageURL, token string) error {
	if flagGrowSize == "" {
		return fmt.Errorf("--grow is required with --from-image (e.g. --grow 20G)")
	}
	if flagImageName == "" {
		return fmt.Errorf("--name is required with --from-image")
	}

	// External tooling required for offline image surgery.
	for _, bin := range []string{"qemu-img", "virt-resize", "guestfish"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%q not found in PATH: this command needs qemu-utils (qemu-img) and libguestfs-tools (virt-resize, guestfish)", bin)
		}
	}

	growBytes, err := parseHumanSize(flagGrowSize)
	if err != nil {
		return fmt.Errorf("invalid --grow value %q: %v", flagGrowSize, err)
	}
	if growBytes <= 0 {
		return fmt.Errorf("--grow must be positive")
	}

	// Resolve the source image and read its details.
	srcID, err := api.GetImageIDByName(imageURL, token, flagFromImage)
	if err != nil {
		return fmt.Errorf("failed to resolve source image %q: %v", flagFromImage, err)
	}
	details, err := api.GetImageDetails(imageURL, token, srcID)
	if err != nil {
		return fmt.Errorf("failed to get source image details: %v", err)
	}
	if details.DiskFormat != "qcow2" {
		return fmt.Errorf("source image disk_format is %q; only qcow2 is supported", details.DiskFormat)
	}
	if details.VirtualSize <= 0 {
		return fmt.Errorf("source image has no known virtual_size; cannot compute new size")
	}

	newSize := details.VirtualSize + growBytes
	minDisk := flagGrowMinDisk
	if minDisk == 0 {
		minDisk = ceilDivGiB(newSize)
	}

	fmt.Printf("Source image : %s (%s)\n", details.Name, srcID)
	fmt.Printf("  virtual size : %s\n", humanBytes(details.VirtualSize))
	fmt.Printf("  disk format  : %s\n", details.DiskFormat)
	fmt.Printf("New image    : %s\n", flagImageName)
	fmt.Printf("  grow LV      : %s by %s\n", flagExpandLV, humanBytes(growBytes))
	fmt.Printf("  virtual size : %s\n", humanBytes(newSize))
	fmt.Printf("  min_disk     : %d GB\n", minDisk)

	// Staging directory.
	workDir := flagWorkDir
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "vhicmd-grow-")
		if err != nil {
			return fmt.Errorf("failed to create work dir: %v", err)
		}
	} else {
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			return fmt.Errorf("failed to create work dir %q: %v", workDir, err)
		}
	}
	srcPath := filepath.Join(workDir, "source.qcow2")
	dstPath := filepath.Join(workDir, "grown.qcow2")
	if !flagKeepTemp {
		defer func() {
			_ = os.Remove(srcPath)
			_ = os.Remove(dstPath)
			if flagWorkDir == "" {
				_ = os.Remove(workDir)
			}
		}()
	}

	// 1. Download the source image.
	fmt.Printf("\nDownloading source image to %s ...\n", srcPath)
	if err := api.DownloadImage(imageURL, token, srcID, srcPath); err != nil {
		return fmt.Errorf("failed to download source image: %v", err)
	}

	// 2. Determine which partition to expand (the VG's PV) if not supplied.
	expandPart := flagExpandPart
	if expandPart == "" {
		expandPart, err = detectPVPartition(srcPath, flagExpandLV)
		if err != nil {
			return fmt.Errorf("failed to auto-detect PV partition (pass --expand-part): %v", err)
		}
		fmt.Printf("Auto-detected PV partition: %s\n", expandPart)
	}

	// 3. Create the (larger) destination qcow2.
	fmt.Printf("Creating destination qcow2 (%s) ...\n", humanBytes(newSize))
	if err := runCmd("qemu-img", "create", "-f", "qcow2", dstPath, strconv.FormatInt(newSize, 10)); err != nil {
		return fmt.Errorf("qemu-img create failed: %v", err)
	}

	// 4. virt-resize: expand the PV partition and the target LV (and its fs).
	fmt.Printf("Running virt-resize (expand %s, lv-expand %s) ...\n", expandPart, flagExpandLV)
	if err := runCmd("virt-resize", "--expand", expandPart, "--lv-expand", flagExpandLV, srcPath, dstPath); err != nil {
		return fmt.Errorf("virt-resize failed: %v", err)
	}

	// 5. Upload the grown image as a new Glance image.
	f, err := os.Open(dstPath)
	if err != nil {
		return fmt.Errorf("failed to open grown image: %v", err)
	}
	defer f.Close()

	req := api.CreateImageRequest{
		Name:         flagImageName,
		ContainerFmt: "bare",
		DiskFmt:      "qcow2",
		MinDisk:      minDisk,
		Visibility:   flagGrowVis,
	}
	fmt.Printf("\nUploading new image %q ...\n", flagImageName)
	newID, err := api.CreateAndUploadImage(imageURL, token, req, f)
	if err != nil {
		return fmt.Errorf("failed to create/upload new image: %v", err)
	}

	fmt.Printf("\nImage created: ID: %s, Name: %s (min_disk %d GB)\n", newID, flagImageName, minDisk)
	if flagKeepTemp {
		fmt.Printf("Temporary files kept in %s\n", workDir)
	}
	return nil
}

// detectPVPartition inspects the qcow2 with guestfish and returns the partition
// device that holds the LVM PV for the volume group of lvPath (e.g.
// /dev/rootvg/varlv -> /dev/sda3). Errors if the LV is missing or the VG spans
// more than one PV (in which case the caller should pass --expand-part).
func detectPVPartition(imagePath, lvPath string) (string, error) {
	script := "run\necho ===LVS===\nlvs\necho ===PVS===\npvs\n"
	cmd := exec.Command("guestfish", "--ro", "-a", imagePath)
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = libguestfsEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("guestfish probe failed: %v\n%s", err, string(out))
	}

	lvs, pvs := parseGuestfishSections(string(out))
	found := false
	for _, lv := range lvs {
		if lv == lvPath {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("logical volume %q not found in image (found: %s)", lvPath, strings.Join(lvs, ", "))
	}
	if len(pvs) == 0 {
		return "", fmt.Errorf("no physical volumes found in image")
	}
	if len(pvs) > 1 {
		return "", fmt.Errorf("volume group spans multiple PVs (%s); specify one with --expand-part", strings.Join(pvs, ", "))
	}
	return pvs[0], nil
}

// parseGuestfishSections splits guestfish output produced by the probe script
// into the LV and PV device lists.
func parseGuestfishSections(out string) (lvs, pvs []string) {
	section := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "===LVS===":
			section = "lvs"
			continue
		case line == "===PVS===":
			section = "pvs"
			continue
		}
		if !strings.HasPrefix(line, "/dev/") {
			continue
		}
		switch section {
		case "lvs":
			lvs = append(lvs, line)
		case "pvs":
			pvs = append(pvs, line)
		}
	}
	return lvs, pvs
}

// libguestfsEnv returns the current environment, defaulting LIBGUESTFS_BACKEND
// to "direct" when unset (more reliable across hosts where the supermin
// appliance cannot read the host kernel).
func libguestfsEnv() []string {
	env := os.Environ()
	if os.Getenv("LIBGUESTFS_BACKEND") == "" {
		env = append(env, "LIBGUESTFS_BACKEND=direct")
	}
	return env
}

// runCmd runs an external command, streaming its output to the user.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = libguestfsEnv()
	return cmd.Run()
}

// parseHumanSize parses a size like "20G", "500M", "1T", or a plain byte count.
// Suffixes are binary (K=1024, M=1024^2, G=1024^3, T=1024^4). A trailing "B" or
// "iB" is tolerated (e.g. "20GiB", "20GB").
func parseHumanSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	// Strip a trailing "iB" or "B".
	u := strings.ToUpper(s)
	u = strings.TrimSuffix(u, "IB")
	u = strings.TrimSuffix(u, "B")

	mult := int64(1)
	if len(u) > 0 {
		switch u[len(u)-1] {
		case 'K':
			mult = 1 << 10
			u = u[:len(u)-1]
		case 'M':
			mult = 1 << 20
			u = u[:len(u)-1]
		case 'G':
			mult = 1 << 30
			u = u[:len(u)-1]
		case 'T':
			mult = 1 << 40
			u = u[:len(u)-1]
		}
	}
	u = strings.TrimSpace(u)
	val, err := strconv.ParseInt(u, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("not a number: %q", u)
	}
	return val * mult, nil
}

// ceilDivGiB returns the number of whole GiB needed to hold b bytes, rounded up.
func ceilDivGiB(b int64) int {
	const giB = int64(1) << 30
	return int((b + giB - 1) / giB)
}

// humanBytes renders a byte count as a human-friendly string.
func humanBytes(b int64) string {
	const (
		kiB = int64(1) << 10
		miB = int64(1) << 20
		giB = int64(1) << 30
	)
	switch {
	case b >= giB:
		return fmt.Sprintf("%.2f GiB (%d bytes)", float64(b)/float64(giB), b)
	case b >= miB:
		return fmt.Sprintf("%.2f MiB (%d bytes)", float64(b)/float64(miB), b)
	case b >= kiB:
		return fmt.Sprintf("%.2f KiB (%d bytes)", float64(b)/float64(kiB), b)
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}
