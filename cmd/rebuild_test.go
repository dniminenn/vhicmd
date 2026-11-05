package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jessegalley/vhicmd/api"
)

func TestMicroversionAtLeast(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"2.90", "2.93", false},
		{"2.93", "2.93", true},
		{"2.100", "2.93", true},
		{"3.0", "2.93", true},
		{"2.9", "2.93", false},
		{"", "2.93", false},
		{"2", "2.93", false},
	}
	for _, c := range cases {
		if got := microversionAtLeast(c.have, c.want); got != c.ok {
			t.Errorf("microversionAtLeast(%q, %q) = %v, want %v", c.have, c.want, got, c.ok)
		}
	}
}

func TestComputeVersionRoot(t *testing.T) {
	cases := map[string]string{
		"https://vhi:8774/v2.1":                                  "https://vhi:8774/v2.1",
		"https://vhi:8774/v2.1/":                                 "https://vhi:8774/v2.1",
		"https://vhi:8774/v2.1/0123456789abcdef0123456789abcdef": "https://vhi:8774/v2.1",
		"https://vhi:8774/compute/v2.1/project":                  "https://vhi:8774/compute/v2.1",
	}
	for in, want := range cases {
		if got := api.ComputeVersionRoot(in); got != want {
			t.Errorf("ComputeVersionRoot(%q) = %q, want %q", in, got, want)
		}
	}
}

// whm2 as the compute API returns it: one boot volume, one unmanaged port.
func recreateFixture(t *testing.T) (api.VMDetail, api.VMNetworkListResponse, []api.VolumeDetail) {
	t.Helper()
	var vm api.VMDetail
	if err := json.Unmarshal([]byte(`{
		"id": "vm-1", "name": "whm2", "status": "ACTIVE",
		"OS-EXT-AZ:availability_zone": "nova",
		"flavor": {"original_name": "8vcpu-8GiB", "disk": 0, "vcpus": 8, "ram": 8192},
		"os-extended-volumes:volumes_attached": [{"id": "vol-1", "delete_on_termination": true}],
		"security_groups": [{"name": "default"}, {"name": "default"}]
	}`), &vm); err != nil {
		t.Fatal(err)
	}
	var ifaces api.VMNetworkListResponse
	if err := json.Unmarshal([]byte(`{"interfaceAttachments": [
		{"port_id": "port-1", "net_id": "net-ext", "mac_addr": "fa:16:3e:d8:47:05", "fixed_ips": []}
	]}`), &ifaces); err != nil {
		t.Fatal(err)
	}
	vols := []api.VolumeDetail{{ID: "vol-1", Size: 100, VolumeType: "nvme_ec7_2", Bootable: "true"}}
	return vm, ifaces, vols
}

func TestBuildRecreatePlanUnmanaged(t *testing.T) {
	vm, ifaces, vols := recreateFixture(t)
	p, err := buildRecreatePlan(vm, ifaces, vols, "whm2-baseline", "img-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "whm2" || p.Flavor != "8vcpu-8GiB" || p.Size != 100 || p.ImageID != "img-1" || !p.DeleteOnTermination {
		t.Errorf("plan = %+v", p)
	}
	o := p.CreateOptions(false)
	want := createVMOptions{Name: "whm2", Flavor: "8vcpu-8GiB", Image: "img-1", Size: 100,
		Networks: "net-ext", IPs: "none", MACs: "fa:16:3e:d8:47:05"}
	if o != want {
		t.Errorf("CreateOptions = %+v, want %+v", o, want)
	}
	if got := p.CreateCommand(false); got != "vhicmd create vm --name whm2 --flavor 8vcpu-8GiB --image img-1 --size 100 --networks net-ext --ips none --macaddr fa:16:3e:d8:47:05" {
		t.Errorf("CreateCommand = %q", got)
	}
	if o := p.CreateOptions(true); o.Ports != "port-1" || o.Networks != "" || o.IPs != "" || o.MACs != "" {
		t.Errorf("CreateOptions(ports) = %+v", o)
	}
	s := p.String()
	for _, w := range []string{"8vcpu-8GiB", "100 GB", "fa:16:3e:d8:47:05", "none (unmanaged)", "availability zone nova", "security groups default (create sets none)"} {
		if !strings.Contains(s, w) {
			t.Errorf("plan text lacks %q:\n%s", w, s)
		}
	}
}

func TestBuildRecreatePlanManagedAndMixed(t *testing.T) {
	vm, _, vols := recreateFixture(t)
	var ifaces api.VMNetworkListResponse
	if err := json.Unmarshal([]byte(`{"interfaceAttachments": [
		{"port_id": "port-1", "net_id": "net-int", "mac_addr": "fa:16:3e:00:00:01", "fixed_ips": [{"ip_address": "10.0.0.5", "subnet_id": "s"}]},
		{"port_id": "port-2", "net_id": "net-ext", "mac_addr": "fa:16:3e:00:00:02", "fixed_ips": []}
	]}`), &ifaces); err != nil {
		t.Fatal(err)
	}
	p, err := buildRecreatePlan(vm, ifaces, vols, "img", "img-1")
	if err != nil {
		t.Fatal(err)
	}
	o := p.CreateOptions(false)
	if o.Networks != "net-int,net-ext" || o.IPs != "10.0.0.5,none" || o.MACs != "auto,fa:16:3e:00:00:02" {
		t.Errorf("CreateOptions = %+v", o)
	}
	if o := p.CreateOptions(true); o.Ports != "port-1,port-2" {
		t.Errorf("CreateOptions(ports) = %+v", o)
	}
	if !strings.Contains(p.String(), "MAC fa:16:3e:00:00:01 on managed network net-int") {
		t.Errorf("plan text does not warn about the managed MAC:\n%s", p.String())
	}
}

func TestBuildRecreatePlanRefuses(t *testing.T) {
	cases := map[string]func(*api.VMDetail, *api.VMNetworkListResponse, *[]api.VolumeDetail){
		"local disk": func(vm *api.VMDetail, _ *api.VMNetworkListResponse, _ *[]api.VolumeDetail) {
			vm.Flavor.Disk = 40
		},
		"data volume": func(vm *api.VMDetail, _ *api.VMNetworkListResponse, vols *[]api.VolumeDetail) {
			vm.OSExtendedVolumesVolumesAttached = append(vm.OSExtendedVolumesVolumesAttached, api.VmVolume{ID: "vol-2"})
			*vols = append(*vols, api.VolumeDetail{ID: "vol-2", Size: 10})
		},
		"no ports": func(_ *api.VMDetail, ifaces *api.VMNetworkListResponse, _ *[]api.VolumeDetail) {
			ifaces.InterfaceAttachments = nil
		},
		"two fixed IPs": func(_ *api.VMDetail, ifaces *api.VMNetworkListResponse, _ *[]api.VolumeDetail) {
			ifaces.InterfaceAttachments[0].FixedIPs = append(ifaces.InterfaceAttachments[0].FixedIPs,
				struct {
					IPAddress string `json:"ip_address"`
					SubnetID  string `json:"subnet_id"`
				}{IPAddress: "10.0.0.5"},
				struct {
					IPAddress string `json:"ip_address"`
					SubnetID  string `json:"subnet_id"`
				}{IPAddress: "10.0.0.6"})
		},
		"no flavor": func(vm *api.VMDetail, _ *api.VMNetworkListResponse, _ *[]api.VolumeDetail) {
			vm.Flavor.OriginalName = ""
		},
		"volume mismatch": func(_ *api.VMDetail, _ *api.VMNetworkListResponse, vols *[]api.VolumeDetail) {
			(*vols)[0].ID = "vol-other"
		},
	}
	for name, mutate := range cases {
		vm, ifaces, vols := recreateFixture(t)
		mutate(&vm, &ifaces, &vols)
		if _, err := buildRecreatePlan(vm, ifaces, vols, "img", "img-1"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
