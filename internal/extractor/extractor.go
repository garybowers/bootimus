package extractor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	cacheDir string
	dataDir  string
}

// New creates a new Extractor
func New(cacheDir, dataDir string) (*Extractor, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Extractor{
		cacheDir: cacheDir,
		dataDir:  dataDir,
	}, nil
}

// Extract extracts kernel and initrd from an ISO
func (e *Extractor) Extract(isoPath string) (*BootFiles, error) {
	// Create mount point
	mountPoint := filepath.Join(e.cacheDir, "mnt_"+filepath.Base(isoPath))
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return nil, fmt.Errorf("failed to create mount point: %w", err)
	}
	defer os.RemoveAll(mountPoint)

	// Mount the ISO
	cmd := exec.Command("mount", "-o", "loop,ro", isoPath, mountPoint)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to mount ISO: %w", err)
	}
	defer exec.Command("umount", mountPoint).Run()

	// Detect distribution and find boot files
	bootFiles, err := e.detectAndExtract(mountPoint, isoPath)
	if err != nil {
		return nil, err
	}

	return bootFiles, nil
}

// detectAndExtract detects the distribution and extracts appropriate files
func (e *Extractor) detectAndExtract(mountPoint, isoPath string) (*BootFiles, error) {
	// Common paths for different distributions
	detectors := []struct {
		name     string
		detector func(string) (*BootFiles, error)
	}{
		{"Ubuntu/Debian", e.detectUbuntuDebian},
		{"Fedora/RHEL", e.detectFedoraRHEL},
		{"CentOS/Rocky/Alma", e.detectCentOS},
		{"Arch Linux", e.detectArch},
		{"OpenSUSE", e.detectOpenSUSE},
	}

	for _, d := range detectors {
		if files, err := d.detector(mountPoint); err == nil && files != nil {
			// Copy files to cache
			if err := e.cacheBootFiles(files, isoPath); err != nil {
				return nil, err
			}
			return files, nil
		}
	}

	return nil, fmt.Errorf("unsupported distribution or unable to find boot files")
}

// detectUbuntuDebian detects Ubuntu/Debian ISOs
func (e *Extractor) detectUbuntuDebian(mountPoint string) (*BootFiles, error) {
	// Ubuntu Desktop/Server locations
	casperKernel := filepath.Join(mountPoint, "casper", "vmlinuz")
	casperInitrd := filepath.Join(mountPoint, "casper", "initrd")

	// Check casper (Ubuntu Desktop)
	if fileExists(casperKernel) && fileExists(casperInitrd) {
		return &BootFiles{
			Kernel:     casperKernel,
			Initrd:     casperInitrd,
			Distro:     "ubuntu",
			BootParams: "boot=casper",
		}, nil
	}

	// Debian installer locations
	installKernel := filepath.Join(mountPoint, "install", "vmlinuz")
	installInitrd := filepath.Join(mountPoint, "install", "initrd.gz")

	if fileExists(installKernel) && fileExists(installInitrd) {
		return &BootFiles{
			Kernel:     installKernel,
			Initrd:     installInitrd,
			Distro:     "debian",
			BootParams: "",
		}, nil
	}

	return nil, fmt.Errorf("not Ubuntu/Debian")
}

// detectFedoraRHEL detects Fedora/RHEL ISOs
func (e *Extractor) detectFedoraRHEL(mountPoint string) (*BootFiles, error) {
	kernel := filepath.Join(mountPoint, "images", "pxeboot", "vmlinuz")
	initrd := filepath.Join(mountPoint, "images", "pxeboot", "initrd.img")

	if fileExists(kernel) && fileExists(initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "fedora",
			BootParams: "inst.stage2=hd:LABEL=",
		}, nil
	}

	return nil, fmt.Errorf("not Fedora/RHEL")
}

// detectCentOS detects CentOS/Rocky/AlmaLinux ISOs
func (e *Extractor) detectCentOS(mountPoint string) (*BootFiles, error) {
	kernel := filepath.Join(mountPoint, "images", "pxeboot", "vmlinuz")
	initrd := filepath.Join(mountPoint, "images", "pxeboot", "initrd.img")

	if fileExists(kernel) && fileExists(initrd) {
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
func (e *Extractor) detectArch(mountPoint string) (*BootFiles, error) {
	kernel := filepath.Join(mountPoint, "arch", "boot", "x86_64", "vmlinuz-linux")
	initrd := filepath.Join(mountPoint, "arch", "boot", "x86_64", "initramfs-linux.img")

	if fileExists(kernel) && fileExists(initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "arch",
			BootParams: "archisobasedir=arch archisolabel=",
		}, nil
	}

	return nil, fmt.Errorf("not Arch Linux")
}

// detectOpenSUSE detects OpenSUSE ISOs
func (e *Extractor) detectOpenSUSE(mountPoint string) (*BootFiles, error) {
	kernel := filepath.Join(mountPoint, "boot", "x86_64", "loader", "linux")
	initrd := filepath.Join(mountPoint, "boot", "x86_64", "loader", "initrd")

	if fileExists(kernel) && fileExists(initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "opensuse",
			BootParams: "install=",
		}, nil
	}

	return nil, fmt.Errorf("not OpenSUSE")
}

// cacheBootFiles copies boot files to cache directory
func (e *Extractor) cacheBootFiles(files *BootFiles, isoPath string) error {
	// Create cache subdirectory based on ISO filename
	isoBase := strings.TrimSuffix(filepath.Base(isoPath), filepath.Ext(isoPath))
	cacheSubdir := filepath.Join(e.cacheDir, isoBase)

	if err := os.MkdirAll(cacheSubdir, 0755); err != nil {
		return fmt.Errorf("failed to create cache subdirectory: %w", err)
	}

	// Copy kernel
	kernelDest := filepath.Join(cacheSubdir, "vmlinuz")
	if err := copyFile(files.Kernel, kernelDest); err != nil {
		return fmt.Errorf("failed to copy kernel: %w", err)
	}
	files.Kernel = kernelDest

	// Copy initrd
	initrdDest := filepath.Join(cacheSubdir, "initrd")
	if err := copyFile(files.Initrd, initrdDest); err != nil {
		return fmt.Errorf("failed to copy initrd: %w", err)
	}
	files.Initrd = initrdDest

	return nil
}

// GetCachedBootFiles returns cached boot files if they exist
func (e *Extractor) GetCachedBootFiles(isoFilename string) (*BootFiles, error) {
	isoBase := strings.TrimSuffix(isoFilename, filepath.Ext(isoFilename))
	cacheSubdir := filepath.Join(e.cacheDir, isoBase)

	kernelPath := filepath.Join(cacheSubdir, "vmlinuz")
	initrdPath := filepath.Join(cacheSubdir, "initrd")

	if !fileExists(kernelPath) || !fileExists(initrdPath) {
		return nil, fmt.Errorf("cached files not found")
	}

	// Try to detect distro from metadata file if exists
	metadataPath := filepath.Join(cacheSubdir, "metadata.txt")
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
	cacheSubdir := filepath.Join(e.cacheDir, isoBase)
	metadataPath := filepath.Join(cacheSubdir, "metadata.txt")

	metadata := fmt.Sprintf("distro=%s\nboot_params=%s\n", files.Distro, files.BootParams)
	return os.WriteFile(metadataPath, []byte(metadata), 0644)
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
