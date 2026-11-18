package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListFlavorsHasSizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/flavors/detail" {
			t.Errorf("path = %s, want /flavors/detail", r.URL.Path)
		}
		w.Write([]byte(`{"flavors": [
			{"id": "f1", "name": "small", "vcpus": 2, "ram": 2048, "disk": 0, "swap": "",
			 "OS-FLV-EXT-DATA:ephemeral": 0, "os-flavor-access:is_public": true, "rxtx_factor": 1.0},
			{"id": "f2", "name": "swapped", "vcpus": 4, "ram": 12288, "disk": 40, "swap": 512,
			 "extra_specs": {"hw:cpu_policy": "shared"}}
		]}`))
	}))
	defer srv.Close()

	resp, err := ListFlavors(srv.URL, "tok", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Flavors) != 2 {
		t.Fatalf("flavors = %+v", resp.Flavors)
	}
	small, swapped := resp.Flavors[0], resp.Flavors[1]
	if small.VCPUs != 2 || small.RAM != 2048 || small.Disk != 0 || small.Swap != 0 || !small.IsPublic {
		t.Errorf("small = %+v", small)
	}
	if swapped.VCPUs != 4 || swapped.RAM != 12288 || swapped.Disk != 40 || swapped.Swap != 512 ||
		swapped.ExtraSpecs["hw:cpu_policy"] != "shared" {
		t.Errorf("swapped = %+v", swapped)
	}

	// --json prints this struct: the sizes must be in it, and swap must be a number.
	out, _ := json.Marshal(small)
	var back map[string]any
	json.Unmarshal(out, &back)
	if back["vcpus"] != 2.0 || back["ram"] != 2048.0 || back["swap"] != 0.0 {
		t.Errorf("json = %s", out)
	}
}

func TestGetFlavorDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/flavors/f1" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"itemNotFound": {"message": "Flavor nope could not be found.", "code": 404}}`))
			return
		}
		w.Write([]byte(`{"flavor": {"id": "f1", "name": "small", "vcpus": 2, "ram": 2048, "disk": 10, "swap": "1024"}}`))
	}))
	defer srv.Close()

	f, err := GetFlavorDetails(srv.URL, "tok", "f1")
	if err != nil || f.Name != "small" || f.VCPUs != 2 || f.RAM != 2048 || f.Disk != 10 || f.Swap != 1024 {
		t.Errorf("flavor = %+v, %v", f, err)
	}
	if _, err := GetFlavorDetails(srv.URL, "tok", "nope"); err == nil {
		t.Error("expected an error for an unknown flavor")
	}
}

func TestFlavorSwapRejectsOtherTypes(t *testing.T) {
	var f Flavor
	if err := json.Unmarshal([]byte(`{"swap": "lots"}`), &f); err == nil {
		t.Error("expected an error for a non-numeric swap")
	}
	if err := json.Unmarshal([]byte(`{"swap": true}`), &f); err == nil {
		t.Error("expected an error for a boolean swap")
	}
}
