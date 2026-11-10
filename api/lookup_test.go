package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

type named struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// pagedServer serves items two per page under key, with a <key>_links next entry.
// filter picks the items for a name query value; nil means the filter is ignored.
func pagedServer(t *testing.T, path, key string, items []named, filter func(item named, name string) bool) (*httptest.Server, *[]string) {
	t.Helper()
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != path {
			t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, path)
		}
		requests = append(requests, r.URL.RawQuery)

		selected := items
		if name := r.URL.Query().Get("name"); name != "" && filter != nil {
			selected = nil
			for _, item := range items {
				if filter(item, name) {
					selected = append(selected, item)
				}
			}
		}
		start := 0
		if marker := r.URL.Query().Get("marker"); marker != "" {
			for i, item := range selected {
				if item.ID == marker {
					start = i + 1
				}
			}
		}
		end := min(start+2, len(selected))
		page := selected[start:end]
		if page == nil {
			page = []named{}
		}
		reply := map[string]any{key: page}
		if end < len(selected) {
			reply[key+"_links"] = []map[string]string{{
				"rel":  "next",
				"href": fmt.Sprintf("http://internal.invalid%s?limit=2&marker=%s", path, page[len(page)-1].ID),
			}}
		}
		json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func regexFilter(item named, name string) bool {
	matched, _ := regexp.MatchString(name, item.Name)
	return matched
}

func exactFilter(item named, name string) bool { return item.Name == name }

var lookupItems = []named{
	{"id-1", "web"}, {"id-2", "web-2"}, {"id-3", "db"}, {"id-4", "webX1"},
	{"id-5", "web.1"}, {"id-6", "dup"}, {"id-7", "dup"}, {"id-8", "last"},
}

func TestGetVMIDByName(t *testing.T) {
	const uuid = "0b2ffb14-4f2a-4f0e-9a43-0d1b6a1c2d3e"

	for _, mode := range []struct {
		name   string
		filter func(named, string) bool
	}{
		{"regex filter", regexFilter},
		{"filter ignored", nil},
		{"substring filter", func(item named, name string) bool { return strings.Contains(item.Name, name) }},
	} {
		t.Run(mode.name, func(t *testing.T) {
			srv, requests := pagedServer(t, "/servers", "servers", lookupItems, mode.filter)

			if id, err := GetVMIDByName(srv.URL, "tok", uuid); err != nil || id != uuid || len(*requests) != 0 {
				t.Errorf("uuid = %q, %v after %d requests", id, err, len(*requests))
			}
			// "web" is a prefix of other names, "web.1" would match "webX1" as a bare regex,
			// and "last" sits on the last page of the unfiltered list.
			for name, want := range map[string]string{"web": "id-1", "web.1": "id-5", "last": "id-8"} {
				if id, err := GetVMIDByName(srv.URL, "tok", name); err != nil || id != want {
					t.Errorf("%s = %q, %v, want %s", name, id, err, want)
				}
			}
			if _, err := GetVMIDByName(srv.URL, "tok", "dup"); err == nil || !strings.Contains(err.Error(), "multiple") {
				t.Errorf("dup: err = %v, want a multiple-match error", err)
			}
			if _, err := GetVMIDByName(srv.URL, "tok", "missing"); err == nil || !strings.Contains(err.Error(), "no VMs found") {
				t.Errorf("missing: err = %v, want a not-found error", err)
			}
		})
	}
}

func TestGetVMIDByNameQuery(t *testing.T) {
	srv, requests := pagedServer(t, "/servers", "servers", lookupItems, regexFilter)
	if _, err := GetVMIDByName(srv.URL, "tok", "web.1"); err != nil {
		t.Fatal(err)
	}
	// One filtered request, the name escaped and anchored, and no full listing after a hit.
	if len(*requests) != 1 || (*requests)[0] != "name=%5Eweb%5C.1%24" {
		t.Errorf("requests = %q", *requests)
	}
}

func TestGetVMIDByNameError(t *testing.T) {
	srv, rec := actionServer(t, http.StatusInternalServerError, `{"computeFault":{"code":500,"message":"boom"}}`)
	if _, err := GetVMIDByName(srv.URL, "tok", "web"); err == nil {
		t.Fatal("expected an error when the list fails")
	}
	if rec.Count != 2 {
		t.Errorf("requests = %d, want the filtered try and the full list", rec.Count)
	}
}

func TestGetVolumeIDByName(t *testing.T) {
	for _, mode := range []struct {
		name   string
		filter func(named, string) bool
	}{
		{"name filter", exactFilter},
		{"filter ignored", nil},
	} {
		t.Run(mode.name, func(t *testing.T) {
			srv, requests := pagedServer(t, "/volumes/detail", "volumes", lookupItems, mode.filter)
			if id, err := GetVolumeIDByName(srv.URL, "tok", "last"); err != nil || id != "id-8" {
				t.Errorf("last = %q, %v", id, err)
			}
			if (*requests)[0] != "name=last" {
				t.Errorf("first request = %q", (*requests)[0])
			}
			if _, err := GetVolumeIDByName(srv.URL, "tok", "dup"); err == nil {
				t.Error("expected an error for two volumes with one name")
			}
			if _, err := GetVolumeIDByName(srv.URL, "tok", "missing"); err == nil {
				t.Error("expected an error for no match")
			}
		})
	}
}

func TestGetVolumeIDByNameFilterRefused(t *testing.T) {
	// The filtered request is refused, the full list still finds the volume.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"badRequest":{"code":400,"message":"Invalid filters name are found in query options."}}`))
			return
		}
		w.Write([]byte(`{"volumes":[{"id":"v1","name":"data"}]}`))
	}))
	defer srv.Close()
	if id, err := GetVolumeIDByName(srv.URL, "tok", "data"); err != nil || id != "v1" {
		t.Errorf("data = %q, %v", id, err)
	}
}

func TestGetNetworkIDByName(t *testing.T) {
	srv, requests := pagedServer(t, "/v2.0/networks", "networks", lookupItems, exactFilter)
	if id, err := GetNetworkIDByName(srv.URL, "tok", "last"); err != nil || id != "id-8" {
		t.Errorf("last = %q, %v", id, err)
	}
	if len(*requests) != 1 || (*requests)[0] != "name=last" {
		t.Errorf("requests = %q", *requests)
	}
	if _, err := GetNetworkIDByName(srv.URL, "tok", "dup"); err == nil {
		t.Error("expected an error for two networks with one name")
	}
	if _, err := GetNetworkIDByName(srv.URL, "tok", "missing"); err == nil {
		t.Error("expected an error for no match")
	}
}

func TestGetFlavorIDByNameFollowsPages(t *testing.T) {
	srv, requests := pagedServer(t, "/flavors", "flavors", lookupItems, nil)
	if id, err := GetFlavorIDByName(srv.URL, "tok", "last"); err != nil || id != "id-8" {
		t.Errorf("last = %q, %v", id, err)
	}
	if len(*requests) != 4 || (*requests)[3] != "marker=id-6" {
		t.Errorf("requests = %q", *requests)
	}
}

func TestListWithLimitIsOnePage(t *testing.T) {
	srv, requests := pagedServer(t, "/flavors", "flavors", lookupItems, nil)
	resp, err := ListFlavors(srv.URL, "tok", map[string]string{"limit": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Flavors) != 2 || len(*requests) != 1 {
		t.Errorf("flavors = %d after %d requests, want one page", len(resp.Flavors), len(*requests))
	}
}
