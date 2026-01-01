package extractor

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/kdomanski/iso9660"
)

// BootFiles represents extracted kernel and initrd
type BootFiles struct {
	Kernel          string
	Initrd          string
	BootParams      string
	Distro          string
	ExtractedDir    string // Directory containing full ISO extraction for HTTP boot
	SquashfsPath    string // Path to filesystem.squashfs within the ISO (for Ubuntu/Debian)
	NetbootRequired bool   // Whether netboot files are required instead of ISO extraction
	NetbootURL      string // URL to download netboot tarball from
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
	// Try to detect actual distribution name from ISO metadata
	distroName := detectDistroName(img, isoPath)

	// Common paths for different distributions
	detectors := []struct {
		name     string
		detector func(*iso9660.Image) (*BootFiles, error)
	}{
		{"Windows", e.detectWindows},
		{"Ubuntu/Debian Family", e.detectUbuntuDebian},
		{"Arch Linux Family", e.detectArch},
		{"Fedora/RHEL Family", e.detectFedoraRHEL},
		{"CentOS/Rocky/Alma Family", e.detectCentOS},
		{"FreeBSD", e.detectFreeBSD},
		{"OpenSUSE", e.detectOpenSUSE},
		{"NixOS", e.detectNixOS},
	}

	var errors []string
	for _, d := range detectors {
		if files, err := d.detector(img); err == nil && files != nil {
			// Override distro if we detected a more specific name
			if distroName != "" {
				files.Distro = distroName
			}
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

// readFileContent reads a file from the ISO and returns its content as a string
func readFileContent(img *iso9660.Image, path string) string {
	file, err := findFile(img, path)
	if err != nil {
		return ""
	}

	if file.IsDir() {
		return ""
	}

	reader := file.Reader()
	content, err := io.ReadAll(reader)
	if err != nil {
		return ""
	}

	return string(content)
}

// detectDistroName tries to identify the specific distribution from ISO metadata
func detectDistroName(img *iso9660.Image, isoPath string) string {
	// Extract distribution name from ISO filename first (most reliable)
	filename := strings.ToLower(filepath.Base(isoPath))

	// Check for common distribution names in filename
	distroPatterns := map[string]string{
		"windows":  "windows",
		"win10":    "windows",
		"win11":    "windows",
		"win7":     "windows",
		"win8":     "windows",
		"server2022": "windows",
		"server2019": "windows",
		"server2016": "windows",
		"popos":    "popos",
		"pop-os":   "popos",
		"pop_os":   "popos",
		"manjaro":  "manjaro",
		"mint":     "mint",
		"linuxmint": "mint",
		"elementary": "elementary",
		"zorin":    "zorin",
		"ubuntu":   "ubuntu",
		"debian":   "debian",
		"arch":     "arch",
		"fedora":   "fedora",
		"centos":   "centos",
		"rocky":    "rocky",
		"alma":     "alma",
		"kali":     "kali",
		"parrot":   "parrot",
		"tails":    "tails",
		"opensuse": "opensuse",
		"freebsd":  "freebsd",
		"nixos":    "nixos",
		"endeavouros": "endeavouros",
		"garuda":   "garuda",
		"arco":     "arco",
	}

	for pattern, distro := range distroPatterns {
		if strings.Contains(filename, pattern) {
			return distro
		}
	}

	// Try to read .disk/info for Ubuntu derivatives
	if fileExists(img, "/.disk/info") {
		if content := readFileContent(img, "/.disk/info"); content != "" {
			contentLower := strings.ToLower(content)
			for pattern, distro := range distroPatterns {
				if strings.Contains(contentLower, pattern) {
					return distro
				}
			}
		}
	}

	return ""
}

// detectUbuntuDebian detects Ubuntu/Debian ISOs
func (e *Extractor) detectUbuntuDebian(img *iso9660.Image) (*BootFiles, error) {
	log.Printf("Checking for Ubuntu/Debian ISO...")

	// Try wildcard matching first for casper directory (handles Pop!_OS and other variants)
	log.Printf("Checking /casper with wildcard matching...")
	if found := findKernelInitrd(img, "/casper", "vmlinuz", "initrd"); found != nil {
		log.Printf("Found kernel/initrd in /casper using wildcard matching")
		found.Distro = "ubuntu"
		found.BootParams = "boot=casper fetch= "
		return found, nil
	}

	// Pop!_OS uses versioned casper directories (e.g., casper_pop-os_24.04_amd64_generic_debug_443)
	// Search for directories starting with "casper"
	if found := findCasperVariant(img); found != nil {
		found.Distro = "ubuntu"
		found.BootParams = "boot=casper fetch= "
		return found, nil
	}

	// Debian installer - search for install* directories
	if found := findInstallVariant(img); found != nil {
		found.Distro = "debian"
		found.BootParams = ""
		// Debian netinst ISOs need proper netboot files for network installation
		found.NetbootRequired = true
		found.NetbootURL = "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"
		return found, nil
	}

	// Try various Ubuntu/Debian paths
	paths := []struct {
		kernel     string
		initrd     string
		distro     string
		bootParams string
	}{
		// Ubuntu live server (newer versions)
		{"/casper/vmlinuz", "/casper/initrd", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz", "/casper/initrd.lz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz", "/casper/initrd.gz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		// Ubuntu live (desktop)
		{"/casper/vmlinuz.efi", "/casper/initrd.lz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz.efi", "/casper/initrd", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz.efi", "/casper/initrd.gz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		// Ubuntu Server installer (uses /install like Debian)
		{"/install/vmlinuz", "/install/initrd.gz", "ubuntu-installer", ""},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "ubuntu-installer", ""},
		// Debian installer
		{"/install/vmlinuz", "/install/initrd.gz", "debian", ""},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "debian", ""},
		// Debian live
		{"/live/vmlinuz", "/live/initrd.img", "debian", "boot=live fetch= "},
		{"/live/vmlinuz1", "/live/initrd1.img", "debian", "boot=live fetch= "},
		{"/live/vmlinuz-*", "/live/initrd.img-*", "debian", "boot=live fetch= "},
	}

	for _, p := range paths {
		// Handle wildcards for Debian live
		if strings.Contains(p.kernel, "*") {
			// Try to find files matching the pattern
			if found := findKernelInitrd(img, "/live", "vmlinuz", "initrd.img"); found != nil {
				found.Distro = "debian"
				found.BootParams = "boot=live fetch= "
				return found, nil
			}
		} else if fileExists(img, p.kernel) && fileExists(img, p.initrd) {
			bootFiles := &BootFiles{
				Kernel:     p.kernel,
				Initrd:     p.initrd,
				Distro:     p.distro,
				BootParams: p.bootParams,
			}
			// Mark Debian installer ISOs as requiring netboot
			if p.distro == "debian" && (strings.Contains(p.kernel, "/install") || strings.Contains(p.kernel, "/install.amd")) {
				bootFiles.NetbootRequired = true
				bootFiles.NetbootURL = "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"
			}
			// Mark Ubuntu Server installer ISOs as requiring netboot
			if p.distro == "ubuntu-installer" && (strings.Contains(p.kernel, "/install") || strings.Contains(p.kernel, "/install.amd")) {
				bootFiles.Distro = "ubuntu" // Use "ubuntu" for boot configuration
				bootFiles.NetbootRequired = true
				// Ubuntu netboot URLs from archive.ubuntu.com
				// For 24.04 LTS (Noble)
				bootFiles.NetbootURL = "http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz"
			}
			return bootFiles, nil
		}
	}

	return nil, fmt.Errorf("kernel/initrd not found in common Ubuntu/Debian paths")
}

// findCasperVariant searches for Pop!_OS style versioned casper directories
func findCasperVariant(img *iso9660.Image) *BootFiles {
	rootDir, err := findFile(img, "/")
	if err != nil {
		return nil
	}

	children, err := rootDir.GetChildren()
	if err != nil {
		return nil
	}

	// Look for directories starting with "casper"
	for _, child := range children {
		name := child.Name()
		if !strings.HasPrefix(strings.ToLower(name), "casper") {
			continue
		}

		// Try to find kernel and initrd in this casper variant directory
		dirPath := "/" + name
		if found := findKernelInitrd(img, dirPath, "vmlinuz", "initrd"); found != nil {
			return found
		}
	}

	return nil
}

// findInstallVariant searches for Debian installer directories (install, install.amd, etc.)
func findInstallVariant(img *iso9660.Image) *BootFiles {
	rootDir, err := findFile(img, "/")
	if err != nil {
		return nil
	}

	children, err := rootDir.GetChildren()
	if err != nil {
		return nil
	}

	// Look for directories starting with "install"
	for _, child := range children {
		name := child.Name()
		nameLower := strings.ToLower(name)
		if !strings.HasPrefix(nameLower, "install") {
			continue
		}

		// Try to find kernel and initrd in this install variant directory
		dirPath := "/" + name
		if found := findKernelInitrd(img, dirPath, "vmlinuz", "initrd"); found != nil {
			return found
		}
	}

	return nil
}

// findKernelInitrd searches for kernel and initrd files in a directory with pattern matching
func findKernelInitrd(img *iso9660.Image, dir, kernelPrefix, initrdPrefix string) *BootFiles {
	dirFile, err := findFile(img, dir)
	if err != nil || !dirFile.IsDir() {
		log.Printf("Directory %s not found or not a directory", dir)
		return nil
	}

	children, err := dirFile.GetChildren()
	if err != nil {
		log.Printf("Failed to get children of %s: %v", dir, err)
		return nil
	}

	log.Printf("Searching in %s for kernel pattern '%s' and initrd pattern '%s'", dir, kernelPrefix, initrdPrefix)
	var kernel, initrd, squashfs string
	var fileNames []string
	for _, child := range children {
		name := child.Name()
		fileNames = append(fileNames, name)
		nameLower := strings.ToLower(name)
		kernelLower := strings.ToLower(kernelPrefix)
		initrdLower := strings.ToLower(initrdPrefix)

		// Match files that start with OR contain the pattern
		if kernel == "" && (strings.HasPrefix(nameLower, kernelLower) || strings.Contains(nameLower, kernelLower)) {
			kernel = filepath.Join(dir, name)
			log.Printf("Found kernel: %s", kernel)
		}
		if initrd == "" && (strings.HasPrefix(nameLower, initrdLower) || strings.Contains(nameLower, initrdLower)) {
			initrd = filepath.Join(dir, name)
			log.Printf("Found initrd: %s", initrd)
		}
		// Look for filesystem.squashfs in the same directory
		if squashfs == "" && strings.Contains(nameLower, "filesystem.squashfs") {
			squashfs = filepath.Join(dir, name)
			log.Printf("Found squashfs: %s", squashfs)
		}
		if kernel != "" && initrd != "" {
			// Return basic boot files, let caller set distro and boot params
			return &BootFiles{
				Kernel:       kernel,
				Initrd:       initrd,
				SquashfsPath: squashfs,
			}
		}
	}

	log.Printf("Files in %s: %v", dir, fileNames)
	log.Printf("No matching kernel/initrd found (kernel='%s', initrd='%s')", kernel, initrd)
	return nil
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
			BootParams: "", // inst.repo will be set by menu template
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
			BootParams: "", // inst.repo will be set by menu template
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
			BootParams: "archisobasedir=arch ", // archiso_http_srv will be appended by menu template
		}, nil
	}

	return nil, fmt.Errorf("not Arch Linux")
}

// detectFreeBSD detects FreeBSD ISOs
func (e *Extractor) detectFreeBSD(img *iso9660.Image) (*BootFiles, error) {
	// FreeBSD uses a different boot approach - typically boots via memdisk or direct ISO
	// Common paths for FreeBSD kernel and loader
	paths := []struct {
		kernel     string
		initrd     string
		bootParams string
	}{
		{"/boot/kernel/kernel", "/boot/mfsroot.gz", ""},
		{"/boot/kernel/kernel", "/boot/kernel/kernel", ""}, // Some versions
	}

	for _, p := range paths {
		if fileExists(img, p.kernel) {
			// FreeBSD may not always have a separate initrd
			initrd := p.initrd
			if !fileExists(img, initrd) {
				// Use kernel as initrd placeholder (FreeBSD boot is different)
				initrd = p.kernel
			}
			return &BootFiles{
				Kernel:     p.kernel,
				Initrd:     initrd,
				Distro:     "freebsd",
				BootParams: "", // FreeBSD-specific params will be set by template
			}, nil
		}
	}

	return nil, fmt.Errorf("not FreeBSD")
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

// detectNixOS detects NixOS ISOs
func (e *Extractor) detectNixOS(img *iso9660.Image) (*BootFiles, error) {
	// NixOS stores kernel and initrd in /boot/nix/store/<hash>-linux-<version>/bzImage
	// and /boot/nix/store/<hash>-initrd-linux-<version>/initrd
	// We need to search for these files in the nix store subdirectories

	storeDir, err := findFile(img, "/boot/nix/store")
	if err != nil {
		return nil, fmt.Errorf("not NixOS: /boot/nix/store not found")
	}

	children, err := storeDir.GetChildren()
	if err != nil {
		return nil, fmt.Errorf("not NixOS: failed to read /boot/nix/store")
	}

	var kernel, initrd string

	// Search through store subdirectories for bzImage and initrd
	for _, child := range children {
		if !child.IsDir() {
			continue
		}

		name := child.Name()
		childPath := "/boot/nix/store/" + name

		// Look for kernel (bzImage in linux-* directories)
		if strings.Contains(strings.ToLower(name), "linux-") && kernel == "" {
			bzImagePath := childPath + "/bzImage"
			if fileExists(img, bzImagePath) {
				kernel = bzImagePath
				log.Printf("Found NixOS kernel: %s", kernel)
			}
		}

		// Look for initrd (in initrd-linux-* directories)
		if strings.Contains(strings.ToLower(name), "initrd-linux-") && initrd == "" {
			initrdPath := childPath + "/initrd"
			if fileExists(img, initrdPath) {
				initrd = initrdPath
				log.Printf("Found NixOS initrd: %s", initrd)
			}
		}

		if kernel != "" && initrd != "" {
			return &BootFiles{
				Kernel:     kernel,
				Initrd:     initrd,
				Distro:     "nixos",
				BootParams: "init=/nix/store/*/init ", // NixOS init path
			}, nil
		}
	}

	return nil, fmt.Errorf("not NixOS: kernel or initrd not found in /boot/nix/store")
}

// detectWindows detects Windows ISOs and extracts boot files for wimboot
func (e *Extractor) detectWindows(img *iso9660.Image) (*BootFiles, error) {
	// Check for Windows boot files
	// Try multiple path variations (case variations, different locations)
	bcdPaths := []string{
		"/boot/bcd",
		"/BOOT/BCD",
		"/efi/microsoft/boot/bcd",
		"/EFI/MICROSOFT/BOOT/BCD",
		"/efi/boot/bootx64.efi", // UEFI boot
	}

	bootSdiPaths := []string{
		"/boot/boot.sdi",
		"/BOOT/BOOT.SDI",
	}

	bootWimPaths := []string{
		"/sources/boot.wim",
		"/SOURCES/BOOT.WIM",
	}

	// Find BCD
	var bcdPath string
	for _, path := range bcdPaths {
		log.Printf("Checking for BCD at: %s", path)
		if fileExists(img, path) {
			bcdPath = path
			log.Printf("Found BCD at: %s", path)
			break
		}
	}

	// Find boot.sdi
	var bootSdiPath string
	for _, path := range bootSdiPaths {
		log.Printf("Checking for boot.sdi at: %s", path)
		if fileExists(img, path) {
			bootSdiPath = path
			log.Printf("Found boot.sdi at: %s", path)
			break
		}
	}

	// Find boot.wim
	var bootWimPath string
	for _, path := range bootWimPaths {
		log.Printf("Checking for boot.wim at: %s", path)
		if fileExists(img, path) {
			bootWimPath = path
			log.Printf("Found boot.wim at: %s", path)
			break
		}
	}

	// Check if we found all required files
	if bcdPath != "" && bootSdiPath != "" && bootWimPath != "" {
		log.Printf("Detected Windows ISO - BCD: %s, boot.sdi: %s, boot.wim: %s", bcdPath, bootSdiPath, bootWimPath)
		return &BootFiles{
			Kernel:     bcdPath,        // We'll use Kernel field for BCD
			Initrd:     bootSdiPath,    // Initrd field for boot.sdi
			Distro:     "windows",
			BootParams: bootWimPath,    // BootParams field for boot.wim path
		}, nil
	}

	return nil, fmt.Errorf("not Windows ISO (found: BCD=%v, boot.sdi=%v, boot.wim=%v)", bcdPath != "", bootSdiPath != "", bootWimPath != "")
}

// cacheBootFiles copies boot files to ISO subdirectory and extracts full ISO contents for HTTP boot
func (e *Extractor) cacheBootFiles(files *BootFiles, img *iso9660.Image, isoPath string) error {
	// Create subdirectory based on ISO filename within the isos directory
	isoBase := strings.TrimSuffix(filepath.Base(isoPath), filepath.Ext(isoPath))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	if err := os.MkdirAll(bootFilesDir, 0755); err != nil {
		return fmt.Errorf("failed to create boot files subdirectory: %w", err)
	}

	// Handle Windows ISOs differently
	if files.Distro == "windows" {
		// For Windows, extract BCD, boot.sdi, and boot.wim
		// Kernel field contains BCD path
		bcdDest := filepath.Join(bootFilesDir, "bcd")
		if err := extractFile(img, files.Kernel, bcdDest); err != nil {
			return fmt.Errorf("failed to extract BCD: %w", err)
		}
		files.Kernel = bcdDest

		// Initrd field contains boot.sdi path
		bootSdiDest := filepath.Join(bootFilesDir, "boot.sdi")
		if err := extractFile(img, files.Initrd, bootSdiDest); err != nil {
			return fmt.Errorf("failed to extract boot.sdi: %w", err)
		}
		files.Initrd = bootSdiDest

		// BootParams field contains boot.wim path
		bootWimDest := filepath.Join(bootFilesDir, "boot.wim")
		if err := extractFile(img, files.BootParams, bootWimDest); err != nil {
			return fmt.Errorf("failed to extract boot.wim: %w", err)
		}
		// Store boot.wim path in BootParams
		files.BootParams = bootWimDest

		log.Printf("Extracted Windows boot files: BCD, boot.sdi, boot.wim to %s", bootFilesDir)
		return nil
	}

	// Linux distros: Extract kernel and initrd
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

	// Extract full ISO contents for HTTP boot (for distributions that need it)
	// Create an "iso" subdirectory to hold the extracted contents
	extractedDir := filepath.Join(bootFilesDir, "iso")
	if err := os.MkdirAll(extractedDir, 0755); err != nil {
		return fmt.Errorf("failed to create extracted ISO directory: %w", err)
	}

	// Extract the entire ISO contents using custom extraction to handle errors gracefully
	log.Printf("Extracting full ISO contents to %s", extractedDir)
	if err := e.extractISOContents(img, extractedDir); err != nil {
		return fmt.Errorf("failed to extract full ISO contents: %w", err)
	}

	files.ExtractedDir = extractedDir

	return nil
}

// extractISOContents extracts ISO contents with error handling for problematic files
func (e *Extractor) extractISOContents(img *iso9660.Image, destDir string) error {
	root, err := img.RootDir()
	if err != nil {
		return fmt.Errorf("failed to get root directory: %w", err)
	}

	return e.extractDirectory(root, destDir, "/")
}

// extractDirectory recursively extracts a directory from the ISO
func (e *Extractor) extractDirectory(dir *iso9660.File, destPath, isoPath string) error {
	children, err := dir.GetChildren()
	if err != nil {
		log.Printf("Warning: failed to get children of %s: %v (skipping)", isoPath, err)
		return nil
	}

	for _, child := range children {
		name := child.Name()
		if name == "" || name == "." || name == ".." {
			continue
		}

		childISOPath := filepath.Join(isoPath, name)
		childDestPath := filepath.Join(destPath, name)

		// Sanitize filename to avoid issues with invalid characters
		safeName := sanitizeFilename(name)
		if safeName != name {
			log.Printf("Warning: sanitized filename from '%s' to '%s'", name, safeName)
			childDestPath = filepath.Join(destPath, safeName)
		}

		if child.IsDir() {
			// Create directory
			if err := os.MkdirAll(childDestPath, 0755); err != nil {
				log.Printf("Warning: failed to create directory %s: %v (skipping)", childDestPath, err)
				continue
			}

			// Recursively extract subdirectory
			if err := e.extractDirectory(child, childDestPath, childISOPath); err != nil {
				log.Printf("Warning: error extracting directory %s: %v (continuing)", childISOPath, err)
			}
		} else {
			// Extract file
			if err := e.extractFile(child, childDestPath, childISOPath); err != nil {
				log.Printf("Warning: failed to extract file %s: %v (skipping)", childISOPath, err)
				continue
			}
		}
	}

	return nil
}

// extractFile extracts a single file from the ISO
func (e *Extractor) extractFile(file *iso9660.File, destPath, isoPath string) error {
	reader := file.Reader()

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, reader); err != nil {
		os.Remove(destPath) // Clean up partial file
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	return nil
}

// sanitizeFilename removes or replaces invalid characters in filenames
func sanitizeFilename(name string) string {
	// Replace invalid characters with underscores
	invalid := []string{"\x00", "<", ">", ":", "\"", "|", "?", "*"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}

	// Remove control characters
	var cleaned strings.Builder
	for _, r := range result {
		if r >= 32 || r == '\t' || r == '\n' {
			cleaned.WriteRune(r)
		}
	}

	return cleaned.String()
}

// GetCachedBootFiles returns cached boot files if they exist
func (e *Extractor) GetCachedBootFiles(isoFilename string) (*BootFiles, error) {
	isoBase := strings.TrimSuffix(isoFilename, filepath.Ext(isoFilename))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	kernelPath := filepath.Join(bootFilesDir, "vmlinuz")
	initrdPath := filepath.Join(bootFilesDir, "initrd")
	extractedDir := filepath.Join(bootFilesDir, "iso")

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
		Kernel:       kernelPath,
		Initrd:       initrdPath,
		Distro:       distro,
		BootParams:   bootParams,
		ExtractedDir: extractedDir,
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

		// Debug: list all children names
		var childNames []string
		for _, child := range children {
			childNames = append(childNames, child.Name())
		}
		log.Printf("Looking for '%s' in directory, found children: %v", part, childNames)

		// Find matching child
		found := false
		for _, child := range children {
			// Case-insensitive comparison (ISO9660 is typically uppercase)
			if strings.EqualFold(child.Name(), part) {
				log.Printf("Matched '%s' with '%s'", part, child.Name())
				current = child
				found = true
				break
			}
		}

		if !found {
			log.Printf("Path component '%s' not found in %v", part, childNames)
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
