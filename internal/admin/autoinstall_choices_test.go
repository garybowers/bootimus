package admin

import (
	"bootimus/internal/models"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestAutoInstallChoicesUpdates(t *testing.T) {
	for _, target := range []string{"image", "client", "group"} {
		t.Run(target, func(t *testing.T) {
			h, store := newAutoInstallHandler(t)
			if err := h.autoInstallLib.Write("windows", "kiosk.xml", "<unattend/>"); err != nil {
				t.Fatal(err)
			}
			if err := store.CreateImage(&models.Image{Filename: "windows.iso", Distro: "windows"}); err != nil {
				t.Fatal(err)
			}
			mac := "aa:bb:cc:dd:ee:99"
			if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
				t.Fatal(err)
			}
			group := &models.ClientGroup{Name: "lab"}
			if err := store.CreateClientGroup(group); err != nil {
				t.Fatal(err)
			}
			update := func(value string) *httptest.ResponseRecorder {
				body := `{"name":"lab","auto_install_choices":` + value + `}`
				url := "/api/images?filename=windows.iso"
				handler := h.UpdateImage
				if target == "client" {
					url, handler = "/api/clients?mac="+mac, h.UpdateClient
				}
				if target == "group" {
					url, handler = fmt.Sprintf("/api/client-groups/update?id=%d", group.ID), h.UpdateClientGroup
				}
				rec := httptest.NewRecorder()
				handler(rec, httptest.NewRequest(http.MethodPut, url, strings.NewReader(body)))
				return rec
			}
			read := func() []string {
				if target == "client" {
					c, err := store.GetClient(mac)
					if err != nil {
						t.Fatal(err)
					}
					return c.AutoInstallChoices
				}
				if target == "group" {
					g, err := store.GetClientGroup(group.ID)
					if err != nil {
						t.Fatal(err)
					}
					return g.AutoInstallChoices
				}
				img, err := store.GetImage("windows.iso")
				if err != nil {
					t.Fatal(err)
				}
				return img.AutoInstallChoices
			}
			if rec := update(`["windows/kiosk.xml"]`); rec.Code != http.StatusOK {
				t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
			}
			if got := read(); !slices.Equal(got, []string{"windows/kiosk.xml"}) {
				t.Fatalf("saved choices = %v", got)
			}
			for _, invalid := range []string{`"windows/kiosk.xml"`, `[42]`, `["windows/missing.xml"]`, `["../kiosk.xml"]`, `["ubuntu/cloud.yaml"]`, `["windows/kiosk.xml","windows/kiosk.xml"]`} {
				if rec := update(invalid); rec.Code != http.StatusBadRequest {
					t.Errorf("accepted %s: %d %s", invalid, rec.Code, rec.Body.String())
				}
				if got := read(); !slices.Equal(got, []string{"windows/kiosk.xml"}) {
					t.Fatal("invalid update changed choices")
				}
			}
			if rec := update(`[]`); rec.Code != http.StatusOK {
				t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
			}
			if got := read(); len(got) != 0 {
				t.Fatalf("clear did not persist: %v", got)
			}
		})
	}
}

func TestAutoInstallChoicesCreateValidation(t *testing.T) {
	h, store := newAutoInstallHandler(t)
	for _, tc := range []struct {
		body    string
		handler http.HandlerFunc
	}{
		{`{"mac_address":"aa:bb:cc:dd:ee:99","auto_install_choices":["windows/missing.xml"]}`, h.CreateClient},
		{`{"name":"bad","auto_install_choices":["windows/missing.xml"]}`, h.CreateClientGroup},
	} {
		rec := httptest.NewRecorder()
		tc.handler(rec, httptest.NewRequest(http.MethodPost, "/api/", strings.NewReader(tc.body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid choices accepted: %d %s", rec.Code, rec.Body.String())
		}
	}
	clients, _ := store.ListClients()
	groups, _ := store.ListClientGroups()
	if len(clients) != 0 || len(groups) != 0 {
		t.Fatal("invalid request created a record")
	}
}
