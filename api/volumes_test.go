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
