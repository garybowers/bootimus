package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage/storagetest"
)

func newDriverHandler(t *testing.T) (*Handler, *models.Image) {
	t.Helper()
	root := t.TempDir()
	handler := &Handler{storage: storagetest.New(t), dataDir: root, isoDir: filepath.Join(root, "isos"), serverAddr: "192.0.2.1"}
	image := &models.Image{Name: "Windows", Filename: "windows.iso", Distro: "windows", Extracted: true}
	if err := handler.storage.CreateImage(image); err != nil {
		t.Fatal(err)
	}
	return handler, image
}

func uploadTestDriver(t *testing.T, h *Handler, image *models.Image, name string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("imageId", fmt.Sprint(image.ID))
	file, err := form.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	file.Write(content)
	form.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/drivers/upload", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := httptest.NewRecorder()
	h.UploadDriverPack(response, request)
	return response
}

func readDriverState(t *testing.T, h *Handler, id uint) driverBuildState {
	t.Helper()
	response := httptest.NewRecorder()
	h.DriverBuildStatus(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/drivers/status?imageId=%d", id), nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body.String())
	}
	var result struct{ Data driverBuildState }
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Data
}

func TestDriverUploadRemovalAndPersistentStatus(t *testing.T) {
	handler, image := newDriverHandler(t)
	for _, payload := range []string{"first", "other"} {
		response := uploadTestDriver(t, handler, image, "same-name.zip", driverZIP(t, driverZipEntry{"nic.inf", "INF", 0}, driverZipEntry{"nic.sys", payload, 0}))
		if response.Code != http.StatusOK {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	packs, err := handler.storage.ListDriverPacksByImage(image.ID)
	if err != nil || len(packs) != 2 {
		t.Fatal(packs, err)
	}
	if packs[0].Filename == packs[1].Filename {
		t.Fatal("duplicate upload overwrote the first pack")
	}
	for _, pack := range packs {
		if _, err := os.Stat(filepath.Join(handler.isoDir, "windows/drivers", pack.Filename)); err != nil {
			t.Fatal(err)
		}
	}
	if state := readDriverState(t, handler, image.ID); state.Status != "pending" || state.CanApply || state.Reason == "" {
		t.Fatalf("unexpected state: %+v", state)
	}
	// A fresh handler represents a server restart, with the same database/files.
	restarted := &Handler{storage: handler.storage, dataDir: handler.dataDir, isoDir: handler.isoDir}
	if state := readDriverState(t, restarted, image.ID); state.Status != "pending" {
		t.Fatal(state)
	}
	for _, pack := range packs {
		response := httptest.NewRecorder()
		handler.DeleteDriverPack(response, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/drivers/delete?id=%d", pack.ID), nil))
		if response.Code != http.StatusOK {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	if state := readDriverState(t, restarted, image.ID); state.Status != "pending" {
		t.Fatal("last removal must still be applied", state)
	}
	if err := handler.saveDriverState(image.ID, driverBuildState{Status: "applying", Revision: driverRevision(nil)}); err != nil {
		t.Fatal(err)
	}
	if state := readDriverState(t, restarted, image.ID); state.Status != "failed" || state.Error == "" {
		t.Fatal("interrupted build not reported", state)
	}
}

func TestDriverUploadRejectsDuplicateContent(t *testing.T) {
	h, image := newDriverHandler(t)
	content := driverZIP(t, driverZipEntry{"nic.inf", "INF", 0}, driverZipEntry{"nic.sys", "driver", 0})
	response := uploadTestDriver(t, h, image, "nic.zip", content)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	packs, _ := h.storage.ListDriverPacksByImage(image.ID)
	if err := h.saveDriverState(image.ID, driverBuildState{Status: "applied", Revision: driverRevision(packs)}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(h.driverStatePath(image.ID))
	// A new handler has no in-memory record of the existing upload.
	restarted := &Handler{storage: h.storage, dataDir: h.dataDir, isoDir: h.isoDir}
	for _, name := range []string{"nic.zip", "renamed.zip"} {
		response := uploadTestDriver(t, restarted, image, name, content)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "already uploaded") {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	after, _ := os.ReadFile(h.driverStatePath(image.ID))
	if !bytes.Equal(before, after) {
		t.Fatal("rejected duplicate changed the build status")
	}
	packs, err := h.storage.ListDriverPacksByImage(image.ID)
	files, readErr := os.ReadDir(filepath.Join(h.isoDir, "windows/drivers"))
	if err != nil || readErr != nil || len(packs) != 1 || len(files) != 1 {
		t.Fatal("duplicate left metadata or an archive", len(packs), len(files), err, readErr)
	}
	// The same driver may be needed by another Windows image.
	other := &models.Image{Name: "Other Windows", Filename: "other.iso", Distro: "windows"}
	if err := h.storage.CreateImage(other); err != nil {
		t.Fatal(err)
	}
	if response := uploadTestDriver(t, h, other, "nic.zip", content); response.Code != http.StatusOK {
		t.Fatal("duplicate check crossed image boundaries", response.Body.String())
	}
	// Removing an unapplied pack permits uploading it again.
	response = httptest.NewRecorder()
	h.DeleteDriverPack(response, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/drivers/delete?id=%d", packs[0].ID), nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if response := uploadTestDriver(t, h, image, "nic.zip", content); response.Code != http.StatusOK {
		t.Fatal("removed pack prevented re-upload", response.Body.String())
	}
}

func TestDriverRemovalRequiresOriginalBackup(t *testing.T) {
	h, image := newDriverHandler(t)
	response := uploadTestDriver(t, h, image, "nic.zip", driverZIP(t, driverZipEntry{"nic.inf", "INF", 0}))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	packs, _ := h.storage.ListDriverPacksByImage(image.ID)
	now := time.Now()
	packs[0].LastApplied = &now
	if err := h.storage.UpdateDriverPack(packs[0].ID, packs[0]); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	h.DeleteDriverPack(response, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/drivers/delete?id=%d", packs[0].ID), nil))
	if response.Code != http.StatusPreconditionFailed || !strings.Contains(response.Body.String(), "Re-extract") {
		t.Fatal(response.Code, response.Body.String())
	}
	remaining, _ := h.storage.ListDriverPacksByImage(image.ID)
	if len(remaining) != 1 {
		t.Fatal("removed a pack that cannot be removed from the WIM")
	}
	// Re-extraction resets the application history; the pack can be removed.
	h.discardStaleBootWimBackup(image.Filename)
	response = httptest.NewRecorder()
	h.DeleteDriverPack(response, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/drivers/delete?id=%d", packs[0].ID), nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Code, response.Body.String())
	}
}

func TestDriverAsyncRebuildFailureAndConcurrency(t *testing.T) {
	h, image := newDriverHandler(t)
	toolDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(toolDir, "wimlib-imagex"), []byte("#!/bin/sh\necho 'test WIM failure' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	wimPath := filepath.Join(h.isoDir, "windows/iso/sources/boot.wim")
	os.MkdirAll(filepath.Dir(wimPath), 0755)
	os.WriteFile(wimPath, []byte("invalid WIM for failure test"), 0644)
	mu := h.bootWimLock(image.Filename)
	mu.Lock()
	request := func() *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		h.RebuildImageBootWim(response, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/drivers/rebuild?imageId=%d", image.ID), nil))
		return response
	}
	first, second := request(), request()
	state := readDriverState(t, h, image.ID)
	mu.Unlock()
	if first.Code != http.StatusAccepted || second.Code != http.StatusConflict || state.Status != "applying" || state.CanApply {
		t.Fatal(first.Code, second.Code, state)
	}
	deadline := time.Now().Add(5 * time.Second)
	for h.driverBuildRunning(image.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	state = readDriverState(t, h, image.ID)
	if state.Status != "failed" || state.Error == "" || !state.CanApply {
		t.Fatal(state)
	}
	content, err := os.ReadFile(wimPath)
	if err != nil || string(content) != "invalid WIM for failure test" {
		t.Fatal("failed async rebuild changed active WIM", err)
	}
}

func TestDriverRequestsRejectInvalidPacksAndConcurrentChanges(t *testing.T) {
	handler, image := newDriverHandler(t)
	response := uploadTestDriver(t, handler, image, "broken.zip", []byte("not a ZIP"))
	if response.Code != http.StatusBadRequest {
		t.Fatal(response.Code)
	}
	packs, _ := handler.storage.ListDriverPacksByImage(image.ID)
	if len(packs) != 0 {
		t.Fatal("invalid upload saved metadata")
	}
	files, _ := os.ReadDir(filepath.Join(handler.isoDir, "windows/drivers"))
	if len(files) != 0 {
		t.Fatal("invalid upload left an archive")
	}
	zip := driverZIP(t, driverZipEntry{"nic.inf", "INF", 0})
	response = uploadTestDriver(t, handler, image, "nic.inf", zip)
	if response.Code != http.StatusBadRequest {
		t.Fatal("standalone file accepted")
	}
	handler.reserveDriverBuild(image.ID)
	response = uploadTestDriver(t, handler, image, "nic.zip", zip)
	if response.Code != http.StatusConflict {
		t.Fatal("upload changed an active build")
	}
	handler.releaseDriverBuild(image.ID)
	mu := handler.bootWimLock(image.Filename)
	mu.Lock()
	response = uploadTestDriver(t, handler, image, "nic.zip", zip)
	mu.Unlock()
	if response.Code != http.StatusConflict {
		t.Fatal("upload raced image extraction")
	}
	image.Distro = "ubuntu"
	handler.storage.UpdateImage(image.Filename, image)
	response = uploadTestDriver(t, handler, image, "nic.zip", zip)
	if response.Code != http.StatusBadRequest {
		t.Fatal("non-Windows image accepted")
	}
}
