package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListSnapshotsFollowsPages(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/snapshots/detail" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("marker") == "" {
			// The href points at another host on purpose; only its marker may be used.
			fmt.Fprint(w, `{"snapshots":[{"id":"s1","name":"a","volume_id":"v1","size":10,"status":"available"}],`+
				`"snapshots_links":[{"rel":"next","href":"http://internal.invalid/v3/p/snapshots/detail?marker=s1"}]}`)
			return
		}
		fmt.Fprint(w, `{"snapshots":[{"id":"s2","name":"b","volume_id":"v1","size":10,"status":"creating"}]}`)
	}))
	defer srv.Close()

	resp, err := ListSnapshots(srv.URL, "tok", map[string]string{"volume_id": "v1"})
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(resp.Snapshots) != 2 || resp.Snapshots[0].ID != "s1" || resp.Snapshots[1].ID != "s2" {
		t.Errorf("snapshots = %+v", resp.Snapshots)
	}
	if len(queries) != 2 || queries[0] != "volume_id=v1" || queries[1] != "marker=s1&volume_id=v1" {
		t.Errorf("queries = %q", queries)
	}
}

func TestListSnapshotsError(t *testing.T) {
	srv, _ := actionServer(t, http.StatusForbidden, `{"forbidden":{"code":403,"message":"Policy doesn't allow volume:get_all_snapshots"}}`)
	if _, err := ListSnapshots(srv.URL, "tok", nil); err == nil {
		t.Fatal("expected an error for a 403 response")
	}
}

func TestGetSnapshotDetails(t *testing.T) {
	srv, rec := actionServer(t, http.StatusOK,
		`{"snapshot":{"id":"s1","name":"nightly","status":"available","size":32,"volume_id":"v1",`+
			`"os-extended-snapshot-attributes:progress":"100%","metadata":{"k":"v"}}}`)
	s, err := GetSnapshotDetails(srv.URL, "tok", "s1")
	if err != nil {
		t.Fatalf("GetSnapshotDetails: %v", err)
	}
	rec.check(t, "GET", "/snapshots/s1", "")
	if s.Name != "nightly" || s.Size != 32 || s.VolumeID != "v1" || s.Progress != "100%" || s.Metadata["k"] != "v" {
		t.Errorf("snapshot = %+v", s)
	}

	srv, _ = actionServer(t, http.StatusNotFound, `{"itemNotFound":{"code":404,"message":"Snapshot s1 could not be found."}}`)
	if _, err := GetSnapshotDetails(srv.URL, "tok", "s1"); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestCreateSnapshot(t *testing.T) {
	cases := []struct {
		name  string
		force bool
		want  string
	}{
		{"plain", false, `{"snapshot":{"volume_id":"v1","name":"snap","description":"d"}}`},
		{"force", true, `{"snapshot":{"volume_id":"v1","name":"snap","description":"d","force":true}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := actionServer(t, http.StatusAccepted, `{"snapshot":{"id":"s1","name":"snap","status":"creating","volume_id":"v1"}}`)
			var req CreateSnapshotRequest
			req.Snapshot.VolumeID = "v1"
			req.Snapshot.Name = "snap"
			req.Snapshot.Description = "d"
			req.Snapshot.Force = tc.force
			s, err := CreateSnapshot(srv.URL, "tok", req)
			if err != nil {
				t.Fatalf("CreateSnapshot: %v", err)
			}
			rec.check(t, "POST", "/snapshots", tc.want)
			if s.ID != "s1" || s.Status != "creating" {
				t.Errorf("snapshot = %+v", s)
			}
		})
	}

	srv, _ := actionServer(t, http.StatusBadRequest, `{"badRequest":{"code":400,"message":"Invalid volume: Volume v1 status must be available, but current status is: in-use."}}`)
	var req CreateSnapshotRequest
	req.Snapshot.VolumeID = "v1"
	_, err := CreateSnapshot(srv.URL, "tok", req)
	if err == nil || !strings.Contains(err.Error(), "in-use") {
		t.Fatalf("err = %v, want the API message", err)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	srv, rec := actionServer(t, http.StatusAccepted, "")
	if err := DeleteSnapshot(srv.URL, "tok", "s1"); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	rec.check(t, "DELETE", "/snapshots/s1", "")

	srv, _ = actionServer(t, http.StatusNotFound, `{"itemNotFound":{"code":404,"message":"Snapshot s1 could not be found."}}`)
	if err := DeleteSnapshot(srv.URL, "tok", "s1"); err == nil {
		t.Fatal("expected an error for a 404 response")
	}
}

func TestGetSnapshotIDByName(t *testing.T) {
	const uuid = "0b2ffb14-4f2a-4f0e-9a43-0d1b6a1c2d3e"
	srv, rec := actionServer(t, http.StatusOK, `{"snapshots":[{"id":"s1","name":"nightly"},{"id":"s2","name":"nightly-old"},{"id":"s3","name":"dup"},{"id":"s4","name":"dup"}]}`)

	if id, err := GetSnapshotIDByName(srv.URL, "tok", uuid); err != nil || id != uuid || rec.Count != 0 {
		t.Errorf("uuid lookup = %q, %v after %d requests", id, err, rec.Count)
	}
	id, err := GetSnapshotIDByName(srv.URL, "tok", "nightly")
	if err != nil || id != "s1" {
		t.Errorf("name lookup = %q, %v", id, err)
	}
	if rec.Path != "/snapshots/detail" || rec.Query != "name=nightly" {
		t.Errorf("request = %s?%s", rec.Path, rec.Query)
	}
	if _, err := GetSnapshotIDByName(srv.URL, "tok", "dup"); err == nil {
		t.Error("expected an error for two snapshots with one name")
	}
	if _, err := GetSnapshotIDByName(srv.URL, "tok", "missing"); err == nil {
		t.Error("expected an error for no match")
	}
}
