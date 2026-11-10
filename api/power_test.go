package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// actionServer records the last request and answers with the given status and body.
func actionServer(t *testing.T, status int, reply string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.Method, rec.Path, rec.Query, rec.Body = r.Method, r.URL.Path, r.URL.RawQuery, string(b)
		rec.Header = r.Header.Clone()
		rec.Count++
		w.WriteHeader(status)
		w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

type recordedRequest struct {
	Method, Path, Query, Body string
	Header                    http.Header
	Count                     int
}

func (r *recordedRequest) check(t *testing.T, method, path, body string) {
	t.Helper()
	if r.Method != method {
		t.Errorf("method = %q, want %q", r.Method, method)
	}
	if r.Path != path {
		t.Errorf("path = %q, want %q", r.Path, path)
	}
	if r.Body != body {
		t.Errorf("body = %s, want %s", r.Body, body)
	}
}

func TestPowerActions(t *testing.T) {
	cases := []struct {
		name string
		call func(computeURL, token, vmID string) error
		want string
	}{
		{"start", StartVM, `{"os-start":null}`},
		{"stop", RequestStopVM, `{"os-stop":{}}`},
		{"shelve", ShelveVM, `{"shelve":null}`},
		{"unshelve", UnshelveVM, `{"unshelve":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := actionServer(t, http.StatusAccepted, "")
			if err := tc.call(srv.URL, "tok", "vm-1"); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			rec.check(t, "POST", "/servers/vm-1/action", tc.want)
			if got := rec.Header.Get("X-Auth-Token"); got != "tok" {
				t.Errorf("token header = %q", got)
			}
		})
		t.Run(tc.name+" conflict", func(t *testing.T) {
			srv, _ := actionServer(t, http.StatusConflict,
				`{"conflictingRequest":{"code":409,"message":"Cannot '`+tc.name+`' instance vm-1 while it is in vm_state stopped"}}`)
			if err := tc.call(srv.URL, "tok", "vm-1"); err == nil {
				t.Fatal("expected an error for a 409 response")
			}
		})
	}
}
