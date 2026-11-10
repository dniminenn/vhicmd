package api

import (
	"net/http"
	"testing"
)

func TestListVMPorts(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK, `{"ports":[{
		"id":"port-1","device_id":"vm-1","mac_address":"fa:16:3e:00:00:01","network_id":"net-1",
		"port_security_enabled":false,
		"security_groups":["sg-1","sg-2"],
		"fixed_ips":[{"subnet_id":"sub-1","ip_address":"10.0.0.5"}],
		"allowed_address_pairs":[{"ip_address":"10.0.0.100","mac_address":"fa:16:3e:00:00:01"}]}]}`)

	ports, err := ListVMPorts(srv.URL, "tok", "vm-1")
	if err != nil {
		t.Fatalf("ListVMPorts: %v", err)
	}
	rec.check(t, "GET", "/v2.0/ports", "")
	if rec.Query != "device_id=vm-1" {
		t.Errorf("query = %q", rec.Query)
	}
	if len(ports) != 1 {
		t.Fatalf("ports = %+v", ports)
	}
	p := ports[0]
	if p.PortSecurityEnabled == nil || *p.PortSecurityEnabled {
		t.Errorf("port security = %v, want false", p.PortSecurityEnabled)
	}
	if len(p.SecurityGroups) != 2 || p.SecurityGroups[1] != "sg-2" {
		t.Errorf("security groups = %v", p.SecurityGroups)
	}
	if len(p.FixedIPs) != 1 || p.FixedIPs[0].IPAddress != "10.0.0.5" {
		t.Errorf("fixed IPs = %+v", p.FixedIPs)
	}
	if len(p.AllowedAddressPairs) != 1 || p.AllowedAddressPairs[0].IPAddress != "10.0.0.100" {
		t.Errorf("allowed address pairs = %+v", p.AllowedAddressPairs)
	}

	srv, _ = actionServer(t, http.StatusForbidden, `{"NeutronError":{"message":"forbidden","type":"PolicyNotAuthorized","detail":""}}`)
	if _, err := ListVMPorts(srv.URL, "tok", "vm-1"); err == nil {
		t.Fatal("expected an error for a 403 response")
	}
}

func TestGetVMDetailsUserData(t *testing.T) {
	flavor := `"flavor":{"vcpus":1,"ram":512,"disk":10}`
	cases := []struct {
		name, body, want string
	}{
		{"returned", `{"server":{"id":"vm-1","OS-EXT-SRV-ATTR:user_data":"I2Nsb3VkLWNvbmZpZwo=",` + flavor + `}}`, "I2Nsb3VkLWNvbmZpZwo="},
		{"null", `{"server":{"id":"vm-1","OS-EXT-SRV-ATTR:user_data":null,` + flavor + `}}`, ""},
		{"absent", `{"server":{"id":"vm-1",` + flavor + `}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := actionServer(t, http.StatusOK, tc.body)
			vm, err := GetVMDetails(srv.URL, "tok", "vm-1")
			if err != nil {
				t.Fatalf("GetVMDetails: %v", err)
			}
			rec.check(t, "GET", "/servers/vm-1", "")
			if vm.UserData != tc.want {
				t.Errorf("user data = %q, want %q", vm.UserData, tc.want)
			}
			// user_data is in the server body from compute microversion 2.3.
			if got := rec.Header.Get("X-OpenStack-Nova-API-Version"); got != "2.72" {
				t.Errorf("microversion header = %q", got)
			}
		})
	}
}
