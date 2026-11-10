package api

import (
	"net/http"
	"strings"
	"testing"
)

const floatingIPJSON = `{"id":"fip-1","floating_ip_address":"203.0.113.10","floating_network_id":"ext-1",` +
	`"fixed_ip_address":null,"port_id":null,"router_id":null,"status":"DOWN","description":""}`

func TestListFloatingIPs(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK, `{"floatingips":[`+floatingIPJSON+`,`+
		`{"id":"fip-2","floating_ip_address":"203.0.113.11","floating_network_id":"ext-1","fixed_ip_address":"10.0.0.5","port_id":"port-1","router_id":"r1","status":"ACTIVE"}]}`)
	resp, err := ListFloatingIPs(srv.URL, "tok", nil)
	if err != nil {
		t.Fatalf("ListFloatingIPs: %v", err)
	}
	rec.check(t, "GET", "/v2.0/floatingips", "")
	if len(resp.FloatingIPs) != 2 {
		t.Fatalf("floating IPs = %+v", resp.FloatingIPs)
	}
	free, used := resp.FloatingIPs[0], resp.FloatingIPs[1]
	if free.FloatingIPAddress != "203.0.113.10" || free.PortID != "" || free.FixedIPAddress != "" {
		t.Errorf("unassociated = %+v", free)
	}
	if used.PortID != "port-1" || used.FixedIPAddress != "10.0.0.5" || used.Status != "ACTIVE" {
		t.Errorf("associated = %+v", used)
	}

	srv, _ = actionServer(t, http.StatusNotFound, `{"NeutronError":{"message":"The resource could not be found.","type":"HTTPNotFound","detail":""}}`)
	if _, err := ListFloatingIPs(srv.URL, "tok", nil); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestCreateFloatingIP(t *testing.T) {
	srv, rec := actionServer(t, http.StatusCreated, `{"floatingip":`+floatingIPJSON+`}`)
	f, err := CreateFloatingIP(srv.URL, "tok", "ext-1")
	if err != nil {
		t.Fatalf("CreateFloatingIP: %v", err)
	}
	rec.check(t, "POST", "/v2.0/floatingips", `{"floatingip":{"floating_network_id":"ext-1"}}`)
	if f.ID != "fip-1" || f.FloatingIPAddress != "203.0.113.10" {
		t.Errorf("floating IP = %+v", f)
	}

	srv, _ = actionServer(t, http.StatusBadRequest, `{"NeutronError":{"message":"Network ext-1 is not a valid external network","type":"BadRequest","detail":""}}`)
	if _, err := CreateFloatingIP(srv.URL, "tok", "ext-1"); err == nil || !strings.Contains(err.Error(), "not a valid external network") {
		t.Fatalf("err = %v, want the API message", err)
	}
}

func TestSetFloatingIPPort(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK, `{"floatingip":{"id":"fip-1","floating_ip_address":"203.0.113.10","port_id":"port-1","fixed_ip_address":"10.0.0.5","status":"ACTIVE"}}`)
	f, err := SetFloatingIPPort(srv.URL, "tok", "fip-1", "port-1")
	if err != nil {
		t.Fatalf("SetFloatingIPPort: %v", err)
	}
	rec.check(t, "PUT", "/v2.0/floatingips/fip-1", `{"floatingip":{"port_id":"port-1"}}`)
	if f.PortID != "port-1" || f.FixedIPAddress != "10.0.0.5" {
		t.Errorf("floating IP = %+v", f)
	}

	srv, rec = actionServer(t, http.StatusOK, `{"floatingip":`+floatingIPJSON+`}`)
	if _, err := SetFloatingIPPort(srv.URL, "tok", "fip-1", ""); err != nil {
		t.Fatalf("SetFloatingIPPort detach: %v", err)
	}
	rec.check(t, "PUT", "/v2.0/floatingips/fip-1", `{"floatingip":{"port_id":null}}`)

	srv, _ = actionServer(t, http.StatusNotFound, `{"NeutronError":{"message":"External network ext-1 is not reachable from subnet sub-1.","type":"ExternalGatewayForFloatingIPNotFound","detail":""}}`)
	if _, err := SetFloatingIPPort(srv.URL, "tok", "fip-1", "port-1"); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("err = %v, want the API message", err)
	}
}

func TestDeleteFloatingIP(t *testing.T) {
	srv, rec := actionServer(t, http.StatusNoContent, "")
	if err := DeleteFloatingIP(srv.URL, "tok", "fip-1"); err != nil {
		t.Fatalf("DeleteFloatingIP: %v", err)
	}
	rec.check(t, "DELETE", "/v2.0/floatingips/fip-1", "")

	srv, _ = actionServer(t, http.StatusNotFound, `{"NeutronError":{"message":"Floating IP fip-1 could not be found","type":"FloatingIPNotFound","detail":""}}`)
	if err := DeleteFloatingIP(srv.URL, "tok", "fip-1"); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestGetFloatingIPIDByAddress(t *testing.T) {
	const uuid = "0b2ffb14-4f2a-4f0e-9a43-0d1b6a1c2d3e"
	srv, rec := actionServer(t, http.StatusOK, `{"floatingips":[`+floatingIPJSON+`]}`)

	if id, err := GetFloatingIPIDByAddress(srv.URL, "tok", uuid); err != nil || id != uuid || rec.Count != 0 {
		t.Errorf("uuid = %q, %v after %d requests", id, err, rec.Count)
	}
	id, err := GetFloatingIPIDByAddress(srv.URL, "tok", "203.0.113.10")
	if err != nil || id != "fip-1" {
		t.Errorf("address = %q, %v", id, err)
	}
	if rec.Path != "/v2.0/floatingips" || rec.Query != "floating_ip_address=203.0.113.10" {
		t.Errorf("request = %s?%s", rec.Path, rec.Query)
	}
	// An address the reply does not contain is not a match, whatever the server returned.
	if _, err := GetFloatingIPIDByAddress(srv.URL, "tok", "203.0.113.99"); err == nil {
		t.Error("expected an error for no match")
	}
}
