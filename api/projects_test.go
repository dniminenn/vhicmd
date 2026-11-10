package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestGetProjectIDByName(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK, `{"projects":[{"id":"p1","name":"prod","domain_id":"d1","enabled":true}]}`)
	id, err := GetProjectIDByName(srv.URL, "tok", "prod")
	if err != nil || id != "p1" {
		t.Errorf("prod = %q, %v", id, err)
	}
	rec.check(t, "GET", "/projects", "")
	if rec.Query != "name=prod" {
		t.Errorf("query = %q", rec.Query)
	}

	// The same name in two domains must not silently pick the first.
	srv, _ = actionServer(t, http.StatusOK, `{"projects":[{"id":"p1","name":"prod","domain_id":"d1"},{"id":"p2","name":"prod","domain_id":"d2"}]}`)
	if _, err := GetProjectIDByName(srv.URL, "tok", "prod"); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Errorf("err = %v, want a multiple-match error", err)
	}

	srv, _ = actionServer(t, http.StatusOK, `{"projects":[]}`)
	if _, err := GetProjectIDByName(srv.URL, "tok", "prod"); err == nil {
		t.Error("expected an error for no match")
	}

	srv, _ = actionServer(t, http.StatusForbidden, `{"error":{"code":403,"message":"You are not authorized","title":"Forbidden"}}`)
	if _, err := GetProjectIDByName(srv.URL, "tok", "prod"); err == nil {
		t.Error("expected an error for a 403 response")
	}
}
