package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fastWaits shortens the shared wait for a test and restores it afterwards.
func fastWaits(t *testing.T, timeout time.Duration) {
	t.Helper()
	oldTimeout, oldInterval := WaitTimeout, waitInterval
	WaitTimeout, waitInterval = timeout, time.Millisecond
	t.Cleanup(func() { WaitTimeout, waitInterval = oldTimeout, oldInterval })
}

// statusServer answers POST with 202 and each GET with the next status; the last one repeats.
// Statuses are served under key ("server" or "volume") with a flavor so no extra lookups happen.
func statusServer(t *testing.T, key string, statuses ...string) (*httptest.Server, *int) {
	t.Helper()
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.Method == "POST" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		status := statuses[min(polls, len(statuses)-1)]
		polls++
		fmt.Fprintf(w, `{%q:{"id":"x-1","status":%q,"flavor":{"vcpus":1,"ram":512,"disk":10}}}`, key, status)
	}))
	t.Cleanup(srv.Close)
	return srv, &polls
}

// repeat returns n copies of status followed by last.
func repeat(status string, n int, last string) []string {
	out := make([]string, 0, n+1)
	for i := 0; i < n; i++ {
		out = append(out, status)
	}
	return append(out, last)
}

func TestDefaultWaitTimeout(t *testing.T) {
	if WaitTimeout != 30*time.Minute {
		t.Errorf("WaitTimeout = %v, want 30m", WaitTimeout)
	}
}

// Each wait used to give up after 30 polls; 40 polls must now be fine within the deadline.
func TestWaitsOutlast30Polls(t *testing.T) {
	fastWaits(t, time.Minute)

	srv, polls := statusServer(t, "server", repeat("ACTIVE", 40, "SHUTOFF")...)
	if err := StopVM(srv.URL, "tok", "x-1"); err != nil || *polls != 41 {
		t.Errorf("StopVM: %v after %d polls", err, *polls)
	}

	srv, polls = statusServer(t, "server", repeat("REBOOT", 40, "ACTIVE")...)
	if err := RebootVM(srv.URL, "tok", "x-1", "SOFT"); err != nil || *polls != 41 {
		t.Errorf("RebootVM: %v after %d polls", err, *polls)
	}

	srv, polls = statusServer(t, "server", repeat("BUILD", 40, "ACTIVE")...)
	vm, err := WaitForStatus(srv.URL, "tok", "x-1", "active")
	if err != nil || vm.Status != "ACTIVE" || *polls != 41 {
		t.Errorf("WaitForStatus: %+v, %v after %d polls", vm.Status, err, *polls)
	}

	srv, polls = statusServer(t, "volume", repeat("creating", 40, "available")...)
	if err := WaitForVolumeStatus(srv.URL, "tok", "x-1", "available"); err != nil || *polls != 41 {
		t.Errorf("WaitForVolumeStatus: %v after %d polls", err, *polls)
	}
}

func TestWaitsTimeOutWithTheirOwnMessage(t *testing.T) {
	fastWaits(t, 20*time.Millisecond)

	vmSrv, _ := statusServer(t, "server", "BUILD")
	volSrv, _ := statusServer(t, "volume", "creating")
	_, statusErr := WaitForStatus(vmSrv.URL, "tok", "x-1", "ACTIVE")
	for want, err := range map[string]error{
		"timeout waiting for VM to stop":                  StopVM(vmSrv.URL, "tok", "x-1"),
		"timeout waiting for VM to reboot":                RebootVM(vmSrv.URL, "tok", "x-1", "HARD"),
		`timeout waiting for VM to reach status "ACTIVE"`: statusErr,
		"timeout waiting for volume to become available":  WaitForVolumeStatus(volSrv.URL, "tok", "x-1", "available"),
	} {
		if err == nil || err.Error() != want {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}

func TestWaitsStopOnErrorState(t *testing.T) {
	fastWaits(t, time.Minute)

	vmSrv, vmPolls := statusServer(t, "server", "BUILD", "ERROR")
	if err := StopVM(vmSrv.URL, "tok", "x-1"); err == nil || !strings.Contains(err.Error(), "error state while stopping") {
		t.Errorf("StopVM: %v", err)
	}
	if *vmPolls != 2 {
		t.Errorf("polls = %d, want the wait to end at the error state", *vmPolls)
	}
	if _, err := WaitForStatus(vmSrv.URL, "tok", "x-1", "ACTIVE"); err == nil || err.Error() != "VM creation failed: status ERROR" {
		t.Errorf("WaitForStatus: %v", err)
	}

	volSrv, _ := statusServer(t, "volume", "creating", "error")
	if err := WaitForVolumeStatus(volSrv.URL, "tok", "x-1", "available"); err == nil || !strings.Contains(err.Error(), "error state") {
		t.Errorf("WaitForVolumeStatus: %v", err)
	}
}
