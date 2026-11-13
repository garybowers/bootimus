package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
)

func TestNativeTFTPClientSettings(t *testing.T) {
	store := storagetest.New(t)
	h := &Handler{storage: store}
	if err := store.CreateClient(&models.Client{MACAddress: "02:00:00:00:54:01", NativeTFTPBootFile: "ec84083c/kernel7.img"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateClient(&models.Client{MACAddress: "02:00:00:00:54:02"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, method, body string
		status             int
	}{
		{"create", "POST", `{"mac_address":"02:00:00:00:54:03","native_tftp_boot_file":"another/custom.bin"}`, 201},
		{"duplicate create", "POST", `{"mac_address":"02:00:00:00:54:04","native_tftp_boot_file":"ec84083c/kernel7.img"}`, 409},
		{"invalid create", "POST", `{"mac_address":"02:00:00:00:54:05","native_tftp_boot_file":"../kernel.img"}`, 400},
		{"duplicate update", "PUT", `{"native_tftp_boot_file":"ec84083c/kernel7.img"}`, 409},
		{"invalid update", "PUT", `{"native_tftp_boot_file":"kernel7.img"}`, 400},
		{"wrong type", "PUT", `{"native_tftp_boot_file":true}`, 400},
		{"set", "PUT", `{"native_tftp_boot_file":"third/kernel.img"}`, 200},
		{"unrelated update", "PUT", `{"name":"Pi"}`, 200},
		{"clear", "PUT", `{"native_tftp_boot_file":""}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/clients?mac=02:00:00:00:54:02", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			if tc.method == http.MethodPost {
				h.CreateClient(rec, req)
			} else {
				h.UpdateClient(rec, req)
			}
			if rec.Code != tc.status {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			if tc.name == "unrelated update" || tc.name == "clear" {
				got, err := store.GetClient("02:00:00:00:54:02")
				want := "third/kernel.img"
				if tc.name == "clear" {
					want = ""
				}
				if err != nil || got.NativeTFTPBootFile != want {
					t.Fatalf("setting = %+v, %v", got, err)
				}
			}
		})
	}
}
