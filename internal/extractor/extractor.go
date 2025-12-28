package extractor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kdomanski/iso9660"
)

// BootFiles represents extracted kernel and initrd
type BootFiles struct {
	Kernel     string
	Initrd     string
	BootParams string
	Distro     string
}

// Extractor handles ISO mounting and boot file extraction
type Extractor struct {
	dataDir string
}

// New creates a new Extractor
func New(dataDir string) (*Extractor, error) {
	return &Extractor{
		dataDir: dataDir,
	}, nil
}

// Extract extracts kernel and initrd from an ISO
func (e *Extractor) Extract(isoPath string) (*BootFiles, error) {
	// Open ISO file
	f, err := os.Open(isoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ISO: %w", err)
	}
	defer f.Close()

	// Read ISO image
	img, err := iso9660.OpenImage(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read ISO image: %w", err)
	}

	// Detect distribution and find boot files
	bootFiles, err := e.detectAndExtract(img, isoPath)
	if err != nil {
		return nil, err
	}

	return bootFiles, nil
}

// detectAndExtract detects the distribution and extracts appropriate files
func (e *Extractor) detectAndExtract(img *iso9660.Image, isoPath string) (*BootFiles, error) {
	// Common paths for different distributions
	detectors := []struct {
		name     string
		detector func(*iso9660.Image) (*BootFiles, error)
	}{
		{"Ubuntu/Debian", e.detectUbuntuDebian},
		{"Fedora/RHEL", e.detectFedoraRHEL},
		{"CentOS/Rocky/Alma", e.detectCentOS},
		{"Arch Linux", e.detectArch},
		{"OpenSUSE", e.detectOpenSUSE},
	}

	var errors []string
	for _, d := range detectors {
		if files, err := d.detector(img); err == nil && files != nil {
			// Copy files to cache
			if err := e.cacheBootFiles(files, img, isoPath); err != nil {
				return nil, err
			}
			return files, nil
		} else {
			errors = append(errors, fmt.Sprintf("%s: %v", d.name, err))
		}
	}

	return nil, fmt.Errorf("unsupported distribution or unable to find boot files (tried: %s)", strings.Join(errors, "; "))
}

// detectUbuntuDebian detects Ubuntu/Debian ISOs
func (e *Extractor) detectUbuntuDebian(img *iso9660.Image) (*BootFiles, error) {
	// Try various Ubuntu/Debian paths
	paths := []struct {
		kernel     string
		initrd     string
		distro     string
		bootParams string
	}{
		{"/casper/vmlinuz", "/casper/initrd", "ubuntu", "boot=casper "},
		{"/casper/vmlinuz", "/casper/initrd.lz", "ubuntu", "boot=casper "},
		{"/install/vmlinuz", "/install/initrd.gz", "debian", ""},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "debian", ""},
		{"/live/vmlinuz", "/live/initrd.img", "debian", "boot=live "},
		{"/live/vmlinuz1", "/live/initrd1.img", "debian", "boot=live "},
	}

	for _, p := range paths {
		if fileExists(img, p.kernel) && fileExists(img, p.initrd) {
			return &BootFiles{
				Kernel:     p.kernel,
				Initrd:     p.initrd,
				Distro:     p.distro,
				BootParams: p.bootParams,
			}, nil
		}
	}

	return nil, fmt.Errorf("kernel/initrd not found in common Ubuntu/Debian paths")
}

// detectFedoraRHEL detects Fedora/RHEL ISOs
func (e *Extractor) detectFedoraRHEL(img *iso9660.Image) (*BootFiles, error) {
	kernel := "/images/pxeboot/vmlinuz"
	initrd := "/images/pxeboot/initrd.img"

	if fileExists(img, kernel) && fileExists(img, initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "fedora",
			BootParams: "inst.stage2=",
		}, nil
	}

	return nil, fmt.Errorf("not Fedora/RHEL")
}

// detectCentOS detects CentOS/Rocky/AlmaLinux ISOs
func (e *Extractor) detectCentOS(img *iso9660.Image) (*BootFiles, error) {
	kernel := "/images/pxeboot/vmlinuz"
	initrd := "/images/pxeboot/initrd.img"

	if fileExists(img, kernel) && fileExists(img, initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "centos",
			BootParams: "inst.repo=",
		}, nil
	}

	return nil, fmt.Errorf("not CentOS/Rocky/Alma")
}

// detectArch detects Arch Linux ISOs
func (e *Extractor) detectArch(img *iso9660.Image) (*BootFiles, error) {
	kernel := "/arch/boot/x86_64/vmlinuz-linux"
	initrd := "/arch/boot/x86_64/initramfs-linux.img"

	if fileExists(img, kernel) && fileExists(img, initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "arch",
			BootParams: "archisobasedir=arch archiso_http_srv=",
		}, nil
	}

	return nil, fmt.Errorf("not Arch Linux")
}

// detectOpenSUSE detects OpenSUSE ISOs
func (e *Extractor) detectOpenSUSE(img *iso9660.Image) (*BootFiles, error) {
	kernel := "/boot/x86_64/loader/linux"
	initrd := "/boot/x86_64/loader/initrd"

	if fileExists(img, kernel) && fileExists(img, initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "opensuse",
			BootParams: "install=",
		}, nil
	}

	return nil, fmt.Errorf("not OpenSUSE")
}

// cacheBootFiles copies boot files to ISO subdirectory
func (e *Extractor) cacheBootFiles(files *BootFiles, img *iso9660.Image, isoPath string) error {
	// Create subdirectory based on ISO filename within the isos directory
	isoBase := strings.TrimSuffix(filepath.Base(isoPath), filepath.Ext(isoPath))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	if err := os.MkdirAll(bootFilesDir, 0755); err != nil {
		return fmt.Errorf("failed to create boot files subdirectory: %w", err)
	}

	// Extract and copy kernel
	kernelDest := filepath.Join(bootFilesDir, "vmlinuz")
	if err := extractFile(img, files.Kernel, kernelDest); err != nil {
		return fmt.Errorf("failed to extract kernel: %w", err)
	}
	files.Kernel = kernelDest

	// Extract and copy initrd
	initrdDest := filepath.Join(bootFilesDir, "initrd")
	if err := extractFile(img, files.Initrd, initrdDest); err != nil {
		return fmt.Errorf("failed to extract initrd: %w", err)
	}
	files.Initrd = initrdDest

	return nil
}

// GetCachedBootFiles returns cached boot files if they exist
func (e *Extractor) GetCachedBootFiles(isoFilename string) (*BootFiles, error) {
	isoBase := strings.TrimSuffix(isoFilename, filepath.Ext(isoFilename))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	kernelPath := filepath.Join(bootFilesDir, "vmlinuz")
	initrdPath := filepath.Join(bootFilesDir, "initrd")

	if !fileExistsOnDisk(kernelPath) || !fileExistsOnDisk(initrdPath) {
		return nil, fmt.Errorf("cached files not found")
	}

	// Try to detect distro from metadata file if exists
	metadataPath := filepath.Join(bootFilesDir, "metadata.txt")
	distro := "unknown"
	bootParams := ""

	if data, err := os.ReadFile(metadataPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "distro=") {
				distro = strings.TrimPrefix(line, "distro=")
			}
			if strings.HasPrefix(line, "boot_params=") {
				bootParams = strings.TrimPrefix(line, "boot_params=")
			}
		}
	}

	return &BootFiles{
		Kernel:     kernelPath,
		Initrd:     initrdPath,
		Distro:     distro,
		BootParams: bootParams,
	}, nil
}

// SaveMetadata saves boot file metadata
func (e *Extractor) SaveMetadata(isoFilename string, files *BootFiles) error {
	isoBase := strings.TrimSuffix(isoFilename, filepath.Ext(isoFilename))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)
	metadataPath := filepath.Join(bootFilesDir, "metadata.txt")

	metadata := fmt.Sprintf("distro=%s\nboot_params=%s\n", files.Distro, files.BootParams)
	return os.WriteFile(metadataPath, []byte(metadata), 0644)
}

// fileExists checks if a file exists in the ISO image
func fileExists(img *iso9660.Image, path string) bool {
	_, err := findFile(img, path)
	return err == nil
}

// fileExistsOnDisk checks if a file exists on disk
func fileExistsOnDisk(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// findFile finds a file in the ISO by path
func findFile(img *iso9660.Image, path string) (*iso9660.File, error) {
	// Remove leading slash and normalize path
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")

	root, err := img.RootDir()
	if err != nil {
		return nil, err
	}

	current := root
	for i, part := range parts {
		if part == "" {
			continue
		}

		// Get children of current directory
		children, err := current.GetChildren()
		if err != nil {
			return nil, fmt.Errorf("failed to get children: %w", err)
		}

		// Find matching child
		found := false
		for _, child := range children {
			// Case-insensitive comparison (ISO9660 is typically uppercase)
			if strings.EqualFold(child.Name(), part) {
				current = child
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("path not found: %s (missing: %s)", path, part)
		}

		// If this is the last part, return it
		if i == len(parts)-1 {
			return current, nil
		}

		// Otherwise, ensure it's a directory
		if !current.IsDir() {
			return nil, fmt.Errorf("not a directory: %s", part)
		}
	}

	return current, nil
}

// extractFile extracts a file from ISO to destination
func extractFile(img *iso9660.Image, isoPath, destPath string) error {
	file, err := findFile(img, isoPath)
	if err != nil {
		return fmt.Errorf("file not found in ISO: %s: %w", isoPath, err)
	}

	if file.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", isoPath)
	}

	reader := file.Reader()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dest.Close()

	_, err = io.Copy(dest, reader)
	return err
}
