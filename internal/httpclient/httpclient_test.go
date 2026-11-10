package httpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestRequestTimeout(t *testing.T) {
	defer viper.Reset()

	viper.Reset()
	if got := requestTimeout(); got != defaultRequestTimeout {
		t.Errorf("default = %v, want %v", got, defaultRequestTimeout)
	}

	viper.Set("timeout", 300)
	if got := requestTimeout(); got != 300*time.Second {
		t.Errorf("configured = %v, want 5m", got)
	}

	viper.Set("timeout", "45")
	if got := requestTimeout(); got != 45*time.Second {
		t.Errorf("string value = %v, want 45s", got)
	}
}

// captureStdout returns what fn prints to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestDebugOutputMasksCredentials(t *testing.T) {
	defer viper.Reset()
	viper.Reset()
	viper.Set("debug", true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Subject-Token", "subject-token-value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":{"id":"body-token-value","expires_at":"2030-01-01T00:00:00Z","user":{"id":"user-1","name":"alice"}}}`))
	}))
	defer srv.Close()

	authBody := `{"auth":{"identity":{"methods":["password","application_credential"],` +
		`"password":{"user":{"name":"alice","password":"hunter2-value"}},` +
		`"application_credential":{"id":"cred-1","secret":"appcred-secret-value"}}}}`

	out := captureStdout(t, func() {
		resp, err := SendRequest(srv.URL, []byte(authBody))
		if err != nil {
			t.Errorf("SendRequest: %v", err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(body), "body-token-value") {
			t.Errorf("the caller must still get the unmasked response body, got %s", body)
		}
		if got := resp.Header.Get("X-Subject-Token"); got != "subject-token-value" {
			t.Errorf("the caller must still get the token header, got %q", got)
		}

		resp, err = SendRequestWithToken("POST", srv.URL, "auth-token-value", strings.NewReader(`{"server":{"adminPass":"admin-pass-value","name":"vm"}}`))
		if err != nil {
			t.Errorf("SendRequestWithToken: %v", err)
			return
		}
		resp.Body.Close()
	})

	for _, secret := range []string{
		"subject-token-value", "body-token-value", "hunter2-value",
		"appcred-secret-value", "auth-token-value", "admin-pass-value",
	} {
		if strings.Contains(out, secret) {
			t.Errorf("debug output leaks %q", secret)
		}
	}
	for _, kept := range []string{"X-Auth-Token", "X-Subject-Token", redacted, "alice", "cred-1", "user-1", `"name": "vm"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("debug output should still contain %q", kept)
		}
	}
}

func TestPrettyPrintJSONRedaction(t *testing.T) {
	// "password" holding an object is the keystone method block, not a secret.
	got := prettyPrintJSON([]byte(`{"password":{"user":{"password":"p"}},"size":10737418240,"items":[{"auth_key":"k"}],"token":{"id":"t"}}`))
	for _, secret := range []string{`"p"`, `"k"`, `"t"`} {
		if strings.Contains(got, secret) {
			t.Errorf("output leaks %s:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, `"user"`) || !strings.Contains(got, "10737418240") {
		t.Errorf("structure or numbers were altered:\n%s", got)
	}
	if raw := prettyPrintJSON([]byte("not json")); raw != "not json" {
		t.Errorf("non-JSON body = %q", raw)
	}
}
