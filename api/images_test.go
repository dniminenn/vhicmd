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

func TestImageLookupsByIDFetchTheImage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/v2/images/img-1" {
			t.Errorf("request = %s %s, want GET /v2/images/img-1", r.Method, r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"id":"img-1","name":"debian-12","status":"active","size":2147483648,"min_disk":10,"min_ram":512,"owner":"p1","visibility":"private"}`))
	}))
	defer srv.Close()

	image, err := GetImageByID(srv.URL, "tok", "img-1")
	if err != nil {
		t.Fatalf("GetImageByID: %v", err)
	}
	want := Image{ID: "img-1", Name: "debian-12", Status: "active", Size: 2147483648, MinDisk: 10, MinRAM: 512, Owner: "p1", Visibility: "private"}
	if image != want {
		t.Errorf("image = %+v, want %+v", image, want)
	}
	if name, err := GetImageNameByID(srv.URL, "tok", "img-1"); err != nil || name != "debian-12" {
		t.Errorf("name = %q, %v", name, err)
	}
	if size, err := GetImageSize(srv.URL, "tok", "img-1"); err != nil || size != 2147483648 {
		t.Errorf("size = %d, %v", size, err)
	}
	if calls != 3 {
		t.Errorf("requests = %d, want one per lookup", calls)
	}
}

func TestGetImageByIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"No image found with ID img-9","code":404,"title":"Not Found"}`))
	}))
	defer srv.Close()

	for name, err := range map[string]error{
		"GetImageByID":     func() error { _, err := GetImageByID(srv.URL, "tok", "img-9"); return err }(),
		"GetImageNameByID": func() error { _, err := GetImageNameByID(srv.URL, "tok", "img-9"); return err }(),
		"GetImageSize":     func() error { _, err := GetImageSize(srv.URL, "tok", "img-9"); return err }(),
	} {
		if err == nil || !strings.Contains(err.Error(), "img-9") {
			t.Errorf("%s: err = %v, want an error naming the image", name, err)
		}
	}
}
