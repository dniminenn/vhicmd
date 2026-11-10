package api

import (
	"net/http"
	"testing"
)

func TestUpdatePortAllowedAddressPairs(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK, `{"port":{"id":"port-1","network_id":"net-1","allowed_address_pairs":[{"ip_address":"10.0.0.100"}]}}`)
	if _, err := UpdatePortAllowedAddressPairs(srv.URL, "tok", "port-1", []AllowedAddressPair{{IPAddress: "10.0.0.100"}}); err != nil {
		t.Fatalf("UpdatePortAllowedAddressPairs: %v", err)
	}
	rec.check(t, "PUT", "/v2.0/ports/port-1", `{"port":{"allowed_address_pairs":[{"ip_address":"10.0.0.100"}]}}`)

	// Clearing the pairs must send an empty list, for a nil slice too.
	for _, pairs := range [][]AllowedAddressPair{{}, nil} {
		srv, rec = actionServer(t, http.StatusOK, `{"port":{"id":"port-1","network_id":"net-1","allowed_address_pairs":[]}}`)
		if _, err := UpdatePortAllowedAddressPairs(srv.URL, "tok", "port-1", pairs); err != nil {
			t.Fatalf("UpdatePortAllowedAddressPairs: %v", err)
		}
		rec.check(t, "PUT", "/v2.0/ports/port-1", `{"port":{"allowed_address_pairs":[]}}`)
	}

	// A different update must leave the pairs alone.
	on := true
	srv, rec = actionServer(t, http.StatusOK, `{"port":{"id":"port-1","network_id":"net-1"}}`)
	if _, err := UpdatePort(srv.URL, "tok", "port-1", PortUpdateFields{PortSecurityEnabled: &on}); err != nil {
		t.Fatal(err)
	}
	rec.check(t, "PUT", "/v2.0/ports/port-1", `{"port":{"port_security_enabled":true}}`)
}
