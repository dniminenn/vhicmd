package cmd

import "testing"

// rebuild has its own --timeout duration; the global --timeout must not read it.
func TestRebuildTimeoutDoesNotHitTheGlobalTimeout(t *testing.T) {
	if rebuildCmd.Flags().Lookup("timeout") == nil || rootCmd.PersistentFlags().Lookup("timeout") == nil {
		t.Fatal("expected both a rebuild and a global --timeout")
	}
	if err := rebuildCmd.ParseFlags([]string{"--timeout", "45m"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rebuildCmd.Flags().Set("timeout", "30m") })
	if rootCmd.PersistentFlags().Changed("timeout") {
		t.Error("rebuild --timeout marked the global --timeout as set")
	}
}
