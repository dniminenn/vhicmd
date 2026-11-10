package responseparser

import "testing"

func TestVMNetworkRows(t *testing.T) {
	rows := vmNetworkRows([]NetworkDetail{
		{Name: "unmanaged", UUID: "net-1", MacAddr: "fa:16:3e:00:00:01"},
		{Name: "managed", UUID: "net-2", MacAddr: "fa:16:3e:00:00:02", PortID: "port-2",
			IPs: []IPDetail{{Address: "10.0.0.5"}, {Address: "10.0.0.6"}}},
	})
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	// Columns: Network Name, IP Address, MAC Address, Network UUID, Port ID.
	for i, row := range rows {
		if len(row) != 5 {
			t.Errorf("row %d has %d cells, the table has 5 columns: %q", i, len(row), row)
		}
	}
	if got := rows[0][1:]; got[0] != "N/A" || got[1] != "fa:16:3e:00:00:01" || got[2] != "net-1" || got[3] != "N/A" {
		t.Errorf("network without IPs = %q", got)
	}
	if got := rows[2][1:]; got[0] != "10.0.0.6" || got[1] != "fa:16:3e:00:00:02" || got[2] != "net-2" || got[3] != "port-2" {
		t.Errorf("second IP row = %q", got)
	}
}
