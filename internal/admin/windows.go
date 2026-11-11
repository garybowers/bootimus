package admin

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/wim"
)

func (h *Handler) RebuildBootWim(imageID uint) error {
	if !h.reserveDriverBuild(imageID) {
		return fmt.Errorf("a driver rebuild is already running for this image")
	}
	defer h.releaseDriverBuild(imageID)
	return h.runDriverBuild(imageID)
}

func (h *Handler) runDriverBuild(imageID uint) (result error) {
	image, err := h.driverImage(imageID)
	if err != nil {
		return err
	}
	mu := h.bootWimLock(image.Filename)
	mu.Lock()
	defer mu.Unlock()
	// Reload after waiting for another image operation, including SMB patching.
	image, err = h.driverImage(imageID)
	if err != nil {
		return err
	}
	packs, err := h.storage.ListDriverPacksByImage(imageID)
	if err != nil {
		return err
	}
	state := driverBuildState{Status: "applying", Revision: driverRevision(packs)}
	if err := h.saveDriverState(imageID, state); err != nil {
		return err
	}
	defer func() {
		if result != nil {
			state.Status, state.Error = "failed", result.Error()
			if err := h.saveDriverState(imageID, state); err != nil {
				log.Printf("Cannot save driver rebuild failure: %v", err)
			}
		}
	}()
	if reason := h.driverBuildReadiness(image); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	if err := h.rebuildDriverWim(image, packs); err != nil {
		return err
	}
	state.Status = "applied"
	if err := h.saveDriverState(imageID, state); err != nil {
		return fmt.Errorf("drivers applied but status could not be saved: %w", err)
	}
	return nil
}

func (h *Handler) rebuildDriverWim(image *models.Image, packs []*models.DriverPack) error {
	bootWimPath := h.extractedBootWimPath(image.Filename)
	imageDir := filepath.Join(h.isoDir, strings.TrimSuffix(image.Filename, filepath.Ext(image.Filename)))
	// Keep staging on the same filesystem so publication is an atomic rename.
	staging, err := os.MkdirTemp(filepath.Dir(bootWimPath), ".bootimus-drivers-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	driversDir := filepath.Join(staging, "drivers")
	if err := os.Mkdir(driversDir, 0755); err != nil {
		return err
	}
	for _, pack := range packs {
		if filepath.Base(pack.Filename) != pack.Filename {
			return fmt.Errorf("invalid driver pack filename")
		}
		// Keep packs separate while preserving each INF's relative paths.
		destination := filepath.Join(driversDir, "pack-"+strconv.FormatUint(uint64(pack.ID), 10))
		if err := unpackDriverArchive(filepath.Join(imageDir, "drivers", pack.Filename), destination); err != nil {
			return fmt.Errorf("driver pack %q: %w", pack.OriginalName, err)
		}
	}
	base := bootWimPath + ".orig"
	if _, err := os.Stat(base); os.IsNotExist(err) {
		source := bootWimPath
		legacy := bootWimPath + ".backup"
		if _, err := os.Stat(legacy); err == nil && !image.SMBInstallEnabled {
			source = legacy
		} else {
			for _, pack := range packs {
				if pack.LastApplied != nil {
					return fmt.Errorf("re-extract this image: its original boot.wim backup is missing")
				}
			}
		}
		if err := atomicCopyFile(source, base); err != nil {
			return fmt.Errorf("back up original boot.wim: %w", err)
		}
	}
	stagedWim := filepath.Join(staging, "boot.wim")
	if err := copyFile(base, stagedWim); err != nil {
		return fmt.Errorf("copy original boot.wim: %w", err)
	}
	activeInfo, err := os.Stat(bootWimPath)
	if err != nil {
		return err
	}
	manager, err := wim.NewManager()
	if err != nil {
		return err
	}
	count, err := manager.GetImageCount(stagedWim)
	if err != nil {
		return err
	}
	if count < 1 {
		return fmt.Errorf("boot.wim contains no images")
	}
	if len(packs) > 0 {
		for index := 1; index <= count; index++ {
			if err := manager.AddDirectory(stagedWim, index, driversDir, "/drivers"); err != nil {
				return err
			}
		}
	}
	// An empty list still restores the original WIM and its SMB startup script.
	if image.SMBInstallEnabled && !h.patchWindowsBootWimAt(image.Filename, stagedWim) {
		return fmt.Errorf("could not restore the SMB startup script; check the Windows SMB service and server logs")
	}
	if err := os.Chmod(stagedWim, activeInfo.Mode().Perm()); err != nil {
		return err
	}
	if err := syncFile(stagedWim); err != nil {
		return err
	}
	if err := os.Rename(stagedWim, bootWimPath); err != nil {
		return fmt.Errorf("publish rebuilt boot.wim: %w", err)
	}
	now := time.Now()
	for _, pack := range packs {
		pack.LastApplied = &now
		if err := h.storage.UpdateDriverPack(pack.ID, pack); err != nil {
			log.Printf("Cannot update driver pack %d application time: %v", pack.ID, err)
		}
	}
	log.Printf("Applied %d driver pack(s) to %s", len(packs), image.Filename)
	return nil
}

func syncFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func atomicCopyFile(source, target string) error {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".bootimus-copy-*")
	if err != nil {
		return err
	}
	temporary.Close()
	defer os.Remove(temporary.Name())
	if err := copyFile(source, temporary.Name()); err != nil {
		return err
	}
	if info, err := os.Stat(source); err == nil {
		if err := os.Chmod(temporary.Name(), info.Mode().Perm()); err != nil {
			return err
		}
	}
	if err := syncFile(temporary.Name()); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), target)
}
