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
