package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage"
	"bootimus/internal/storage/storagetest"
)

const testWorkerConfig = "version: v1alpha1\nmachine:\n  type: worker\ncluster:\n  controlPlane:\n    endpoint: https://10.0.0.5:6443\n"

func newTalosTestServer(t *testing.T) (*Server, storage.Storage) {
	t.Helper()
	store := storagetest.New(t)
	srv := &Server{config: &Config{Storage: store, ServerAddr: "10.0.0.1", HTTPPort: 8080, DataDir: t.TempDir()}}
	return srv, store
}

func setupApprovedNode(t *testing.T, store storage.Storage, mac, uuid, token string) (*models.Cluster, *models.NodeImage) {
	t.Helper()
	image := &models.NodeImage{Name: "talos v1.10.5", Provider: models.ProviderTalos, Version: "v1.10.5", ImageRef: "abc123"}
	if err := store.CreateNodeImage(image); err != nil {
		t.Fatalf("CreateNodeImage: %v", err)
	}
	if err := store.SetNodeImageDownloaded(image.ID, true); err != nil {
		t.Fatalf("SetNodeImageDownloaded: %v", err)
	}
	cluster := &models.Cluster{
		Name:               "prod",
		NodeImageID:        &image.ID,
		DefaultInstallDisk: "/dev/sda",
		WorkerConfig:       testWorkerConfig,
	}
	if err := store.CreateCluster(cluster); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := store.CreateClient(&models.Client{MACAddress: mac, Name: "worker-01"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.AssignNodeToCluster(mac, cluster.ID, models.ClusterRoleWorker, "", "", token, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("AssignNodeToCluster: %v", err)
	}
	if uuid != "" {
		if err := store.SaveHardwareInventory(&models.HardwareInventory{MACAddress: mac, UUID: uuid}); err != nil {
			t.Fatalf("SaveHardwareInventory: %v", err)
		}
	}
	return cluster, image
}

func fetchMachineConfig(srv *Server, mac, uuid, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/machineconfig?m="+mac+"&u="+uuid+"&t="+token, nil)
	rec := httptest.NewRecorder()
	srv.handleMachineConfig(rec, req)
	return rec
}

func TestMachineConfigDeniedWithoutApproval(t *testing.T) {
	srv, store := newTalosTestServer(t)

	if rec := fetchMachineConfig(srv, "aa:bb:cc:dd:ee:01", "", "token"); rec.Code != http.StatusForbidden {
		t.Errorf("unknown client: expected 403, got %d", rec.Code)
	}

	if err := store.CreateClient(&models.Client{MACAddress: "aa:bb:cc:dd:ee:01"}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if rec := fetchMachineConfig(srv, "aa:bb:cc:dd:ee:01", "", "token"); rec.Code != http.StatusForbidden {
		t.Errorf("unmanaged client: expected 403, got %d", rec.Code)
	}
}

func TestMachineConfigDeniedBadToken(t *testing.T) {
	srv, store := newTalosTestServer(t)
	mac := "aa:bb:cc:dd:ee:02"
	setupApprovedNode(t, store, mac, "uuid-1", "good-token")

	if rec := fetchMachineConfig(srv, mac, "uuid-1", "bad-token"); rec.Code != http.StatusForbidden {
		t.Errorf("bad token: expected 403, got %d", rec.Code)
	}
	if rec := fetchMachineConfig(srv, mac, "uuid-1", ""); rec.Code != http.StatusForbidden {
		t.Errorf("missing token: expected 403, got %d", rec.Code)
	}

	client, _ := store.GetClient(mac)
	if client.EnrollmentState != models.EnrollmentStateApproved {
		t.Errorf("expected state to remain approved after denied fetches, got %q", client.EnrollmentState)
	}
}

func TestMachineConfigDeniedUUIDMismatch(t *testing.T) {
	srv, store := newTalosTestServer(t)
	mac := "aa:bb:cc:dd:ee:03"
	setupApprovedNode(t, store, mac, "uuid-real", "good-token")

	if rec := fetchMachineConfig(srv, mac, "uuid-spoofed", "good-token"); rec.Code != http.StatusForbidden {
		t.Errorf("uuid mismatch: expected 403, got %d", rec.Code)
	}
}

func TestMachineConfigServesAndTransitionsState(t *testing.T) {
	srv, store := newTalosTestServer(t)
	mac := "aa:bb:cc:dd:ee:04"
	setupApprovedNode(t, store, mac, "uuid-1", "good-token")

	rec := fetchMachineConfig(srv, mac, "uuid-1", "good-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "hostname: worker-01") {
		t.Errorf("expected rendered hostname in config:\n%s", body)
	}
	if !strings.Contains(body, "endpoint: https://10.0.0.5:6443") {
		t.Errorf("expected endpoint from uploaded config in output:\n%s", body)
	}
	if !strings.Contains(body, "disk: /dev/sda") {
		t.Errorf("expected default install disk in config:\n%s", body)
	}

	client, _ := store.GetClient(mac)
	if client.EnrollmentState != models.EnrollmentStateInstalling {
		t.Errorf("expected state installing after first serve, got %q", client.EnrollmentState)
	}

	if rec := fetchMachineConfig(srv, mac, "uuid-1", "good-token"); rec.Code != http.StatusOK {
		t.Errorf("expected re-fetch to succeed while installing, got %d", rec.Code)
	}
}

func TestMachineConfigNoUUIDInventorySkipsBinding(t *testing.T) {
	srv, store := newTalosTestServer(t)
	mac := "aa:bb:cc:dd:ee:05"
	setupApprovedNode(t, store, mac, "", "good-token")

	if rec := fetchMachineConfig(srv, mac, "anything", "good-token"); rec.Code != http.StatusOK {
		t.Errorf("expected 200 when no inventory uuid recorded, got %d", rec.Code)
	}
}

func TestTalosInstallScriptFor(t *testing.T) {
	srv, store := newTalosTestServer(t)
	mac := "aa:bb:cc:dd:ee:06"
	_, image := setupApprovedNode(t, store, mac, "", "good-token")

	client, _ := store.GetClient(mac)
	script, ok := srv.provisionInstallScriptFor(client)
	if !ok {
		t.Fatal("expected install script for approved node with assets")
	}
	for _, want := range []string{
		"#!ipxe",
		"kernel http://10.0.0.1:8080/node-images/talos/v1.10.5/abc123/vmlinuz",
		"talos.platform=metal",
		"talos.config=http://10.0.0.1:8080/machineconfig?m=${net0/mac}&u=${uuid}&t=good-token",
		"initrd http://10.0.0.1:8080/node-images/talos/v1.10.5/abc123/initramfs.xz",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("expected script to contain %q:\n%s", want, script)
		}
	}

	if err := store.SetNodeImageDownloaded(image.ID, false); err != nil {
		t.Fatalf("SetNodeImageDownloaded: %v", err)
	}
	if _, ok := srv.provisionInstallScriptFor(client); ok {
		t.Error("expected no install script when assets are missing")
	}

	if err := store.SetNodeImageDownloaded(image.ID, true); err != nil {
		t.Fatalf("SetNodeImageDownloaded: %v", err)
	}
	client.EnrollmentToken = ""
	if _, ok := srv.provisionInstallScriptFor(client); ok {
		t.Error("expected no install script without a token")
	}
}

func TestClusterNodeInstallBootIsRecordedOnceServed(t *testing.T) {
	srv, store := newTalosTestServer(t)
	mac := "aa:bb:cc:dd:ee:07"
	setupApprovedNode(t, store, mac, "", "good-token")

	if preview := srv.renderBootMenuWithAccess(mac, "192.0.2.7:4011", false, bootMenuAccess{}); preview.Kind != "install" {
		t.Fatalf("expected the preview to show the node install boot, got %q", preview.Kind)
	}
	if logs, err := store.GetBootLogsByMAC(mac, 10); err != nil || len(logs) != 0 {
		t.Fatalf("a preview must not record a boot, got %+v (%v)", logs, err)
	}

	if served := srv.renderBootMenuWithAccess(mac, "192.0.2.7:4011", true, bootMenuAccess{}); served.Kind != "install" {
		t.Fatalf("expected the node install boot to be served, got %q", served.Kind)
	}
	logs, err := store.GetBootLogsByMAC(mac, 10)
	if err != nil || len(logs) != 1 || logs[0].ImageName != "Cluster node install" {
		t.Fatalf("expected the node install boot in the history, got %+v (%v)", logs, err)
	}
}
