package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"bootimus/internal/models"
	"bootimus/internal/storage"
	"bootimus/internal/storage/storagetest"
)

const bootEventMAC = "aa:bb:cc:dd:ee:10"

func uintString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}

func newBootEventServer(t *testing.T) (*Server, storage.Storage) {
	t.Helper()
	store := storagetest.New(t)
	if err := store.CreateClient(&models.Client{MACAddress: bootEventMAC, Name: "lab-10", Enabled: true, ShowPublicImages: true}); err != nil {
		t.Fatal(err)
	}
	return &Server{config: &Config{Storage: store, DataDir: t.TempDir()}}, store
}

func requestBootEvent(t *testing.T, server *Server, query string) {
	t.Helper()
	response := httptest.NewRecorder()
	server.handleBootEvent(response, httptest.NewRequest(http.MethodGet, "/boot-event?"+query, nil))
	if response.Code != http.StatusOK || response.Body.String() != "#!ipxe\n" {
		t.Fatalf("%s: expected an empty iPXE script, got %d %q", query, response.Code, response.Body.String())
	}
}

func bootHistory(t *testing.T, store storage.Storage) []models.BootLog {
	t.Helper()
	logs, err := store.GetBootLogsByMAC(bootEventMAC, 50)
	if err != nil {
		t.Fatal(err)
	}
	return logs
}

func TestBootEventRecordsImageBoot(t *testing.T) {
	server, store := newBootEventServer(t)
	image := &models.Image{Name: "Debian 13", Filename: "debian-13.iso", Enabled: true, Public: true, BootMethod: "sanboot"}
	if err := store.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	query := "mac=AA-BB-CC-DD-EE-10&image=" + uintString(image.ID)

	requestBootEvent(t, server, query)
	logs := bootHistory(t, store)
	if len(logs) != 1 || logs[0].ImageName != "Debian 13" || !logs[0].Success {
		t.Fatalf("expected one successful boot of the image in the history, got %+v", logs)
	}
	if logs[0].ImageID == nil || *logs[0].ImageID != image.ID || logs[0].ClientID == nil {
		t.Errorf("boot log is not linked to the image and client: %+v", logs[0])
	}
	client, err := store.GetClient(bootEventMAC)
	if err != nil {
		t.Fatal(err)
	}
	if client.BootCount != 1 || client.LastBoot == nil {
		t.Errorf("client boot statistics were not updated: count=%d last=%v", client.BootCount, client.LastBoot)
	}

	requestBootEvent(t, server, query)
	if logs := bootHistory(t, store); len(logs) != 1 {
		t.Errorf("a repeated report within the retry window must not add a second entry, got %d", len(logs))
	}
}

func TestBootEventRecordsSANBoot(t *testing.T) {
	server, store := newBootEventServer(t)
	target := &models.SANTarget{Name: "lab", URI: "aoe:e0.0"}
	if err := store.CreateSANTarget(target); err != nil {
		t.Fatal(err)
	}

	requestBootEvent(t, server, "mac="+bootEventMAC+"&san="+uintString(target.ID))
	logs := bootHistory(t, store)
	if len(logs) != 1 || logs[0].ImageName != "SAN target lab" {
		t.Fatalf("expected the SAN boot in the history, got %+v", logs)
	}
}

func TestBootEventIgnoresWhatItCannotVerify(t *testing.T) {
	server, store := newBootEventServer(t)
	hidden := &models.Image{Name: "Hidden", Filename: "hidden.iso", Enabled: true, Public: false, BootMethod: "sanboot"}
	if err := store.CreateImage(hidden); err != nil {
		t.Fatal(err)
	}
	disabled := &models.Image{Name: "Disabled", Filename: "disabled.iso", Public: true, BootMethod: "sanboot"}
	if err := store.CreateImage(disabled); err != nil {
		t.Fatal(err)
	}
	disabled.Enabled = false
	if err := store.UpdateImage(disabled.Filename, disabled); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{
		"mac=" + bootEventMAC + "&image=" + uintString(hidden.ID),
		"mac=" + bootEventMAC + "&image=" + uintString(disabled.ID),
		"mac=" + bootEventMAC + "&image=999",
		"mac=" + bootEventMAC + "&image=abc",
		"mac=" + bootEventMAC + "&san=999",
		"mac=" + bootEventMAC + "&tool=gparted",
		"mac=" + bootEventMAC + "&diskimage=1",
		"mac=" + bootEventMAC,
		"mac=unknown&image=1",
		"mac=not-a-mac&image=1",
		"image=1",
	} {
		requestBootEvent(t, server, query)
	}
	if logs, err := store.GetBootLogs(50); err != nil || len(logs) != 0 {
		t.Fatalf("expected nothing in the boot log, got %+v (%v)", logs, err)
	}
}
