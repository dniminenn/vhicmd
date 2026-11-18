package cmd

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestImageWaitFlags(t *testing.T) {
	for name, cmd := range map[string]*cobra.Command{"create image": createImageCmd, "download volume": downloadVolumeCmd} {
		flag := cmd.Flags().Lookup("wait")
		if flag == nil {
			t.Errorf("%s has no --wait flag", name)
			continue
		}
		if d, err := time.ParseDuration(flag.DefValue); err != nil || d != 30*time.Minute {
			t.Errorf("%s --wait default = %q, want 30m", name, flag.DefValue)
		}
	}
}

func TestImageVolumeSize(t *testing.T) {
	const gb = 1024 * 1024 * 1024
	cases := []struct {
		requested  int
		imageBytes int64
		minDisk    int
		want       int
	}{
		{0, 409403392, 3, 3},   // min_disk is larger than the image file
		{0, 3 * gb, 0, 3},      // a whole number of GB is not rounded up
		{0, 3*gb + 1, 0, 4},    // the file size rounds up
		{20, 409403392, 3, 20}, // --size above what the image needs
		{2, 409403392, 3, 3},   // --size below what the image needs
	}
	for _, c := range cases {
		if got := imageVolumeSize(c.requested, c.imageBytes, c.minDisk); got != c.want {
			t.Errorf("imageVolumeSize(%d, %d, %d) = %d, want %d", c.requested, c.imageBytes, c.minDisk, got, c.want)
		}
	}
}
