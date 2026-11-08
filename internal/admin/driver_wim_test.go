package admin

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bootimus/internal/smb"
)

func testWimCommand(t *testing.T, arguments ...string) string {
	t.Helper()
	output, err := exec.Command("wimlib-imagex", arguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("wimlib-imagex %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func testDriverWim(t *testing.T, h *Handler) string {
	t.Helper()
	if _, err := exec.LookPath("wimlib-imagex"); err != nil {
		t.Skip("wimtools required for real WIM integration")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Windows/System32"), 0755)
	os.WriteFile(filepath.Join(root, "Windows/System32/startnet.cmd"), []byte("original startup\r\n"), 0644)
	filename := filepath.Join(h.isoDir, "windows/iso/sources/boot.wim")
	os.MkdirAll(filepath.Dir(filename), 0755)
	testWimCommand(t, "capture", root, filename, "WinPE", "--compress=none", "--quiet")
	testWimCommand(t, "append", root, filename, "Setup", "--boot", "--quiet")
	return filename
}

func TestDriverWIMRoundTrip(t *testing.T) {
	h, image := newDriverHandler(t)
	filename := testDriverWim(t, h)
	if err := os.Chmod(filename, 0640); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(filename)
	for _, payload := range []string{"network", "storage"} {
		response := uploadTestDriver(t, h, image, "drivers.zip", driverZIP(t, driverZipEntry{"amd64/device.inf", "INF", 0}, driverZipEntry{"amd64/device.sys", payload, 0}))
		if response.Code != http.StatusOK {
			t.Fatal(response.Body.String())
		}
	}
	if err := h.RebuildBootWim(image.ID); err != nil {
		t.Fatal(err)
	}
	packs, _ := h.storage.ListDriverPacksByImage(image.ID)
	for index := 1; index <= 2; index++ {
		listing := testWimCommand(t, "dir", filename, fmt.Sprint(index), "--path=/drivers")
		for _, pack := range packs {
			if !strings.Contains(listing, fmt.Sprintf("/drivers/pack-%d/amd64/device.sys", pack.ID)) {
				t.Fatal("pack hierarchy lost", listing)
			}
		}
	}
	if state := readDriverState(t, h, image.ID); state.Status != "applied" {
		t.Fatal(state)
	}
	if info, err := os.Stat(filename); err != nil || info.Mode().Perm() != 0640 {
		t.Fatal("WIM permissions changed", info, err)
	}
	// A failure after one image has already been updated must not replace the
	// active WIM. The wrapper delegates all other operations to real wimlib.
	t.Run("failed-second-index", func(t *testing.T) {
		before, _ := os.ReadFile(filename)
		realTool, _ := exec.LookPath("wimlib-imagex")
		tools := t.TempDir()
		script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = update ] && [ \"$3\" = 2 ]; then echo 'test write failure' >&2; exit 1; fi\nexec %q \"$@\"\n", realTool)
		os.WriteFile(filepath.Join(tools, "wimlib-imagex"), []byte(script), 0755)
		t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
		if err := h.RebuildBootWim(image.ID); err == nil {
			t.Fatal("expected update failure")
		}
		after, _ := os.ReadFile(filename)
		if sha256.Sum256(before) != sha256.Sum256(after) {
			t.Fatal("failed rebuild changed the active WIM")
		}
		if state := readDriverState(t, h, image.ID); state.Status != "failed" || state.Error == "" {
			t.Fatal(state)
		}
	})
	for _, pack := range packs {
		response := httptest.NewRecorder()
		h.DeleteDriverPack(response, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/drivers/delete?id=%d", pack.ID), nil))
		if response.Code != http.StatusOK {
			t.Fatal(response.Body.String())
		}
	}
	if err := h.RebuildBootWim(image.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(original, after) {
		t.Fatal("removing the last pack did not restore the pristine WIM")
	}
	if state := readDriverState(t, h, image.ID); state.Status != "applied" {
		t.Fatal(state)
	}
}

func TestDriverWIMPreservesSMBStartup(t *testing.T) {
	h, image := newDriverHandler(t)
	filename := testDriverWim(t, h)
	h.smbManager = smb.NewManager(h.dataDir, "127.0.0.1", "", 1445)
	if !h.patchWindowsBootWim(image.Filename) {
		t.Fatal("cannot stage original SMB startup")
	}
	image.SMBInstallEnabled = true
	if err := h.storage.UpdateImage(image.Filename, image); err != nil {
		t.Fatal(err)
	}
	response := uploadTestDriver(t, h, image, "drivers.zip", driverZIP(t, driverZipEntry{"nic.inf", "INF", 0}))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	for _, remove := range []bool{false, true} {
		if remove {
			packs, _ := h.storage.ListDriverPacksByImage(image.ID)
			for _, pack := range packs {
				if err := h.storage.DeleteDriverPack(pack.ID); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := h.RebuildBootWim(image.ID); err != nil {
			t.Fatal(err)
		}
		output := testWimCommand(t, "extract", filename, "2", "/Windows/System32/startnet.cmd", "--to-stdout")
		if !strings.Contains(output, "PnPUnattend") || !strings.Contains(output, "/installdrivers x:\\drivers") || !strings.Contains(output, "192.0.2.1") {
			t.Fatal("SMB/driver startup lost", output)
		}
		current, _ := h.storage.GetImage(image.Filename)
		if !current.SMBInstallEnabled {
			t.Fatal("driver rebuild disabled SMB")
		}
	}
}

func TestDriverWIMUnpatchKeepsOriginalForDrivers(t *testing.T) {
	h, image := newDriverHandler(t)
	filename := testDriverWim(t, h)
	h.smbManager = smb.NewManager(h.dataDir, "127.0.0.1", "", 1445)
	if !h.patchWindowsBootWim(image.Filename) {
		t.Fatal("cannot patch SMB startup")
	}
	image.SMBInstallEnabled = true
	h.storage.UpdateImage(image.Filename, image)
	response := uploadTestDriver(t, h, image, "drivers.zip", driverZIP(t, driverZipEntry{"nic.inf", "INF", 0}))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if err := h.RebuildBootWim(image.ID); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	h.UnpatchImageSMB(response, httptest.NewRequest(http.MethodPost, "/api/images/unpatch-smb?filename="+image.Filename, nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		output, err := exec.Command("wimlib-imagex", "dir", filename, "2", "--path=/drivers").CombinedOutput()
		if err == nil && strings.Contains(string(output), "nic.inf") && !h.driverBuildRunning(image.ID) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("unpatch did not reapply drivers", readDriverState(t, h, image.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}
	startup := testWimCommand(t, "extract", filename, "2", "/Windows/System32/startnet.cmd", "--to-stdout")
	if startup != "original startup\r\n" {
		t.Fatal("SMB startup remained after unpatch", startup)
	}
	if !h.driverBackupAvailable(image) {
		t.Fatal("unpatch removed original backup")
	}
}
