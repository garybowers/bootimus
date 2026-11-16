package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bootimus/internal/admin"
	"bootimus/internal/autoinstall"
	"bootimus/internal/models"
	"bootimus/internal/smb"
)

func TestWindowsAnswerSurvivesSMBRepatch(t *testing.T) {
	if _, err := exec.LookPath("wimlib-imagex"); err != nil {
		t.Skip("wimtools required for the SMB re-patch integration test")
	}
	srv, store := newInstallEnv(t)
	lib, err := autoinstall.New(srv.config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	srv.autoInstallLib = lib
	isoDir := t.TempDir()
	share := filepath.Join(isoDir, "win11/iso")
	wimPath := filepath.Join(share, "sources/boot.wim")
	root := t.TempDir()
	for _, dir := range []string{filepath.Dir(wimPath), filepath.Join(root, "Windows/System32")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "Windows/System32/startnet.cmd"), []byte("wpeinit\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"capture", root, wimPath, "WinPE", "--compress=none", "--quiet"}, {"append", root, wimPath, "Setup", "--boot", "--quiet"}} {
		if out, err := exec.Command("wimlib-imagex", args...).CombinedOutput(); err != nil {
			t.Fatalf("create WIM: %v: %s", err, out)
		}
	}
	image := &models.Image{Filename: "win11.iso", Name: "Windows", Distro: "windows", BootMethod: "kernel", Enabled: true, Public: true, Extracted: true}
	if err := store.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	client, _ := store.GetClient(installTestMAC)
	client.Variables = models.StringMap{"role": "desktop"}
	if err := store.UpdateClient(client.MACAddress, client); err != nil {
		t.Fatal(err)
	}
	second := &models.Client{MACAddress: "aa:bb:cc:dd:ee:11", Name: "bench-2", Enabled: true, Variables: models.StringMap{"role": "kiosk"}}
	if err := store.CreateClient(second); err != nil {
		t.Fatal(err)
	}
	smbManager := smb.NewManager(srv.config.DataDir, "127.0.0.1", "", 1445)
	h := admin.NewHandler(store, srv.config.DataDir, isoDir, "", "test", nil, nil, "", nil, false, 8080, "10.0.0.1", 1445, smbManager, true, lib)
	request := func(handler http.HandlerFunc, method, url string, body any) {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest(method, url, strings.NewReader(string(encoded))))
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, url, response.Code, response.Body.String())
		}
	}
	// An old patch may have left a shared, unrendered template behind.
	if err := os.WriteFile(filepath.Join(share, "AutoUnattend.xml"), []byte("<unattend>{{HOSTNAME}} stale</unattend>"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []string{"first", "edited"} {
		content := fmt.Sprintf("<unattend><ComputerName>{{HOSTNAME}}</ComputerName><Role>{{VAR.role}}</Role><Revision>%s</Revision></unattend>", revision)
		request(h.SaveAutoInstallFile, http.MethodPost, "/api/autoinstall-files/save", map[string]string{"distro": "windows", "filename": "answer.xml", "content": content})
		request(h.UpdateImage, http.MethodPut, "/api/images?filename=win11.iso", map[string]string{"auto_install_file": "windows/answer.xml"})
		request(h.PatchImageSMB, http.MethodPost, "/api/images/patch-smb?filename=win11.iso", nil)
		saved, err := store.GetImage(image.Filename)
		if err != nil {
			t.Fatal(err)
		}
		for _, current := range []*models.Client{client, second} {
			menu := srv.generateIPXEMenuWithAccess([]models.Image{*saved}, current.MACAddress, 0, 0, bootMenuAccess{})
			answerURL := "http://10.0.0.1:8080/autoinstall/win11.iso?mac=" + current.MACAddress
			if !strings.Contains(menu, "initrd --name AutoUnattend.xml "+answerURL+" AutoUnattend.xml || goto failed") {
				t.Fatalf("rendered answer disappeared after %s re-patch:\n%s", revision, menu)
			}
			response := httptest.NewRecorder()
			srv.handleAutoInstallScript(response, httptest.NewRequest(http.MethodGet, answerURL, nil))
			want := fmt.Sprintf("<unattend><ComputerName>%s</ComputerName><Role>%s</Role><Revision>%s</Revision></unattend>", current.Name, current.Variables["role"], revision)
			if response.Code != http.StatusOK || response.Body.String() != want {
				t.Fatalf("rendered answer after %s re-patch: %d %q, want %q", revision, response.Code, response.Body.String(), want)
			}
		}
		if _, err := os.Stat(filepath.Join(share, "AutoUnattend.xml")); !os.IsNotExist(err) {
			t.Fatalf("template left on the shared install media: %v", err)
		}
	}
}
