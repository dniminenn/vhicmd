package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWaitForImageActive(t *testing.T) {
	// One failed poll, then two statuses before active.
	replies := []string{"", "queued", "saving", "saving", "active"}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v2/images/img-1" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		status := replies[calls]
		calls++
		if status == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"id":"img-1","status":%q}`, status)
	}))
	defer srv.Close()

	var progress []string
	err := WaitForImageActive(srv.URL, "tok", "img-1", time.Minute, time.Millisecond, func(m string) {
		progress = append(progress, m)
	})
	if err != nil {
		t.Fatalf("WaitForImageActive: %v", err)
	}
	if calls != len(replies) {
		t.Errorf("polls = %d, want %d", calls, len(replies))
	}
	if len(progress) != 4 || !strings.HasPrefix(progress[0], "Warning:") ||
		progress[1] != "Image status: queued" || progress[2] != "Image status: saving" || progress[3] != "Image status: active" {
		t.Errorf("progress = %q", progress)
	}
}

func TestWaitForImageActiveTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"img-1","status":"saving"}`))
	}))
	defer srv.Close()

	err := WaitForImageActive(srv.URL, "tok", "img-1", 20*time.Millisecond, 5*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "saving") {
		t.Fatalf("err = %v, want a timeout naming the last status", err)
	}
}

func TestWaitForImageActiveError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"img-1","status":"error"}`))
	}))
	defer srv.Close()

	err := WaitForImageActive(srv.URL, "tok", "img-1", time.Minute, time.Millisecond, nil)
	if err == nil || strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want the error status reported at once", err)
	}
}
