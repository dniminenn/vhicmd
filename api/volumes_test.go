package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateVolumeFromVolume(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"volume":{"id":"new","size":27}}`))
	}))
	defer srv.Close()

	var request CreateVolumeRequest
	request.Volume.Name = "copy"
	request.Volume.Size = 27
	request.Volume.SourceVolID = "src"
	resp, err := CreateVolume(srv.URL, "tok", request)
	if err != nil || resp.Volume.ID != "new" {
		t.Fatalf("CreateVolume = %+v, %v", resp, err)
	}
	if want := `{"volume":{"name":"copy","size":27,"source_volid":"src"}}`; body != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

func TestListVolumeTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/types" {
			t.Errorf("request = %s %s, want GET /types", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"volume_types": [
			{"id": "t1", "name": "default", "description": "Default Volume Type", "is_public": true},
			{"id": "t2", "name": "fast", "description": null, "is_public": false}
		]}`))
	}))
	defer srv.Close()

	types, err := ListVolumeTypes(srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 || types[0].Name != "default" || !types[0].IsPublic || types[0].Description != "Default Volume Type" ||
		types[1].Name != "fast" || types[1].IsPublic || types[1].Description != "" {
		t.Errorf("types = %+v", types)
	}
}

func TestListVolumeTypesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"forbidden": {"message": "Policy doesn't allow it.", "code": 403}}`))
	}))
	defer srv.Close()

	if _, err := ListVolumeTypes(srv.URL, "tok"); err == nil {
		t.Error("expected an error")
	}
}

func TestCreateVolumeFromImageUsesTheGivenType(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"volume":{"id":"new","size":3}}`))
	}))
	defer srv.Close()

	if _, err := CreateVolumeFromImage(srv.URL, "tok", "img", "boot", 3, "fast"); err != nil {
		t.Fatal(err)
	}
	if want := `{"volume":{"name":"boot","size":3,"imageRef":"img","volume_type":"fast"}}`; body != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}
