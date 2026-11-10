package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
