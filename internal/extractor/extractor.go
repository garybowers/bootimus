package extractor

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"bootimus/internal/udf"

	"github.com/kdomanski/iso9660"
)

type BootFiles struct {
	Kernel          string
	Initrd          string
	BootParams      string
	Distro          string
	ExtractedDir    string
	SquashfsPath    string
	NetbootRequired bool
	NetbootURL      string
	InstallWim      string
}

type Extractor struct {
	dataDir string
}

func New(dataDir string) (*Extractor, error) {
	return &Extractor{
		dataDir: dataDir,
	}, nil
}

func (e *Extractor) Extract(isoPath string) (*BootFiles, error) {
	isUDF, err := detectISOFormat(isoPath)
	if err != nil {
		log.Printf("Warning: failed to detect ISO format, will try both methods: %v", err)
	}

	if isUDF {
		log.Printf("Detected UDF format, using UDF reader")
		bootFiles, err := e.extractViaUDF(isoPath)
		if err == nil {
			return bootFiles, nil
		}
		log.Printf("UDF extraction failed (%v), trying ISO9660 as fallback", err)
		bootFiles, err = e.extractViaISO9660(isoPath)
		if err != nil {
			return nil, fmt.Errorf("both UDF and ISO9660 extraction failed: %w", err)
		}
		return bootFiles, nil
	}

	bootFiles, err := e.extractViaISO9660(isoPath)
	if err == nil {
		return bootFiles, nil
	}

	log.Printf("ISO9660 extraction failed (%v), trying UDF method", err)

	bootFiles, err = e.extractViaUDF(isoPath)
	if err != nil {
		return nil, fmt.Errorf("both ISO9660 and UDF extraction failed: %w", err)
	}

	return bootFiles, nil
}

func detectISOFormat(isoPath string) (bool, error) {
	f, err := os.Open(isoPath)
	if err != nil {
		return false, fmt.Errorf("failed to open ISO: %w", err)
	}
	defer f.Close()

	var hasUDF, hasISO9660 bool

	anchorOffset := int64(256 * 2048)
	anchorBuf := make([]byte, 16)
	_, err = f.ReadAt(anchorBuf, anchorOffset)
	if err == nil {
		tagIdentifier := uint16(anchorBuf[0]) | uint16(anchorBuf[1])<<8
		if tagIdentifier == 0x0002 {
			hasUDF = true
		}
	}

	pvdOffset := int64(16 * 2048)
	pvdBuf := make([]byte, 6)
	_, err = f.ReadAt(pvdBuf, pvdOffset)
	if err == nil {
		if pvdBuf[0] == 0x01 && string(pvdBuf[1:6]) == "CD001" {
			hasISO9660 = true
		}
	}

	if hasUDF && hasISO9660 {
		log.Printf("Detected hybrid ISO (both UDF and ISO9660), preferring UDF")
		return true, nil
	} else if hasUDF {
		log.Printf("Detected UDF format (anchor descriptor found at sector 256)")
		return true, nil
	} else if hasISO9660 {
		log.Printf("Detected ISO9660 format (primary volume descriptor found at sector 16)")
		return false, nil
	}

	log.Printf("Could not definitively detect format, defaulting to ISO9660")
	return false, nil
}

func (e *Extractor) extractViaISO9660(isoPath string) (*BootFiles, error) {
	f, err := os.Open(isoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ISO: %w", err)
	}
	defer f.Close()

	img, err := iso9660.OpenImage(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read ISO image: %w", err)
	}

	reader := &ISO9660Reader{img: img, extract: e}
	bootFiles, err := e.detectAndExtractUnified(reader, isoPath)
	if err != nil {
		return nil, err
	}

	return bootFiles, nil
}

func (e *Extractor) detectAndExtract(img *iso9660.Image, isoPath string) (*BootFiles, error) {
	distroName := detectDistroName(img, isoPath)

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
			if distroName != "" {
				files.Distro = distroName
			}
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

func detectDistroName(img *iso9660.Image, isoPath string) string {
	filename := strings.ToLower(filepath.Base(isoPath))

	distroPatterns := map[string]string{
		"windows":     "windows",
		"win10":       "windows",
		"win11":       "windows",
		"win7":        "windows",
		"win8":        "windows",
		"server2022":  "windows",
		"server2019":  "windows",
		"server2016":  "windows",
		"popos":       "popos",
		"pop-os":      "popos",
		"pop_os":      "popos",
		"manjaro":     "manjaro",
		"mint":        "mint",
		"linuxmint":   "mint",
		"elementary":  "elementary",
		"zorin":       "zorin",
		"ubuntu":      "ubuntu",
		"debian":      "debian",
		"arch":        "arch",
		"fedora":      "fedora",
		"centos":      "centos",
		"rocky":       "rocky",
		"alma":        "alma",
		"kali":        "kali",
		"parrot":      "parrot",
		"tails":       "tails",
		"opensuse":    "opensuse",
		"freebsd":     "freebsd",
		"nixos":       "nixos",
		"endeavouros": "endeavouros",
		"garuda":      "garuda",
		"arco":        "arco",
	}

	for pattern, distro := range distroPatterns {
		if strings.Contains(filename, pattern) {
			return distro
		}
	}

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

func (e *Extractor) detectUbuntuDebian(img *iso9660.Image) (*BootFiles, error) {
	log.Printf("Checking for Ubuntu/Debian ISO...")

	log.Printf("Checking /casper with wildcard matching...")
	if found := findKernelInitrd(img, "/casper", "vmlinuz", "initrd"); found != nil {
		log.Printf("Found kernel/initrd in /casper using wildcard matching")
		found.Distro = "ubuntu"
		found.BootParams = "boot=casper fetch= "
		return found, nil
	}

	if found := findCasperVariant(img); found != nil {
		found.Distro = "ubuntu"
		found.BootParams = "boot=casper fetch= "
		return found, nil
	}

	if found := findInstallVariant(img); found != nil {
		found.Distro = "debian"
		found.BootParams = ""
		found.NetbootRequired = true
		found.NetbootURL = "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"
		return found, nil
	}

	paths := []struct {
		kernel     string
		initrd     string
		distro     string
		bootParams string
	}{
		{"/casper/vmlinuz", "/casper/initrd", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz", "/casper/initrd.lz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz", "/casper/initrd.gz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz.efi", "/casper/initrd.lz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz.efi", "/casper/initrd", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/casper/vmlinuz.efi", "/casper/initrd.gz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled "},
		{"/install/vmlinuz", "/install/initrd.gz", "ubuntu-installer", ""},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "ubuntu-installer", ""},
		{"/install/vmlinuz", "/install/initrd.gz", "debian", ""},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "debian", ""},
		{"/live/vmlinuz", "/live/initrd.img", "debian", "boot=live fetch= "},
		{"/live/vmlinuz1", "/live/initrd1.img", "debian", "boot=live fetch= "},
		{"/live/vmlinuz-*", "/live/initrd.img-*", "debian", "boot=live fetch= "},
	}

	for _, p := range paths {
		if strings.Contains(p.kernel, "*") {
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
			if p.distro == "debian" && (strings.Contains(p.kernel, "/install") || strings.Contains(p.kernel, "/install.amd")) {
				bootFiles.NetbootRequired = true
				bootFiles.NetbootURL = "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"
			}
			if p.distro == "ubuntu-installer" && (strings.Contains(p.kernel, "/install") || strings.Contains(p.kernel, "/install.amd")) {
				bootFiles.Distro = "ubuntu"
				bootFiles.NetbootRequired = true
				bootFiles.NetbootURL = "http://archive.ubuntu.com/ubuntu/dists/noble/main/installer-amd64/current/legacy-images/netboot/netboot.tar.gz"
			}
			return bootFiles, nil
		}
	}

	return nil, fmt.Errorf("kernel/initrd not found in common Ubuntu/Debian paths")
}

func findCasperVariant(img *iso9660.Image) *BootFiles {
	rootDir, err := findFile(img, "/")
	if err != nil {
		return nil
	}

	children, err := rootDir.GetChildren()
	if err != nil {
		return nil
	}

	for _, child := range children {
		name := child.Name()
		if !strings.HasPrefix(strings.ToLower(name), "casper") {
			continue
		}

		dirPath := "/" + name
		if found := findKernelInitrd(img, dirPath, "vmlinuz", "initrd"); found != nil {
			return found
		}
	}

	return nil
}

func findInstallVariant(img *iso9660.Image) *BootFiles {
	rootDir, err := findFile(img, "/")
	if err != nil {
		return nil
	}

	children, err := rootDir.GetChildren()
	if err != nil {
		return nil
	}

	for _, child := range children {
		name := child.Name()
		nameLower := strings.ToLower(name)
		if !strings.HasPrefix(nameLower, "install") {
			continue
		}

		dirPath := "/" + name
		if found := findKernelInitrd(img, dirPath, "vmlinuz", "initrd"); found != nil {
			return found
		}
	}

	return nil
}

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

		if kernel == "" && (strings.HasPrefix(nameLower, kernelLower) || strings.Contains(nameLower, kernelLower)) {
			kernel = filepath.Join(dir, name)
			log.Printf("Found kernel: %s", kernel)
		}
		if initrd == "" && (strings.HasPrefix(nameLower, initrdLower) || strings.Contains(nameLower, initrdLower)) {
			initrd = filepath.Join(dir, name)
			log.Printf("Found initrd: %s", initrd)
		}
		if squashfs == "" && strings.Contains(nameLower, "filesystem.squashfs") {
			squashfs = filepath.Join(dir, name)
			log.Printf("Found squashfs: %s", squashfs)
		}
		if kernel != "" && initrd != "" {
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

func (e *Extractor) detectFedoraRHEL(img *iso9660.Image) (*BootFiles, error) {
	kernel := "/images/pxeboot/vmlinuz"
	initrd := "/images/pxeboot/initrd.img"

	if fileExists(img, kernel) && fileExists(img, initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "fedora",
			BootParams: "",
		}, nil
	}

	return nil, fmt.Errorf("not Fedora/RHEL")
}

func (e *Extractor) detectCentOS(img *iso9660.Image) (*BootFiles, error) {
	kernel := "/images/pxeboot/vmlinuz"
	initrd := "/images/pxeboot/initrd.img"

	if fileExists(img, kernel) && fileExists(img, initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "centos",
			BootParams: "",
		}, nil
	}

	return nil, fmt.Errorf("not CentOS/Rocky/Alma")
}

func (e *Extractor) detectArch(img *iso9660.Image) (*BootFiles, error) {
	kernel := "/arch/boot/x86_64/vmlinuz-linux"
	initrd := "/arch/boot/x86_64/initramfs-linux.img"

	if fileExists(img, kernel) && fileExists(img, initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "arch",
			BootParams: "archisobasedir=arch ",
		}, nil
	}

	return nil, fmt.Errorf("not Arch Linux")
}

func (e *Extractor) detectFreeBSD(img *iso9660.Image) (*BootFiles, error) {
	paths := []struct {
		kernel     string
		initrd     string
		bootParams string
	}{
		{"/boot/kernel/kernel", "/boot/mfsroot.gz", ""},
		{"/boot/kernel/kernel", "/boot/kernel/kernel", ""},
	}

	for _, p := range paths {
		if fileExists(img, p.kernel) {
			initrd := p.initrd
			if !fileExists(img, initrd) {
				initrd = p.kernel
			}
			return &BootFiles{
				Kernel:     p.kernel,
				Initrd:     initrd,
				Distro:     "freebsd",
				BootParams: "",
			}, nil
		}
	}

	return nil, fmt.Errorf("not FreeBSD")
}

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

func (e *Extractor) detectNixOS(img *iso9660.Image) (*BootFiles, error) {
	storeDir, err := findFile(img, "/boot/nix/store")
	if err != nil {
		return nil, fmt.Errorf("not NixOS: /boot/nix/store not found")
	}

	children, err := storeDir.GetChildren()
	if err != nil {
		return nil, fmt.Errorf("not NixOS: failed to read /boot/nix/store")
	}

	var kernel, initrd string

	for _, child := range children {
		if !child.IsDir() {
			continue
		}

		name := child.Name()
		childPath := "/boot/nix/store/" + name

		if strings.Contains(strings.ToLower(name), "linux-") && kernel == "" {
			bzImagePath := childPath + "/bzImage"
			if fileExists(img, bzImagePath) {
				kernel = bzImagePath
				log.Printf("Found NixOS kernel: %s", kernel)
			}
		}

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
				BootParams: "init=/nix/store/*/init ",
			}, nil
		}
	}

	return nil, fmt.Errorf("not NixOS: kernel or initrd not found in /boot/nix/store")
}

func (e *Extractor) detectWindows(img *iso9660.Image) (*BootFiles, error) {
	bcdPaths := []string{
		"/boot/bcd",
		"/BOOT/BCD",
		"/efi/microsoft/boot/bcd",
		"/EFI/MICROSOFT/BOOT/BCD",
		"/efi/boot/bootx64.efi",
	}

	bootSdiPaths := []string{
		"/boot/boot.sdi",
		"/BOOT/BOOT.SDI",
	}

	bootWimPaths := []string{
		"/sources/boot.wim",
		"/SOURCES/BOOT.WIM",
	}

	var bcdPath string
	for _, path := range bcdPaths {
		log.Printf("Checking for BCD at: %s", path)
		if fileExists(img, path) {
			bcdPath = path
			log.Printf("Found BCD at: %s", path)
			break
		}
	}

	var bootSdiPath string
	for _, path := range bootSdiPaths {
		log.Printf("Checking for boot.sdi at: %s", path)
		if fileExists(img, path) {
			bootSdiPath = path
			log.Printf("Found boot.sdi at: %s", path)
			break
		}
	}

	var bootWimPath string
	for _, path := range bootWimPaths {
		log.Printf("Checking for boot.wim at: %s", path)
		if fileExists(img, path) {
			bootWimPath = path
			log.Printf("Found boot.wim at: %s", path)
			break
		}
	}

	if bcdPath != "" && bootSdiPath != "" && bootWimPath != "" {
		log.Printf("Detected Windows ISO - BCD: %s, boot.sdi: %s, boot.wim: %s", bcdPath, bootSdiPath, bootWimPath)
		return &BootFiles{
			Kernel:     bcdPath,
			Initrd:     bootSdiPath,
			Distro:     "windows",
			BootParams: bootWimPath,
		}, nil
	}

	return nil, fmt.Errorf("not Windows ISO (found: BCD=%v, boot.sdi=%v, boot.wim=%v)", bcdPath != "", bootSdiPath != "", bootWimPath != "")
}

func (e *Extractor) cacheBootFiles(files *BootFiles, img *iso9660.Image, isoPath string) error {
	isoBase := strings.TrimSuffix(filepath.Base(isoPath), filepath.Ext(isoPath))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	if err := os.MkdirAll(bootFilesDir, 0755); err != nil {
		return fmt.Errorf("failed to create boot files subdirectory: %w", err)
	}

	if files.Distro == "windows" {
		bcdDest := filepath.Join(bootFilesDir, "bcd")
		if err := extractFile(img, files.Kernel, bcdDest); err != nil {
			return fmt.Errorf("failed to extract BCD: %w", err)
		}
		files.Kernel = bcdDest

		bootSdiDest := filepath.Join(bootFilesDir, "boot.sdi")
		if err := extractFile(img, files.Initrd, bootSdiDest); err != nil {
			return fmt.Errorf("failed to extract boot.sdi: %w", err)
		}
		files.Initrd = bootSdiDest

		bootWimDest := filepath.Join(bootFilesDir, "boot.wim")
		if err := extractFile(img, files.BootParams, bootWimDest); err != nil {
			return fmt.Errorf("failed to extract boot.wim: %w", err)
		}
		files.BootParams = bootWimDest

		log.Printf("Extracted Windows boot files: BCD, boot.sdi, boot.wim to %s", bootFilesDir)
		return nil
	}

	kernelDest := filepath.Join(bootFilesDir, "vmlinuz")
	if err := extractFile(img, files.Kernel, kernelDest); err != nil {
		return fmt.Errorf("failed to extract kernel: %w", err)
	}
	files.Kernel = kernelDest

	initrdDest := filepath.Join(bootFilesDir, "initrd")
	if err := extractFile(img, files.Initrd, initrdDest); err != nil {
		return fmt.Errorf("failed to extract initrd: %w", err)
	}
	files.Initrd = initrdDest

	extractedDir := filepath.Join(bootFilesDir, "iso")
	if err := os.MkdirAll(extractedDir, 0755); err != nil {
		return fmt.Errorf("failed to create extracted ISO directory: %w", err)
	}

	log.Printf("Extracting full ISO contents to %s", extractedDir)
	if err := e.extractISOContents(img, extractedDir); err != nil {
		return fmt.Errorf("failed to extract full ISO contents: %w", err)
	}

	files.ExtractedDir = extractedDir

	return nil
}

func (e *Extractor) extractISOContents(img *iso9660.Image, destDir string) error {
	root, err := img.RootDir()
	if err != nil {
		return fmt.Errorf("failed to get root directory: %w", err)
	}

	return e.extractDirectory(root, destDir, "/")
}

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

		safeName := sanitizeFilename(name)
		if safeName != name {
			log.Printf("Warning: sanitized filename from '%s' to '%s'", name, safeName)
			childDestPath = filepath.Join(destPath, safeName)
		}

		if child.IsDir() {
			if err := os.MkdirAll(childDestPath, 0755); err != nil {
				log.Printf("Warning: failed to create directory %s: %v (skipping)", childDestPath, err)
				continue
			}

			if err := e.extractDirectory(child, childDestPath, childISOPath); err != nil {
				log.Printf("Warning: error extracting directory %s: %v (continuing)", childISOPath, err)
			}
		} else {
			if err := e.extractFile(child, childDestPath, childISOPath); err != nil {
				log.Printf("Warning: failed to extract file %s: %v (skipping)", childISOPath, err)
				continue
			}
		}
	}

	return nil
}

func (e *Extractor) extractFile(file *iso9660.File, destPath, isoPath string) error {
	reader := file.Reader()

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, reader); err != nil {
		os.Remove(destPath)
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	return nil
}

func sanitizeFilename(name string) string {
	invalid := []string{"\x00", "<", ">", ":", "\"", "|", "?", "*"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}

	var cleaned strings.Builder
	for _, r := range result {
		if r >= 32 || r == '\t' || r == '\n' {
			cleaned.WriteRune(r)
		}
	}

	return cleaned.String()
}

func (e *Extractor) GetCachedBootFiles(isoFilename string) (*BootFiles, error) {
	isoBase := strings.TrimSuffix(isoFilename, filepath.Ext(isoFilename))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	kernelPath := filepath.Join(bootFilesDir, "vmlinuz")
	initrdPath := filepath.Join(bootFilesDir, "initrd")
	extractedDir := filepath.Join(bootFilesDir, "iso")

	if !fileExistsOnDisk(kernelPath) || !fileExistsOnDisk(initrdPath) {
		return nil, fmt.Errorf("cached files not found")
	}

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

func (e *Extractor) SaveMetadata(isoFilename string, files *BootFiles) error {
	isoBase := strings.TrimSuffix(isoFilename, filepath.Ext(isoFilename))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)
	metadataPath := filepath.Join(bootFilesDir, "metadata.txt")

	metadata := fmt.Sprintf("distro=%s\nboot_params=%s\n", files.Distro, files.BootParams)
	return os.WriteFile(metadataPath, []byte(metadata), 0644)
}

func fileExists(img *iso9660.Image, path string) bool {
	_, err := findFile(img, path)
	return err == nil
}

func fileExistsOnDisk(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func findFile(img *iso9660.Image, path string) (*iso9660.File, error) {
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

		children, err := current.GetChildren()
		if err != nil {
			return nil, fmt.Errorf("failed to get children: %w", err)
		}

		var childNames []string
		for _, child := range children {
			childNames = append(childNames, child.Name())
		}
		log.Printf("Looking for '%s' in directory, found children: %v", part, childNames)

		found := false
		for _, child := range children {
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

		if i == len(parts)-1 {
			return current, nil
		}

		if !current.IsDir() {
			return nil, fmt.Errorf("not a directory: %s", part)
		}
	}

	return current, nil
}

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

func (e *Extractor) extractViaUDF(isoPath string) (*BootFiles, error) {
	log.Printf("Attempting UDF extraction for %s", isoPath)

	f, err := os.Open(isoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ISO: %w", err)
	}
	defer f.Close()

	udfReader := udf.NewReader(f)

	_, err = udfReader.Root()
	if err != nil {
		return nil, fmt.Errorf("failed to read UDF filesystem: %w", err)
	}

	log.Printf("Successfully opened UDF filesystem")

	reader := &UDFReader{reader: udfReader, extract: e}
	bootFiles, err := e.detectAndExtractUnified(reader, isoPath)
	if err != nil {
		return nil, err
	}

	return bootFiles, nil
}

func (e *Extractor) detectAndExtractUnified(reader FileSystemReader, isoPath string) (*BootFiles, error) {
	distroName := detectDistroNameUnified(reader, isoPath)

	detectors := []struct {
		name     string
		detector func(FileSystemReader) (*BootFiles, error)
	}{
		{"Windows", e.detectWindowsUnified},
		{"Ubuntu/Debian Family", e.detectUbuntuDebianUnified},
		{"Arch Linux Family", e.detectArchUnified},
		{"Fedora/RHEL Family", e.detectFedoraRHELUnified},
		{"CentOS/Rocky/Alma Family", e.detectCentOSUnified},
		{"FreeBSD", e.detectFreeBSDUnified},
		{"OpenSUSE", e.detectOpenSUSEUnified},
		{"NixOS", e.detectNixOSUnified},
	}

	var errors []string
	for _, d := range detectors {
		if files, err := d.detector(reader); err == nil && files != nil {
			if distroName != "" {
				files.Distro = distroName
			}
			if err := e.cacheBootFilesUnified(files, reader, isoPath); err != nil {
				return nil, err
			}
			return files, nil
		} else {
			errors = append(errors, fmt.Sprintf("%s: %v", d.name, err))
		}
	}

	return nil, fmt.Errorf("unsupported distribution or unable to find boot files (tried: %s)", strings.Join(errors, "; "))
}

func findFileUDF(reader *udf.Reader, path string) (*udf.File, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		files, err := reader.Root()
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("empty root directory")
		}
		return files[0], nil
	}

	parts := strings.Split(path, "/")
	files, err := reader.Root()
	if err != nil {
		return nil, err
	}

	for _, part := range parts {
		if part == "" {
			continue
		}

		var found *udf.File
		for _, f := range files {
			if strings.EqualFold(f.Name(), part) {
				found = f
				break
			}
		}

		if found == nil {
			return nil, fmt.Errorf("path not found: %s (missing: %s)", path, part)
		}

		if part == parts[len(parts)-1] {
			return found, nil
		}

		if !found.IsDir() {
			return nil, fmt.Errorf("not a directory: %s", part)
		}

		files, err = found.ReadDir()
		if err != nil {
			return nil, fmt.Errorf("failed to read directory %s: %w", part, err)
		}
	}

	return nil, fmt.Errorf("path not found: %s", path)
}

func fileExistsUDF(reader *udf.Reader, path string) bool {
	_, err := findFileUDF(reader, path)
	return err == nil
}

func extractFileUDF(reader *udf.Reader, isoPath, destPath string) error {
	file, err := findFileUDF(reader, isoPath)
	if err != nil {
		return fmt.Errorf("file not found in UDF: %s: %w", isoPath, err)
	}

	if file.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", isoPath)
	}

	fileReader, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dest.Close()

	_, err = io.Copy(dest, fileReader)
	return err
}

func (e *Extractor) extractUDFContents(reader *udf.Reader, destDir string) error {
	root, err := reader.Root()
	if err != nil {
		return fmt.Errorf("failed to get root directory: %w", err)
	}

	for _, file := range root {
		if err := e.extractUDFFile(reader, file, destDir, file.Name()); err != nil {
			log.Printf("Warning: failed to extract %s: %v", file.Name(), err)
		}
	}

	return nil
}

func (e *Extractor) extractUDFFile(reader *udf.Reader, file *udf.File, destDir, relativePath string) error {
	destPath := filepath.Join(destDir, relativePath)

	if file.IsDir() {
		if err := os.MkdirAll(destPath, 0755); err != nil {
			return err
		}

		children, err := file.ReadDir()
		if err != nil {
			return err
		}

		for _, child := range children {
			childPath := filepath.Join(relativePath, child.Name())
			if err := e.extractUDFFile(reader, child, destDir, childPath); err != nil {
				log.Printf("Warning: failed to extract %s: %v", childPath, err)
			}
		}
	} else {
		fileReader, err := file.Open()
		if err != nil {
			return err
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer outFile.Close()

		if _, err := io.Copy(outFile, fileReader); err != nil {
			os.Remove(destPath)
			return err
		}
	}

	return nil
}
