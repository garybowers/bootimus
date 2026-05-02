package tools

import (
	"archive/zip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage"
)

//go:embed tools-profiles.json
var embeddedTools embed.FS

const RemoteToolsURL = "https://raw.githubusercontent.com/garybowers/bootimus/main/tools-profiles.json"

type ToolDefinition struct {
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	Description     string `json:"description"`
	Version         string `json:"version"`
	DownloadURL     string `json:"download_url"`
	KernelPath      string `json:"kernel_path"`
	InitrdPath      string `json:"initrd_path"`
	BootParams      string `json:"boot_params"`
	BootMethod      string `json:"boot_method,omitempty"`
	ArchiveType     string `json:"archive_type,omitempty"`
	DownloadURLBIOS string `json:"download_url_bios,omitempty"`
	KernelPathBIOS  string `json:"kernel_path_bios,omitempty"`
}

type toolsManifest struct {
	Version string           `json:"version"`
	Tools   []ToolDefinition `json:"tools"`
}

var BuiltInTools []ToolDefinition

func init() {
	data, err := embeddedTools.ReadFile("tools-profiles.json")
	if err != nil {
		panic(fmt.Sprintf("tools: failed to read embedded tools-profiles.json: %v", err))
	}
	var m toolsManifest
	if err := json.Unmarshal(data, &m); err != nil {
		panic(fmt.Sprintf("tools: failed to parse embedded tools-profiles.json: %v", err))
	}
	BuiltInTools = m.Tools
}

type DownloadProgress struct {
	Status     string  `json:"status"`
	Percent    float64 `json:"percent"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Error      string  `json:"error,omitempty"`
}

type Manager struct {
	store              storage.Storage
	dataDir            string
	progressMu         sync.RWMutex
	progress           map[string]*DownloadProgress
	DisableRemoteCheck bool
}

func NewManager(store storage.Storage, dataDir string) *Manager {
	return &Manager{store: store, dataDir: dataDir, progress: make(map[string]*DownloadProgress)}
}

func (m *Manager) GetProgress(name string) DownloadProgress {
	m.progressMu.RLock()
	defer m.progressMu.RUnlock()
	if p, ok := m.progress[name]; ok {
		return *p
	}
	return DownloadProgress{Status: "idle"}
}

func (m *Manager) setProgress(name string, p *DownloadProgress) {
	m.progressMu.Lock()
	defer m.progressMu.Unlock()
	m.progress[name] = p
}

func (m *Manager) ToolsDir() string {
	return filepath.Join(m.dataDir, "tools")
}

func (m *Manager) ToolDir(name string) string {
	return filepath.Join(m.ToolsDir(), name)
}

func GetDefinition(name string) *ToolDefinition {
	for i := range BuiltInTools {
		if BuiltInTools[i].Name == name {
			return &BuiltInTools[i]
		}
	}
	return nil
}

func (m *Manager) SeedTools() error {
	data, err := embeddedTools.ReadFile("tools-profiles.json")
	if err != nil {
		return fmt.Errorf("failed to read embedded tools manifest: %w", err)
	}

	var mnf toolsManifest
	if err := json.Unmarshal(data, &mnf); err != nil {
		return fmt.Errorf("failed to parse embedded tools manifest: %w", err)
	}

	added, updated := m.applyManifest(mnf)
	if added+updated > 0 {
		log.Printf("Tools: Seeded/updated %d tools (version: %s)", added+updated, mnf.Version)
	} else {
		log.Printf("Tools: %d tools loaded (version: %s)", len(mnf.Tools), mnf.Version)
	}
	return nil
}

func (m *Manager) UpdateFromRemote() (added int, updated int, version string, err error) {
	if m.DisableRemoteCheck {
		return 0, 0, "", fmt.Errorf("remote tool updates are disabled")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(RemoteToolsURL)
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to fetch remote tools manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("remote tools manifest returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to read response: %w", err)
	}

	var mnf toolsManifest
	if err := json.Unmarshal(body, &mnf); err != nil {
		return 0, 0, "", fmt.Errorf("failed to parse remote tools manifest: %w", err)
	}

	added, updated = m.applyManifest(mnf)
	log.Printf("Tools: Remote update complete (version: %s, added: %d, updated: %d)", mnf.Version, added, updated)
	return added, updated, mnf.Version, nil
}

func (m *Manager) applyManifest(mnf toolsManifest) (added int, updated int) {
	BuiltInTools = mnf.Tools

	for _, def := range mnf.Tools {
		existing, err := m.store.GetBootTool(def.Name)
		if err != nil {
			tool := &models.BootTool{
				Name:        def.Name,
				DisplayName: def.DisplayName,
				Description: def.Description,
				Version:     def.Version,
				DownloadURL: def.DownloadURL,
				KernelPath:  def.KernelPath,
				InitrdPath:  def.InitrdPath,
				BootParams:  def.BootParams,
				BootMethod:  def.BootMethod,
				ArchiveType: def.ArchiveType,
				Downloaded:  m.IsDownloaded(def.Name),
			}
			if err := m.store.SaveBootTool(tool); err != nil {
				log.Printf("Tools: Failed to seed %s: %v", def.Name, err)
				continue
			}
			added++
			continue
		}

		if existing.Custom {
			continue
		}

		existing.DisplayName = def.DisplayName
		existing.Description = def.Description
		existing.Version = def.Version
		existing.DownloadURL = def.DownloadURL
		existing.KernelPath = def.KernelPath
		existing.InitrdPath = def.InitrdPath
		existing.BootParams = def.BootParams
		existing.BootMethod = def.BootMethod
		existing.ArchiveType = def.ArchiveType
		existing.Downloaded = m.IsDownloaded(def.Name)
		if err := m.store.SaveBootTool(existing); err != nil {
			log.Printf("Tools: Failed to update %s: %v", def.Name, err)
			continue
		}
		updated++
	}
	return added, updated
}

func (m *Manager) IsDownloaded(name string) bool {
	var kernelPath, initrdPath string

	if def := GetDefinition(name); def != nil {
		kernelPath = def.KernelPath
		initrdPath = def.InitrdPath
	} else if tool, err := m.store.GetBootTool(name); err == nil && tool.Custom {
		kernelPath = tool.KernelPath
		initrdPath = tool.InitrdPath
	} else {
		return false
	}

	if kernelPath == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(m.ToolDir(name), kernelPath)); err != nil {
		return false
	}
	if initrdPath != "" {
		if _, err := os.Stat(filepath.Join(m.ToolDir(name), initrdPath)); err != nil {
			return false
		}
	}
	return true
}

func (m *Manager) Download(name string, progressCh chan<- string) error {
	tool, err := m.store.GetBootTool(name)
	if err != nil {
		return fmt.Errorf("tool not found in database: %w", err)
	}

	var displayName, downloadURL, kernelPath, archiveType string
	var downloadURLBIOS, kernelPathBIOS string

	if def := GetDefinition(name); def != nil {
		displayName = def.DisplayName
		downloadURL = tool.DownloadURL
		if downloadURL == "" {
			downloadURL = def.DownloadURL
		}
		kernelPath = def.KernelPath
		archiveType = def.ArchiveType
		downloadURLBIOS = def.DownloadURLBIOS
		kernelPathBIOS = def.KernelPathBIOS
	} else if tool.Custom {
		displayName = tool.DisplayName
		downloadURL = tool.DownloadURL
		kernelPath = tool.KernelPath
		archiveType = tool.ArchiveType
		downloadURLBIOS = tool.DownloadURLBIOS
		kernelPathBIOS = tool.KernelPathBIOS
	} else {
		return fmt.Errorf("unknown tool: %s", name)
	}

	if downloadURL == "" {
		return fmt.Errorf("no download URL configured for %s", name)
	}

	toolDir := m.ToolDir(name)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return fmt.Errorf("failed to create tool directory: %w", err)
	}

	log.Printf("Tools: Downloading %s from %s", displayName, downloadURL)

	tmpFile, err := os.CreateTemp("", "bootimus-tool-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Bootimus PXE Server")

	client := &http.Client{
		Timeout: 30 * time.Minute,
	}
	resp, err := client.Do(req)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		m.setProgress(name, &DownloadProgress{Status: "error", Error: fmt.Sprintf("HTTP %d", resp.StatusCode)})
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	totalSize := resp.ContentLength
	prog := &DownloadProgress{Status: "downloading", Total: totalSize}
	m.setProgress(name, prog)

	pw := &progressWriter{
		writer: tmpFile,
		onProgress: func(n int64) {
			prog.Downloaded = n
			if totalSize > 0 {
				prog.Percent = float64(n) / float64(totalSize) * 100
			}
			m.setProgress(name, prog)
		},
	}

	written, err := io.Copy(pw, resp.Body)
	tmpFile.Close()
	if err != nil {
		m.setProgress(name, &DownloadProgress{Status: "error", Error: err.Error()})
		return fmt.Errorf("download write failed: %w", err)
	}

	log.Printf("Tools: Downloaded %s (%d bytes)", displayName, written)
	m.setProgress(name, &DownloadProgress{Status: "extracting", Percent: 100, Downloaded: written, Total: totalSize})

	if archiveType == "" {
		archiveType = "zip"
	}

	switch archiveType {
	case "zip":
		if err := extractZip(tmpPath, toolDir); err != nil {
			m.setProgress(name, &DownloadProgress{Status: "error", Error: err.Error()})
			return fmt.Errorf("zip extraction failed: %w", err)
		}
	case "bin":
		destName := kernelPath
		if destName == "" {
			destName = filepath.Base(downloadURL)
		}
		destPath := filepath.Join(toolDir, destName)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			m.setProgress(name, &DownloadProgress{Status: "error", Error: err.Error()})
			return fmt.Errorf("failed to create directory: %w", err)
		}
		if err := copyFile(tmpPath, destPath); err != nil {
			m.setProgress(name, &DownloadProgress{Status: "error", Error: err.Error()})
			return fmt.Errorf("failed to copy binary: %w", err)
		}
	case "iso":
		isoPath := filepath.Join(toolDir, name+".iso")
		if err := copyFile(tmpPath, isoPath); err != nil {
			m.setProgress(name, &DownloadProgress{Status: "error", Error: err.Error()})
			return fmt.Errorf("failed to copy ISO: %w", err)
		}
	default:
		m.setProgress(name, &DownloadProgress{Status: "error", Error: "unknown archive type: " + archiveType})
		return fmt.Errorf("unknown archive type: %s", archiveType)
	}

	if downloadURLBIOS != "" && kernelPathBIOS != "" {
		biosDest := filepath.Join(toolDir, kernelPathBIOS)
		if err := os.MkdirAll(filepath.Dir(biosDest), 0755); err != nil {
			m.setProgress(name, &DownloadProgress{Status: "error", Error: err.Error()})
			return fmt.Errorf("failed to create BIOS directory: %w", err)
		}
		log.Printf("Tools: Downloading BIOS variant for %s from %s", displayName, downloadURLBIOS)
		if err := m.fetchToFile(downloadURLBIOS, biosDest); err != nil {
			m.setProgress(name, &DownloadProgress{Status: "error", Error: err.Error()})
			return fmt.Errorf("failed to download BIOS variant: %w", err)
		}
	}

	tool, err = m.store.GetBootTool(name)
	if err != nil {
		return err
	}
	tool.Downloaded = true
	if err := m.store.SaveBootTool(tool); err != nil {
		return err
	}

	log.Printf("Tools: %s ready at %s", displayName, toolDir)
	m.setProgress(name, &DownloadProgress{Status: "done", Percent: 100, Downloaded: written, Total: totalSize})

	return nil
}

func (m *Manager) fetchToFile(rawURL, destPath string) error {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Bootimus PXE Server")
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Delete(name string) error {
	toolDir := m.ToolDir(name)
	if err := os.RemoveAll(toolDir); err != nil {
		return err
	}

	tool, err := m.store.GetBootTool(name)
	if err != nil {
		return err
	}
	tool.Downloaded = false
	tool.Enabled = false
	return m.store.SaveBootTool(tool)
}

func (m *Manager) GetEnabledTools(serverURL string) []EnabledTool {
	tools, err := m.store.ListBootTools()
	if err != nil {
		return nil
	}

	var result []EnabledTool
	for _, tool := range tools {
		if !tool.Enabled || !tool.Downloaded {
			continue
		}

		var kp, ip, bp, bm, kpBIOS string

		if def := GetDefinition(tool.Name); def != nil {
			kp = def.KernelPath
			ip = def.InitrdPath
			bp = def.BootParams
			bm = def.BootMethod
			kpBIOS = def.KernelPathBIOS
		} else if tool.Custom {
			kp = tool.KernelPath
			ip = tool.InitrdPath
			bp = tool.BootParams
			bm = tool.BootMethod
			kpBIOS = tool.KernelPathBIOS
		} else {
			continue
		}

		if kp == "" {
			continue
		}

		params := strings.ReplaceAll(bp, "{{HTTP_URL}}", serverURL)
		if bm == "" {
			bm = "kernel"
		}
		et := EnabledTool{
			Name:        tool.Name,
			DisplayName: tool.DisplayName,
			KernelURL:   fmt.Sprintf("%s/tools/%s/%s", serverURL, tool.Name, kp),
			BootParams:  params,
			BootMethod:  bm,
		}
		if ip != "" {
			et.InitrdURL = fmt.Sprintf("%s/tools/%s/%s", serverURL, tool.Name, ip)
		}
		if kpBIOS != "" {
			et.KernelURLBIOS = fmt.Sprintf("%s/tools/%s/%s", serverURL, tool.Name, kpBIOS)
		}
		result = append(result, et)
	}
	return result
}

type EnabledTool struct {
	Name          string
	DisplayName   string
	KernelURL     string
	KernelURLBIOS string
	InitrdURL     string
	BootParams    string
	BootMethod    string
}

type progressWriter struct {
	writer     io.Writer
	written    int64
	onProgress func(int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.written += int64(n)
	if pw.onProgress != nil {
		pw.onProgress(pw.written)
	}
	return n, err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
