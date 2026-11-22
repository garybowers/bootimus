package storage

import (
	"testing"
	"time"

	"bootimus/internal/models"

	"gorm.io/gorm"
)

func newTestCluster(t *testing.T, store *testStore, name string) *models.Cluster {
	t.Helper()
	cluster := &models.Cluster{Name: name, DefaultInstallDisk: "/dev/sda"}
	if err := store.CreateCluster(cluster); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	return cluster
}

func approveTestClient(t *testing.T, store *testStore, mac string, clusterID uint) {
	t.Helper()
	if err := store.CreateClient(&models.Client{MACAddress: mac}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if err := store.AssignNodeToCluster(mac, clusterID, models.ClusterRoleWorker, "/dev/nvme0n1", "", "tok-"+mac, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("AssignNodeToCluster: %v", err)
	}
}

func TestAssignNodeToCluster(t *testing.T) {
	store := newTestStore(t)
	cluster := newTestCluster(t, store, "prod")
	mac := "aa:bb:cc:00:00:01"
	approveTestClient(t, store, mac, cluster.ID)

	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client.EnrollmentState != models.EnrollmentStateApproved {
		t.Errorf("expected approved state, got %q", client.EnrollmentState)
	}
	if client.ClusterID == nil || *client.ClusterID != cluster.ID {
		t.Errorf("expected cluster id %d, got %v", cluster.ID, client.ClusterID)
	}
	if client.ClusterRole != models.ClusterRoleWorker {
		t.Errorf("expected worker role, got %q", client.ClusterRole)
	}
	if client.EnrollmentToken != "tok-"+mac {
		t.Errorf("expected token to persist, got %q", client.EnrollmentToken)
	}
	if client.TokenExpiresAt == nil {
		t.Error("expected token expiry to persist")
	}
	if client.ApprovedAt == nil {
		t.Error("expected approved timestamp")
	}
	if !client.Static {
		t.Error("expected approval to promote client to static")
	}
}

func TestAssignNodeMissingClient(t *testing.T) {
	store := newTestStore(t)
	cluster := newTestCluster(t, store, "prod")
	err := store.AssignNodeToCluster("no:such:mac:00:00:00", cluster.ID, models.ClusterRoleWorker, "", "", "tok", time.Now().Add(time.Hour))
	if err != gorm.ErrRecordNotFound {
		t.Errorf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestSetClientEnrollmentStateClearsToken(t *testing.T) {
	store := newTestStore(t)
	cluster := newTestCluster(t, store, "prod")
	mac := "aa:bb:cc:00:00:02"
	approveTestClient(t, store, mac, cluster.ID)

	if err := store.SetClientEnrollmentState(mac, models.EnrollmentStateInstalling); err != nil {
		t.Fatalf("SetClientEnrollmentState installing: %v", err)
	}
	client, _ := store.GetClient(mac)
	if client.EnrollmentToken == "" {
		t.Error("expected token to survive installing state")
	}

	if err := store.SetClientEnrollmentState(mac, models.EnrollmentStateInstalled); err != nil {
		t.Fatalf("SetClientEnrollmentState installed: %v", err)
	}
	client, _ = store.GetClient(mac)
	if client.EnrollmentState != models.EnrollmentStateInstalled {
		t.Errorf("expected installed state, got %q", client.EnrollmentState)
	}
	if client.EnrollmentToken != "" || client.TokenExpiresAt != nil {
		t.Error("expected token to clear on installed")
	}
	if client.ClusterID == nil {
		t.Error("expected cluster membership to survive installed state")
	}
}

func TestResetClientEnrollment(t *testing.T) {
	store := newTestStore(t)
	cluster := newTestCluster(t, store, "prod")
	mac := "aa:bb:cc:00:00:03"
	approveTestClient(t, store, mac, cluster.ID)

	if err := store.ResetClientEnrollment(mac); err != nil {
		t.Fatalf("ResetClientEnrollment: %v", err)
	}
	client, _ := store.GetClient(mac)
	if client.EnrollmentState != models.EnrollmentStateUnmanaged {
		t.Errorf("expected unmanaged state, got %q", client.EnrollmentState)
	}
	if client.ClusterID != nil || client.ClusterRole != "" || client.EnrollmentToken != "" || client.ApprovedAt != nil {
		t.Error("expected all enrolment fields to clear on reset")
	}
}

func TestDeleteClusterDetachesNodes(t *testing.T) {
	store := newTestStore(t)
	cluster := newTestCluster(t, store, "prod")
	mac := "aa:bb:cc:00:00:08"
	approveTestClient(t, store, mac, cluster.ID)

	if err := store.DeleteCluster(cluster.ID); err != nil {
		t.Fatalf("DeleteCluster: %v", err)
	}

	client, err := store.GetClient(mac)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client.ClusterID != nil || client.EnrollmentState != models.EnrollmentStateUnmanaged || client.EnrollmentToken != "" {
		t.Error("expected node to be fully detached after cluster delete")
	}

	if _, err := store.GetCluster(cluster.ID); err == nil {
		t.Error("expected cluster to be gone")
	}

	if err := store.CreateCluster(&models.Cluster{Name: "prod"}); err != nil {
		t.Errorf("expected cluster name to be reusable after delete: %v", err)
	}
}

func TestSetClusterConfigsPartialUpdate(t *testing.T) {
	store := newTestStore(t)
	cluster := newTestCluster(t, store, "prod")

	if err := store.SetClusterConfigs(cluster.ID, "cp-yaml", "worker-yaml", "upload"); err != nil {
		t.Fatalf("SetClusterConfigs: %v", err)
	}
	got, _ := store.GetCluster(cluster.ID)
	if got.ControlPlaneConfig != "cp-yaml" || got.WorkerConfig != "worker-yaml" {
		t.Error("expected configs to persist")
	}
	if got.ConfigSource != "upload" {
		t.Errorf("expected config source to persist, got %q", got.ConfigSource)
	}

	if err := store.SetClusterConfigs(cluster.ID, "", "worker-yaml-2", "edit"); err != nil {
		t.Fatalf("SetClusterConfigs partial: %v", err)
	}
	got, _ = store.GetCluster(cluster.ID)
	if got.ControlPlaneConfig != "cp-yaml" || got.WorkerConfig != "worker-yaml-2" {
		t.Error("expected partial update to preserve existing configs")
	}
	if got.ConfigSource != "edit" {
		t.Errorf("expected config source to update, got %q", got.ConfigSource)
	}
}

func TestNodeImageLifecycle(t *testing.T) {
	store := newTestStore(t)

	img := &models.NodeImage{Name: "talos v1.10.5", Provider: models.ProviderTalos, Version: "v1.10.5", ImageRef: "ref1"}
	if err := store.CreateNodeImage(img); err != nil {
		t.Fatalf("CreateNodeImage: %v", err)
	}
	if err := store.SetNodeImageDownloaded(img.ID, true); err != nil {
		t.Fatalf("SetNodeImageDownloaded: %v", err)
	}
	got, err := store.GetNodeImage(img.ID)
	if err != nil || !got.Downloaded {
		t.Fatalf("expected downloaded node image, got %v %v", got, err)
	}

	cluster := &models.Cluster{Name: "prod", NodeImageID: &img.ID}
	if err := store.CreateCluster(cluster); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := store.DeleteNodeImage(img.ID); err == nil {
		t.Error("expected delete to fail while a cluster references the node image")
	}

	if err := store.DeleteCluster(cluster.ID); err != nil {
		t.Fatalf("DeleteCluster: %v", err)
	}
	if err := store.DeleteNodeImage(img.ID); err != nil {
		t.Fatalf("expected delete to succeed once unreferenced: %v", err)
	}
}

func TestUpdateClusterWhitelistExcludesSecrets(t *testing.T) {
	store := newTestStore(t)
	cluster := newTestCluster(t, store, "prod")
	if err := store.SetClusterConfigs(cluster.ID, "cp-yaml", "worker-yaml", "upload"); err != nil {
		t.Fatalf("SetClusterConfigs: %v", err)
	}

	if err := store.UpdateCluster(cluster.ID, &models.Cluster{Name: "prod2", Description: "updated", ControlPlaneConfig: "evil"}); err != nil {
		t.Fatalf("UpdateCluster: %v", err)
	}
	got, _ := store.GetCluster(cluster.ID)
	if got.Name != "prod2" || got.Description != "updated" {
		t.Error("expected metadata update to apply")
	}
	if got.ControlPlaneConfig != "cp-yaml" {
		t.Error("expected UpdateCluster to never touch config blobs")
	}
}
