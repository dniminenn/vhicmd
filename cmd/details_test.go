package cmd

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jessegalley/vhicmd/api"
)

func TestDescribeUserData(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("#cloud-config\nhostname: web1\n"))

	summary, decoded := describeUserData("", true)
	if summary != "not returned by the API" || decoded != "" {
		t.Errorf("empty = %q, %q", summary, decoded)
	}

	summary, decoded = describeUserData(encoded, false)
	if !strings.HasPrefix(summary, "present, 29 bytes") || decoded != "" {
		t.Errorf("hidden = %q, %q", summary, decoded)
	}

	summary, decoded = describeUserData(encoded, true)
	if summary != "present, 29 bytes" || decoded != "#cloud-config\nhostname: web1\n" {
		t.Errorf("shown = %q, %q", summary, decoded)
	}

	if _, decoded = describeUserData("not base64!", true); decoded != "not base64!" {
		t.Errorf("invalid base64 = %q, want it as returned", decoded)
	}
	if _, decoded = describeUserData("not base64!", false); decoded != "" {
		t.Errorf("invalid base64 without the flag = %q, want nothing", decoded)
	}
}

func TestVMPortRows(t *testing.T) {
	off := false
	rows := vmPortRows([]api.Port{
		{
			ID:                  "port-1",
			MACAddress:          "fa:16:3e:00:00:01",
			FixedIPs:            []api.IPInfo{{IPAddress: "10.0.0.5"}, {IPAddress: "10.0.0.6"}},
			PortSecurityEnabled: &off,
			AllowedAddressPairs: []api.AllowedAddressPair{
				{IPAddress: "10.0.0.100", MACAddress: "fa:16:3e:00:00:01"},
				{IPAddress: "10.0.0.101", MACAddress: "fa:16:3e:00:00:99"},
			},
		},
		{ID: "port-2", MACAddress: "fa:16:3e:00:00:02", SecurityGroups: []string{"sg-1", "sg-2"}},
	})
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0][2] != "10.0.0.5, 10.0.0.6" || !strings.Contains(rows[0][3], "FALSE") || rows[0][4] != "none" ||
		rows[0][5] != "10.0.0.100, 10.0.0.101 (fa:16:3e:00:00:99)" {
		t.Errorf("row 0 = %q", rows[0])
	}
	if rows[1][2] != "none" || rows[1][3] != "N/A" || rows[1][4] != "sg-1, sg-2" || rows[1][5] != "none" {
		t.Errorf("row 1 = %q", rows[1])
	}
}
