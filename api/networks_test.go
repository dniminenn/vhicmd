package api

import (
	"net/http"
	"testing"
)

func TestDetachNetworkFromVM(t *testing.T) {
	// nova documents 202 for this call; 200 was the only code accepted before.
	for _, status := range []int{http.StatusAccepted, http.StatusOK} {
		srv, rec := actionServer(t, status, "")
		if err := DetachNetworkFromVM(srv.URL, "tok", "vm-1", "port-1"); err != nil {
			t.Errorf("status %d: %v", status, err)
		}
		rec.check(t, "DELETE", "/servers/vm-1/os-interface/port-1", "")
	}

	srv, _ := actionServer(t, http.StatusNotFound, `{"itemNotFound":{"code":404,"message":"Port port-1 is not attached"}}`)
	if err := DetachNetworkFromVM(srv.URL, "tok", "vm-1", "port-1"); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}
