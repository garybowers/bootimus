package admin

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (h *Handler) DownloadNetboot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendJSON(w, http.StatusMethodNotAllowed, Response{Success: false, Error: "Method not allowed"})
		return
	}

	filename := r.URL.Query().Get("filename")
	if filename == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{Success: false, Error: "Missing filename parameter"})
		return
	}

	image, err := h.storage.GetImage(filename)
	if err != nil {
		h.sendJSON(w, http.StatusNotFound, Response{Success: false, Error: "Image not found"})
		return
	}

	if !image.NetbootRequired {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "Netboot download not required for this image",
		})
		return
	}

	if image.NetbootURL == "" {
		h.sendJSON(w, http.StatusBadRequest, Response{
			Success: false,
			Error:   "No netboot URL configured for this image",
		})
		return
	}

	imageDir := filepath.Join(h.isoDir, strings.TrimSuffix(filename, filepath.Ext(filename))+"-netboot")
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create netboot directory: %v", err),
		})
		return
	}

	log.Printf("Downloading netboot tarball from: %s", image.NetbootURL)

	resp, err := http.Get(image.NetbootURL)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to download netboot tarball: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to download: HTTP %d", resp.StatusCode),
		})
		return
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to create gzip reader: %v", err),
		})
		return
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	filesExtracted := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			h.sendJSON(w, http.StatusInternalServerError, Response{
				Success: false,
				Error:   fmt.Sprintf("Failed to read tar: %v", err),
			})
			return
		}

		targetPath := filepath.Join(imageDir, header.Name)

		if !strings.HasPrefix(targetPath, filepath.Clean(imageDir)+string(os.PathSeparator)) {
			log.Printf("Warning: Skipping file outside target directory: %s", header.Name)
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				log.Printf("Warning: Failed to create directory %s: %v", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				log.Printf("Warning: Failed to create parent directory for %s: %v", targetPath, err)
				continue
			}

			outFile, err := os.Create(targetPath)
			if err != nil {
				log.Printf("Warning: Failed to create file %s: %v", targetPath, err)
				continue
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				log.Printf("Warning: Failed to write file %s: %v", targetPath, err)
				continue
			}
			outFile.Close()

			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
				log.Printf("Warning: Failed to set permissions on %s: %v", targetPath, err)
			}

			filesExtracted++
		}
	}

	log.Printf("Extracted %d files from netboot tarball to %s", filesExtracted, imageDir)

	var vmlinuzPath, initrdPath string
	filepath.Walk(imageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		name := info.Name()
		if strings.Contains(name, "vmlinuz") || name == "linux" {
			vmlinuzPath = path
		} else if strings.Contains(name, "initrd") {
			initrdPath = path
		}
		return nil
	})

	if vmlinuzPath == "" || initrdPath == "" {
		h.sendJSON(w, http.StatusInternalServerError, Response{
			Success: false,
			Error:   "Netboot files downloaded but vmlinuz/initrd not found in tarball",
		})
		return
	}

	imageRootDir := filepath.Join(h.isoDir, strings.TrimSuffix(filename, filepath.Ext(filename)))
	if err := copyFile(vmlinuzPath, filepath.Join(imageRootDir, "vmlinuz")); err != nil {
		log.Printf("Warning: Failed to copy vmlinuz: %v", err)
	}
	if err := copyFile(initrdPath, filepath.Join(imageRootDir, "initrd")); err != nil {
		log.Printf("Warning: Failed to copy initrd: %v", err)
	}

	image.NetbootAvailable = true
	if err := h.storage.UpdateImage(filename, image); err != nil {
		log.Printf("Warning: Failed to update image netboot status: %v", err)
	}

	h.sendJSON(w, http.StatusOK, Response{
		Success: true,
		Message: fmt.Sprintf("Netboot files downloaded and extracted successfully (%d files)", filesExtracted),
		Data: map[string]interface{}{
			"files_extracted":   filesExtracted,
			"netboot_available": true,
		},
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}
