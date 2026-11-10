package cmd

import "testing"

// Every filter list ports reads must be a registered flag, or it can never be set.
func TestListPortsFlags(t *testing.T) {
	for _, name := range []string{"device-id", "network-id", "mac-address", "status", "name"} {
		if listPortsCmd.Flags().Lookup(name) == nil {
			t.Errorf("list ports has no --%s flag", name)
		}
	}
}
