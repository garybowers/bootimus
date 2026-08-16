package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bootimus/internal/kubernetes"
	_ "bootimus/internal/kubernetes/flatcar"
	_ "bootimus/internal/kubernetes/talos"
	"bootimus/internal/models"

	"bootimus/internal/webhook"
	"bootimus/internal/wol"

	"gorm.io/gorm"
)

type clusterInfo struct {
	models.Cluster
	HasControlPlaneConfig bool   `json:"has_control_plane_config"`
	HasWorkerConfig       bool   `json:"has_worker_config"`
	NodeImageName         string `json:"node_image_name,omitempty"`
	NodeImageProvider     string `json:"node_image_provider,omitempty"`
	NodeImageVersion      string `json:"node_image_version,omitempty"`
	NodeImageDownloaded   bool   `json:"node_image_downloaded"`
	NodeCount             int    `json:"node_count"`
}

func (h *Handler) clusterInfoFrom(cluster *models.Cluster, nodeCount int) clusterInfo {
	info := clusterInfo{
		Cluster:               *cluster,
		HasControlPlaneConfig: cluster.ControlPlaneConfig != "",
		HasWorkerConfig:       cluster.WorkerConfig != "",
		NodeCount:             nodeCount,
	}
	if cluster.NodeImageID != nil {
		if img, err := h.storage.GetNodeImage(*cluster.NodeImageID); err == nil {
			info.NodeImageName = img.Name
			info.NodeImageProvider = img.Provider
			info.NodeImageVersion = img.Version
			info.NodeImageDownloaded = img.Downloaded
		}
	}
	return info
}

func (h *Handler) clusterFromQuery(w http.ResponseWriter, r *http.Request) (*models.Cluster, bool) {
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing or invalid id parameter"})
		return nil, false
	}
	cluster, err := h.storage.GetCluster(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Cluster not found"})
		return nil, false
	}
	return cluster, true
}

func normaliseMAC(mac string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(mac), "-", ":"))
}

func (h *Handler) Clusters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		clusters, err := h.storage.ListClusters()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		infos := make([]clusterInfo, 0, len(clusters))
		for _, c := range clusters {
			nodes, _ := h.storage.ListClusterNodes(c.ID)
			infos = append(infos, h.clusterInfoFrom(c, len(nodes)))
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: infos})

	case http.MethodPost:
		var req struct {
			Name               string `json:"name"`
			Description        string `json:"description"`
			NodeImageID        *uint  `json:"node_image_id"`
			KernelParams       string `json:"kernel_params"`
			DefaultInstallDisk string `json:"default_install_disk"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid JSON body"})
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Cluster name is required"})
			return
		}
		if req.NodeImageID != nil {
			if _, err := h.storage.GetNodeImage(*req.NodeImageID); err != nil {
				h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Selected node image not found"})
				return
			}
		}
		cluster := models.Cluster{
			Name:               req.Name,
			Description:        req.Description,
			NodeImageID:        req.NodeImageID,
			KernelParams:       req.KernelParams,
			DefaultInstallDisk: req.DefaultInstallDisk,
		}
		if err := h.storage.CreateCluster(&cluster); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		log.Printf("Admin: Cluster created - %s", cluster.Name)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: h.clusterInfoFrom(&cluster, 0)})

	default:
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
	}
}

func (h *Handler) UpdateCluster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	existing, ok := h.clusterFromQuery(w, r)
	if !ok {
		return
	}
	var cluster models.Cluster
	if err := json.NewDecoder(r.Body).Decode(&cluster); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid JSON body"})
		return
	}
	if err := h.storage.UpdateCluster(existing.ID, &cluster); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Cluster updated - %s", cluster.Name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Cluster updated"})
}

func (h *Handler) DeleteCluster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	cluster, ok := h.clusterFromQuery(w, r)
	if !ok {
		return
	}
	if err := h.storage.DeleteCluster(cluster.ID); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Cluster deleted - %s (nodes detached)", cluster.Name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Cluster deleted"})
}

func (h *Handler) ClusterConfig(w http.ResponseWriter, r *http.Request) {
	cluster, ok := h.clusterFromQuery(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]bool{
			"has_control_plane_config": cluster.ControlPlaneConfig != "",
			"has_worker_config":        cluster.WorkerConfig != "",
		}})

	case http.MethodPost:
		var req struct {
			ControlPlaneConfig string `json:"control_plane_config"`
			WorkerConfig       string `json:"worker_config"`
			Source             string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid JSON body"})
			return
		}
		if req.ControlPlaneConfig == "" && req.WorkerConfig == "" {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "No config documents provided"})
			return
		}
		if cluster.NodeImageID == nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Assign a node image to this cluster before saving configs"})
			return
		}
		nodeImage, err := h.storage.GetNodeImage(*cluster.NodeImageID)
		if err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Cluster node image not found"})
			return
		}
		provider, err := kubernetes.GetProvider(nodeImage.Provider)
		if err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
			return
		}
		for _, cfg := range []string{req.ControlPlaneConfig, req.WorkerConfig} {
			if cfg != "" && !provider.ValidateConfig(cfg) {
				h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: fmt.Sprintf("Config is not a valid %s join config", provider.Info().Label)})
				return
			}
		}

		source := req.Source
		if source == "" {
			source = "upload"
		}
		if err := h.storage.SetClusterConfigs(cluster.ID, req.ControlPlaneConfig, req.WorkerConfig, source); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}

		log.Printf("Admin: Join configs saved for cluster %s (%s)", cluster.Name, provider.Name())
		h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Join configs saved"})

	default:
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
	}
}

func (h *Handler) ConfigTemplate(w http.ResponseWriter, r *http.Request) {
	providerName := r.URL.Query().Get("provider")
	role := r.URL.Query().Get("role")
	provider, err := kubernetes.GetProvider(providerName)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: map[string]string{"template": provider.ConfigTemplate(role)}})
}

func (h *Handler) KubernetesProviders(w http.ResponseWriter, r *http.Request) {
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: kubernetes.ProvidersInfo()})
}

func (h *Handler) downloadProvisionAsset(url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create asset directory: %w", err)
	}
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned HTTP %d", url, resp.StatusCode)
	}
	tmpPath := destPath + ".partial"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", tmpPath, err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write %s: %w", destPath, err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, destPath)
}

func (h *Handler) nodeImageFromQuery(w http.ResponseWriter, r *http.Request) (*models.NodeImage, bool) {
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing or invalid id parameter"})
		return nil, false
	}
	image, err := h.storage.GetNodeImage(uint(id))
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Node image not found"})
		return nil, false
	}
	return image, true
}

func (h *Handler) NodeImages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		images, err := h.storage.ListNodeImages()
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: images})

	case http.MethodPost:
		var req struct {
			Name     string `json:"name"`
			Provider string `json:"provider"`
			Version  string `json:"version"`
			ImageRef string `json:"image_ref"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid JSON body"})
			return
		}
		provider, err := kubernetes.GetProvider(req.Provider)
		if err != nil {
			h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
			return
		}
		info := provider.Info()
		if strings.TrimSpace(req.Version) == "" {
			req.Version = info.VersionDefault
		}
		if strings.TrimSpace(req.ImageRef) == "" {
			req.ImageRef = info.RefDefault
		}
		if strings.TrimSpace(req.Name) == "" {
			req.Name = fmt.Sprintf("%s %s", info.Label, req.Version)
		}
		image := models.NodeImage{Name: req.Name, Provider: provider.Name(), Version: req.Version, ImageRef: req.ImageRef}
		if err := h.storage.CreateNodeImage(&image); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
		log.Printf("Admin: Node image created - %s (%s %s)", image.Name, image.Provider, image.Version)
		h.sendJSON(w, http.StatusOK, Response{Success: true, Data: image})

	default:
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
	}
}

func (h *Handler) DeleteNodeImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	image, ok := h.nodeImageFromQuery(w, r)
	if !ok {
		return
	}
	if err := h.storage.DeleteNodeImage(image.ID); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	log.Printf("Admin: Node image deleted - %s", image.Name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Node image deleted"})
}

func (h *Handler) DownloadNodeImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	image, ok := h.nodeImageFromQuery(w, r)
	if !ok {
		return
	}
	provider, err := kubernetes.GetProvider(image.Provider)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}
	assets, err := provider.Assets(image)
	if err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Downloading %s boot assets for node image %s", provider.Name(), image.Name)

	assetRoot := filepath.Join(h.dataDir, "node-images")
	for _, asset := range assets {
		dest := filepath.Join(assetRoot, filepath.FromSlash(asset.Path))
		if !strings.HasPrefix(filepath.Clean(dest), filepath.Clean(assetRoot)+string(os.PathSeparator)) {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Invalid asset path"})
			return
		}
		if err := h.downloadProvisionAsset(asset.URL, dest); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
			return
		}
	}
	if err := h.storage.SetNodeImageDownloaded(image.ID, true); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	log.Printf("Admin: Boot assets ready for node image %s", image.Name)
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: "Boot assets downloaded"})
}

func (h *Handler) ClusterNodes(w http.ResponseWriter, r *http.Request) {
	cluster, ok := h.clusterFromQuery(w, r)
	if !ok {
		return
	}
	nodes, err := h.storage.ListClusterNodes(cluster.ID)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Data: nodes})
}

func (h *Handler) fireEnrollmentEvent(event, mac, name string, metadata map[string]string) {
	if h.WebhookNotifier == nil {
		return
	}
	h.WebhookNotifier.Fire(webhook.Event{Event: event, MAC: mac, ClientName: name, Metadata: metadata})
}

func (h *Handler) AssignNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var req struct {
		MAC         string `json:"mac"`
		ClusterID   uint   `json:"cluster_id"`
		Role        string `json:"role"`
		InstallDisk string `json:"install_disk"`
		ConfigPatch string `json:"config_patch"`
		Reboot      bool   `json:"reboot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid JSON body"})
		return
	}
	mac := normaliseMAC(req.MAC)
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac"})
		return
	}
	if req.Role != models.ClusterRoleControlPlane && req.Role != models.ClusterRoleWorker {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Role must be controlplane or worker"})
		return
	}
	cluster, err := h.storage.GetCluster(req.ClusterID)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Cluster not found"})
		return
	}
	if _, err := h.storage.GetClient(mac); err != nil {
		if err := h.storage.CreateClient(&models.Client{MACAddress: mac}); err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: fmt.Sprintf("Failed to create client: %v", err)})
			return
		}
	}
	token, err := kubernetes.GenerateToken()
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: "Failed to generate node token"})
		return
	}
	if err := h.storage.AssignNodeToCluster(mac, cluster.ID, req.Role, req.InstallDisk, req.ConfigPatch, token, time.Now().Add(kubernetes.DefaultTokenTTL)); err != nil {
		if err == gorm.ErrRecordNotFound {
			h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
			return
		}
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}

	client, _ := h.storage.GetClient(mac)
	name := ""
	if client != nil {
		name = client.Name
	}
	log.Printf("Admin: Node %s assigned to cluster %s as %s", mac, cluster.Name, req.Role)
	h.fireEnrollmentEvent(webhook.EventNodeApproved, mac, name, map[string]string{"cluster": cluster.Name, "role": req.Role})

	message := "Node assigned - it will be provisioned on next PXE boot"
	if req.Reboot {
		if err := wol.SendMagicPacket(mac, h.wolBroadcastAddr); err != nil {
			message = fmt.Sprintf("Node assigned, but Wake-on-LAN failed: %v", err)
		} else {
			message = "Node assigned and Wake-on-LAN sent"
		}
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: message})
}

func (h *Handler) enrollmentStateAction(w http.ResponseWriter, r *http.Request, apply func(mac string, client *models.Client) (string, error)) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}
	var req struct {
		MAC string `json:"mac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Invalid JSON body"})
		return
	}
	mac := normaliseMAC(req.MAC)
	if mac == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing mac"})
		return
	}
	client, err := h.storage.GetClient(mac)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Client not found"})
		return
	}
	message, err := apply(mac, client)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{Success: false, Error: err.Error()})
		return
	}
	h.sendJSON(w, http.StatusOK, Response{Success: true, Message: message})
}

func (h *Handler) ReinstallNode(w http.ResponseWriter, r *http.Request) {
	h.enrollmentStateAction(w, r, func(mac string, client *models.Client) (string, error) {
		if client.ClusterID == nil {
			return "", fmt.Errorf("client is not assigned to a cluster")
		}
		token, err := kubernetes.GenerateToken()
		if err != nil {
			return "", fmt.Errorf("failed to generate node token")
		}
		if err := h.storage.AssignNodeToCluster(mac, *client.ClusterID, client.ClusterRole, client.InstallDisk, client.ConfigPatch, token, time.Now().Add(kubernetes.DefaultTokenTTL)); err != nil {
			return "", err
		}
		log.Printf("Admin: Node %s scheduled for reinstall", mac)
		return "Node will be reprovisioned on next PXE boot", nil
	})
}

func (h *Handler) UnassignNode(w http.ResponseWriter, r *http.Request) {
	h.enrollmentStateAction(w, r, func(mac string, client *models.Client) (string, error) {
		if err := h.storage.ResetClientEnrollment(mac); err != nil {
			return "", err
		}
		log.Printf("Admin: Node %s unassigned from cluster", mac)
		return "Node removed from cluster", nil
	})
}

func (h *Handler) MarkNodeInstalled(w http.ResponseWriter, r *http.Request) {
	h.enrollmentStateAction(w, r, func(mac string, client *models.Client) (string, error) {
		if err := h.storage.SetClientEnrollmentState(mac, models.EnrollmentStateInstalled); err != nil {
			return "", err
		}
		log.Printf("Admin: Node %s manually marked installed", mac)
		return "Node marked installed", nil
	})
}
