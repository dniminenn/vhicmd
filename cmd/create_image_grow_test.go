package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseHumanSize(t *testing.T) {
	cases := map[string]int64{
		"20G":   20 << 30,
		"20g":   20 << 30,
		"20GB":  20 << 30,
		"20GiB": 20 << 30,
		"25G":   25 << 30,
		"500M":  500 << 20,
		"1T":    1 << 40,
		"1024":  1024,
	}
	for in, want := range cases {
		got, err := parseHumanSize(in)
		if err != nil {
			t.Errorf("parseHumanSize(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseHumanSize(%q) = %d, want %d", in, got, want)
		}
	}
	if _, err := parseHumanSize("abc"); err == nil {
		t.Errorf("expected error for %q", "abc")
	}
}

func TestCeilDivGiB(t *testing.T) {
	cases := map[int64]int{
		21587034112:   21, // Rocky8-mqueue1G virtual size -> min_disk 21
		43061870592:   41, // + 20 GiB -> 41
		1 << 30:       1,
		(1 << 30) + 1: 2,
	}
	for in, want := range cases {
		if got := ceilDivGiB(in); got != want {
			t.Errorf("ceilDivGiB(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestParseGuestfishSections(t *testing.T) {
	out := "===LVS===\n/dev/rootvg/rootlv\n/dev/rootvg/varlv\n===PVS===\n/dev/sda3\n"
	lvs, pvs := parseGuestfishSections(out)
	if len(lvs) != 2 || lvs[1] != "/dev/rootvg/varlv" {
		t.Errorf("lvs = %v", lvs)
	}
	if len(pvs) != 1 || pvs[0] != "/dev/sda3" {
		t.Errorf("pvs = %v", pvs)
	}
}

// Integration test against the real downloaded base image, when present.
func TestDetectPVPartition_RealImage(t *testing.T) {
	const img = "/home/jr/vhi-work/Rocky8-mqueue1G.qcow2"
	if _, err := os.Stat(img); err != nil {
		t.Skipf("base image not present: %v", err)
	}
	part, err := detectPVPartition(img, "/dev/rootvg/varlv")
	if err != nil {
		t.Fatalf("detectPVPartition error: %v", err)
	}
	if part != "/dev/sda3" {
		t.Errorf("detectPVPartition = %q, want /dev/sda3", part)
	}

	// Missing LV should error clearly.
	if _, err := detectPVPartition(img, "/dev/rootvg/nope"); err == nil {
		t.Errorf("expected error for missing LV")
	}
}

// TestGrowPipeline_RealImage exercises the exact qemu-img + virt-resize
// orchestration used by runGrowImageFromImage (everything except the VHI
// upload) against the real base image, and verifies /var (varlv) actually grew.
func TestGrowPipeline_RealImage(t *testing.T) {
	const (
		img      = "/home/jr/vhi-work/Rocky8-mqueue1G.qcow2"
		baseSize = int64(21587034112) // known virtual size of Rocky8-mqueue1G
		growBy   = int64(20) << 30
	)
	for _, bin := range []string{"qemu-img", "virt-resize", "guestfish"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	if _, err := os.Stat(img); err != nil {
		t.Skipf("base image not present: %v", err)
	}

	part, err := detectPVPartition(img, "/dev/rootvg/varlv")
	if err != nil {
		t.Fatalf("detectPVPartition: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "grown.qcow2")
	newSize := baseSize + growBy
	if err := runCmd("qemu-img", "create", "-f", "qcow2", dst, strconv.FormatInt(newSize, 10)); err != nil {
		t.Fatalf("qemu-img create: %v", err)
	}
	if err := runCmd("virt-resize", "--expand", part, "--lv-expand", "/dev/rootvg/varlv", img, dst); err != nil {
		t.Fatalf("virt-resize: %v", err)
	}

	// Confirm varlv grew well past its original 5 GiB.
	cmd := exec.Command("guestfish", "--ro", "-a", dst, "run", ":", "lvs-full")
	cmd.Env = libguestfsEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("guestfish lvs-full: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "varlv") {
		t.Fatalf("varlv not found in grown image:\n%s", out)
	}
	// 26843545600 == 25 GiB, the expected grown size (5 + 20).
	if !strings.Contains(string(out), "26843545600") {
		t.Errorf("expected varlv lv_size 26843545600 (25 GiB) in output:\n%s", out)
	}
}
