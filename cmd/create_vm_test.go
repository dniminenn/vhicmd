package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jessegalley/vhicmd/api"
	"github.com/spf13/viper"
)

func TestBootVolumeType(t *testing.T) {
	t.Cleanup(func() { viper.Set("volume_type", "") })

	viper.Set("volume_type", "")
	if got := bootVolumeType(""); got != defaultVolumeType {
		t.Errorf("no flag, no setting = %q, want %q", got, defaultVolumeType)
	}
	viper.Set("volume_type", "from-config")
	if got := bootVolumeType(""); got != "from-config" {
		t.Errorf("setting = %q", got)
	}
	if got := bootVolumeType("from-flag"); got != "from-flag" {
		t.Errorf("flag over setting = %q", got)
	}
}

func TestCheckBootVolumeOptions(t *testing.T) {
	ok := []createVMOptions{
		{Image: "img", Size: 10, VolumeType: "fast"},
		{Volume: "vol"},
		{Volume: "vol", UserData: "init.sh"},
	}
	for _, o := range ok {
		if err := checkBootVolumeOptions(o); err != nil {
			t.Errorf("%+v: %v", o, err)
		}
	}
	bad := map[string]createVMOptions{
		"--image":       {Volume: "vol", Image: "img"},
		"--netboot":     {Volume: "vol", Netboot: true},
		"--size":        {Volume: "vol", Size: 10},
		"--volume-type": {Volume: "vol", VolumeType: "fast"},
	}
	for flag, o := range bad {
		if err := checkBootVolumeOptions(o); err == nil || !strings.Contains(err.Error(), flag) {
			t.Errorf("--volume with %s: %v", flag, err)
		}
	}
}

func TestCheckBootVolume(t *testing.T) {
	if err := checkBootVolume(api.VolumeDetail{ID: "v", Status: "available", Bootable: "true"}); err != nil {
		t.Error(err)
	}
	if err := checkBootVolume(api.VolumeDetail{ID: "v", Status: "in-use", Bootable: "true"}); err == nil || !strings.Contains(err.Error(), "in-use") {
		t.Errorf("in-use: %v", err)
	}
	if err := checkBootVolume(api.VolumeDetail{ID: "v", Status: "available", Bootable: "false"}); err == nil || !strings.Contains(err.Error(), "vhicmd bootable v true") {
		t.Errorf("not bootable: %v", err)
	}
}

// createVMServer fakes the calls create vm makes and records the body of POST /servers.
func createVMServer(t *testing.T, volume string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/flavors/detail":
			w.Write([]byte(`{"flavors": [{"id": "flavor-1", "name": "small"}]}`))
		case r.Method == "GET" && r.URL.Path == "/volumes/11111111-1111-1111-1111-111111111111":
			w.Write([]byte(volume))
		case r.Method == "POST" && r.URL.Path == "/servers":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &posted); err != nil {
				t.Errorf("POST /servers body is not JSON: %s", body)
			}
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"server": {"id": "vm-1"}}`))
		case r.Method == "GET" && r.URL.Path == "/servers/vm-1":
			w.Write([]byte(`{"server": {"id": "vm-1", "name": "t", "status": "ACTIVE", "flavor": {"vcpus": 1, "ram": 1, "disk": 0}}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	old := tok
	tok = api.Token{Value: "tok", Endpoints: map[string]string{
		"compute": srv.URL, "volumev3": srv.URL, "network": srv.URL, "image": srv.URL,
	}}
	oldJSON := flagJsonOutput
	flagJsonOutput = true
	t.Cleanup(func() { tok, flagJsonOutput = old, oldJSON })
	return srv, &posted
}

const testVolumeID = "11111111-1111-1111-1111-111111111111"

func TestCreateVMFromVolume(t *testing.T) {
	_, posted := createVMServer(t, `{"volume": {"id": "`+testVolumeID+`", "status": "available", "bootable": "true", "size": 20}}`)

	err := runCreateVM(createVMOptions{Name: "t", Flavor: "small", Volume: testVolumeID, Networks: "none"})
	if err != nil {
		t.Fatal(err)
	}
	server := (*posted)["server"].(map[string]any)
	if server["networks"] != "none" || server["flavorRef"] != "flavor-1" {
		t.Errorf("server = %v", server)
	}
	mappings := server["block_device_mapping_v2"].([]any)
	if len(mappings) != 1 {
		t.Fatalf("mappings = %v", mappings)
	}
	m := mappings[0].(map[string]any)
	want := map[string]any{
		"boot_index": "0", "uuid": testVolumeID, "source_type": "volume",
		"destination_type": "volume", "delete_on_termination": false,
	}
	if len(m) != len(want) {
		t.Errorf("mapping = %v, want %v", m, want)
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("mapping[%s] = %v, want %v", k, m[k], v)
		}
	}
}

func TestCreateVMFromVolumeRefusesVolumeInUse(t *testing.T) {
	_, posted := createVMServer(t, `{"volume": {"id": "`+testVolumeID+`", "status": "in-use", "bootable": "true", "size": 20}}`)

	err := runCreateVM(createVMOptions{Name: "t", Flavor: "small", Volume: testVolumeID, Networks: "none"})
	if err == nil || !strings.Contains(err.Error(), "in-use") {
		t.Errorf("err = %v", err)
	}
	if *posted != nil {
		t.Errorf("a VM was requested: %v", *posted)
	}
}

func TestCreateVMFromImageUsesVolumeType(t *testing.T) {
	_, posted := createVMServer(t, "")

	// An image UUID is used as it is, so no image lookup is needed.
	image := "22222222-2222-2222-2222-222222222222"
	err := runCreateVM(createVMOptions{Name: "t", Flavor: "small", Image: image, Size: 7, VolumeType: "fast", Networks: "none"})
	if err != nil {
		t.Fatal(err)
	}
	m := (*posted)["server"].(map[string]any)["block_device_mapping_v2"].([]any)[0].(map[string]any)
	if m["volume_type"] != "fast" || m["uuid"] != image || m["source_type"] != "image" || m["volume_size"] != 7.0 || m["delete_on_termination"] != true {
		t.Errorf("mapping = %v", m)
	}
}

func TestCreateVMFlags(t *testing.T) {
	for _, name := range []string{"volume", "volume-type"} {
		if createVMCmd.Flags().Lookup(name) == nil {
			t.Errorf("create vm has no --%s flag", name)
		}
	}
}

// The volume_type setting must be one "config set" accepts.
func TestVolumeTypeIsAConfigKey(t *testing.T) {
	for _, key := range configKeys {
		if key == "volume_type" {
			return
		}
	}
	t.Error("volume_type is not in configKeys")
}
