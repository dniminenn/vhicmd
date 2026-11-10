package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetConsoleOutput(t *testing.T) {
	cases := []struct {
		name  string
		lines int
		want  string
	}{
		{"whole log", 0, `{"os-getConsoleOutput":{}}`},
		{"last lines", 50, `{"os-getConsoleOutput":{"length":50}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				gotPath, gotBody = r.URL.Path, string(b)
				json.NewEncoder(w).Encode(map[string]string{"output": "boot ok\n"})
			}))
			defer srv.Close()

			out, err := GetConsoleOutput(srv.URL, "tok", "vm-1", tc.lines)
			if err != nil {
				t.Fatalf("GetConsoleOutput: %v", err)
			}
			if out != "boot ok\n" {
				t.Errorf("output = %q", out)
			}
			if gotPath != "/servers/vm-1/action" {
				t.Errorf("path = %q", gotPath)
			}
			if gotBody != tc.want {
				t.Errorf("body = %s, want %s", gotBody, tc.want)
			}
		})
	}
}

func TestGetConsoleOutputError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"itemNotFound":{"code":404,"message":"Instance vm-1 could not be found."}}`))
	}))
	defer srv.Close()

	if _, err := GetConsoleOutput(srv.URL, "tok", "vm-1", 0); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestGetConsoleURL(t *testing.T) {
	var gotPath, gotBody, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody, gotVersion = r.URL.Path, string(b), r.Header.Get("X-OpenStack-Nova-API-Version")
		w.Write([]byte(`{"remote_console":{"protocol":"vnc","type":"novnc","url":"https://vhi:6083/vnc_auto.html?token=abc"}}`))
	}))
	defer srv.Close()

	got, err := GetConsoleURL(srv.URL, "tok", "vm-1")
	if err != nil {
		t.Fatalf("GetConsoleURL: %v", err)
	}
	if got != "https://vhi:6083/vnc_auto.html?token=abc" {
		t.Errorf("url = %q", got)
	}
	if gotPath != "/servers/vm-1/remote-consoles" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody != `{"remote_console":{"protocol":"vnc","type":"novnc"}}` {
		t.Errorf("body = %s", gotBody)
	}
	if gotVersion != "2.72" {
		t.Errorf("microversion = %q, remote-consoles needs 2.6 or later", gotVersion)
	}
}

func TestGetConsoleURLLegacyFallback(t *testing.T) {
	var paths []string
	var legacyBody, legacyVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/servers/vm-1/remote-consoles" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"itemNotFound":{"code":404,"message":"The resource could not be found."}}`))
			return
		}
		b, _ := io.ReadAll(r.Body)
		legacyBody, legacyVersion = string(b), r.Header.Get("X-OpenStack-Nova-API-Version")
		w.Write([]byte(`{"console":{"type":"novnc","url":"https://vhi:6080/vnc_auto.html?token=old"}}`))
	}))
	defer srv.Close()

	got, err := GetConsoleURL(srv.URL, "tok", "vm-1")
	if err != nil {
		t.Fatalf("GetConsoleURL: %v", err)
	}
	if got != "https://vhi:6080/vnc_auto.html?token=old" {
		t.Errorf("url = %q", got)
	}
	if len(paths) != 2 || paths[1] != "/servers/vm-1/action" {
		t.Errorf("paths = %q", paths)
	}
	if legacyBody != `{"os-getVNCConsole":{"type":"novnc"}}` {
		t.Errorf("legacy body = %s", legacyBody)
	}
	if legacyVersion != "2.1" {
		t.Errorf("legacy microversion = %q, the action is gone from 2.6", legacyVersion)
	}
}

func TestGetConsoleURLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"conflictingRequest":{"code":409,"message":"Instance not yet ready"}}`))
	}))
	defer srv.Close()

	_, err := GetConsoleURL(srv.URL, "tok", "vm-1")
	if err == nil || !strings.Contains(err.Error(), "Instance not yet ready") {
		t.Fatalf("err = %v, want the API message", err)
	}
}
