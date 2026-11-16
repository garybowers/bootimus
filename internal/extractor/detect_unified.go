package extractor

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bootimus/internal/bsdboot"
)

func detectDistroNameUnified(reader FileSystemReader, isoPath string) string {
	if isTrueNASScale(reader.FileExists) {
		return "truenas-scale"
	}
	filename := strings.ToLower(filepath.Base(isoPath))

	distroPatterns := map[string]string{
		"windows":         "windows",
		"win10":           "windows",
		"win11":           "windows",
		"win7":            "windows",
		"win8":            "windows",
		"server2022":      "windows",
		"server2019":      "windows",
		"server2016":      "windows",
		"popos":           "popos",
		"pop-os":          "popos",
		"pop_os":          "popos",
		"manjaro":         "manjaro",
		"mint":            "mint",
		"linuxmint":       "mint",
		"elementary":      "elementary",
		"zorin":           "zorin",
		"ubuntu":          "ubuntu",
		"debian":          "debian",
		"arch":            "arch",
		"fedora":          "fedora",
		"centos":          "centos",
		"rocky":           "rocky",
		"alma":            "alma",
		"kali":            "kali",
		"parrot":          "parrot",
		"tails":           "tails",
		"opensuse":        "opensuse",
		"freebsd":         "freebsd",
		"openbsd":         "openbsd",
		"netbsd":          "netbsd",
		"nixos":           "nixos",
		"endeavouros":     "endeavouros",
		"garuda":          "garuda",
		"arco":            "arco",
		"cachyos":         "arch",
		"cachy":           "arch",
		"artix":           "arch",
		"blackarch":       "arch",
		"parabola":        "arch",
		"truenas-scale":   "truenas-scale",
		"proxmox":         "proxmox",
		"coreos":          "coreos",
		"flatcar":         "flatcar",
		"devuan":          "debian",
		"antix":           "debian",
		"mx-":             "debian",
		"mxlinux":         "debian",
		"pureos":          "debian",
		"deepin":          "debian",
		"lmde":            "debian",
		"sparky":          "debian",
		"bunsenlabs":      "debian",
		"xubuntu":         "ubuntu",
		"kubuntu":         "ubuntu",
		"lubuntu":         "ubuntu",
		"edubuntu":        "ubuntu",
		"budgie":          "ubuntu",
		"ubuntumate":      "ubuntu",
		"ubuntustudio":    "ubuntu",
		"noble":           "ubuntu",
		"jammy":           "ubuntu",
		"mageia":          "fedora",
		"nobara":          "fedora",
		"ultramarine":     "fedora",
		"eurolinux":       "centos",
		"springdale":      "centos",
		"oracle":          "centos",
		"oraclelinux":     "centos",
		"scientificlinux": "centos",
		"gentoo":          "gentoo",
		"calculate":       "gentoo",
		"void":            "void",
		"slackware":       "slackware",
		"solus":           "solus",
		"steamos":         "arch",
		"tinycore":        "tinycore",
		"alpine":          "alpine",
		"clearlinux":      "clearlinux",
		"clear-linux":     "clearlinux",
	}

	if distro := matchDistroPattern(distroPatterns, filename); distro != "" {
		return distro
	}

	if reader.FileExists("/.disk/info") {
		if content := reader.ReadFileContent("/.disk/info"); content != "" {
			return matchDistroPattern(distroPatterns, strings.ToLower(content))
		}
	}

	return ""
}

func matchDistroPattern(distroPatterns map[string]string, text string) string {
	patterns := make([]string, 0, len(distroPatterns))
	for pattern := range distroPatterns {
		patterns = append(patterns, pattern)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i]) != len(patterns[j]) {
			return len(patterns[i]) > len(patterns[j])
		}
		return patterns[i] < patterns[j]
	})
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return distroPatterns[pattern]
		}
	}
	return ""
}

func (e *Extractor) detectLiveDebianUnified(reader FileSystemReader) *BootFiles {
	entries, err := reader.ListDirectory("/live")
	if err != nil || entries == nil {
		return nil
	}

	var kernel, versionedKernel, initrd, versionedInitrd, squashfs string
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := strings.ToLower(entry.Name)
		path := "/live/" + entry.Name
		switch {
		case name == "vmlinuz":
			kernel = path
		case strings.HasPrefix(name, "vmlinuz-") && versionedKernel == "":
			versionedKernel = path
		case name == "initrd.img" || name == "initrd":
			initrd = path
		case (strings.HasPrefix(name, "initrd.img-") || strings.HasPrefix(name, "initrd-")) && versionedInitrd == "":
			versionedInitrd = path
		case strings.HasPrefix(name, "filesystem.squashfs"):
			squashfs = path
		case strings.HasSuffix(name, ".squashfs") && squashfs == "":
			squashfs = path
		}
	}

	if versionedKernel != "" {
		kernel = versionedKernel
	}
	if versionedInitrd != "" {
		initrd = versionedInitrd
	}
	if kernel == "" || initrd == "" || squashfs == "" {
		return nil
	}

	log.Printf("Detected Debian live system: kernel=%s initrd=%s squashfs=%s", kernel, initrd, squashfs)
	return &BootFiles{
		Kernel:       kernel,
		Initrd:       initrd,
		Distro:       "debian",
		SquashfsPath: squashfs,
	}
}

func (e *Extractor) detectUbuntuDebianUnified(reader FileSystemReader) (*BootFiles, error) {
	if files := e.detectLiveDebianUnified(reader); files != nil {
		return files, nil
	}

	paths := []struct {
		kernel      string
		initrd      string
		distro      string
		bootParams  string
		netbootArch string
	}{
		{"/casper/vmlinuz", "/casper/initrd", "ubuntu", "", ""},
		{"/casper/vmlinuz", "/casper/initrd.lz", "ubuntu", "", ""},
		{"/casper/vmlinuz", "/casper/initrd.gz", "ubuntu", "", ""},
		{"/casper/vmlinuz.efi", "/casper/initrd.lz", "ubuntu", "", ""},
		{"/casper/vmlinuz.efi", "/casper/initrd", "ubuntu", "", ""},
		{"/casper/vmlinuz.efi", "/casper/initrd.gz", "ubuntu", "", ""},
		{"/install/netboot/ubuntu-installer/amd64/linux", "/install/netboot/ubuntu-installer/amd64/initrd.gz", "ubuntu", "", ""},
		{"/install/netboot/ubuntu-installer/arm64/linux", "/install/netboot/ubuntu-installer/arm64/initrd.gz", "ubuntu", "", ""},
		{"/install/netboot/ubuntu-installer/i386/linux", "/install/netboot/ubuntu-installer/i386/initrd.gz", "ubuntu", "", ""},
		{"/install/vmlinuz", "/install/initrd.gz", "debian", "", "amd64"},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "debian", "", "amd64"},
		{"/install.a64/vmlinuz", "/install.a64/initrd.gz", "debian", "", "arm64"},
		{"/live/vmlinuz", "/live/initrd.img", "debian", "", ""},
		{"/live/vmlinuz1", "/live/initrd1.img", "debian", "", ""},
		{"/vmlinuz", "/initrd.img", "debian", "", ""},
		{"/boot/linux26", "/boot/initrd.img", "proxmox", "", ""},
	}

	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			bootFiles := &BootFiles{
				Kernel:      p.kernel,
				Initrd:      p.initrd,
				Distro:      p.distro,
				BootParams:  p.bootParams,
				NetbootArch: p.netbootArch,
			}
			resolveNetboot(bootFiles)
			squashfs := parentDir(p.kernel) + "/filesystem.squashfs"
			if reader.FileExists(squashfs) {
				bootFiles.SquashfsPath = squashfs
			}
			return bootFiles, nil
		}
	}

	return nil, fmt.Errorf("kernel/initrd not found in common Ubuntu/Debian paths")
}

var netbootSources = map[string]string{
	"debian": "http://ftp.debian.org/debian/dists/trixie/main/installer-%s/current/images/netboot/netboot.tar.gz",
	"kali":   "http://http.kali.org/dists/kali-rolling/main/installer-%s/current/images/netboot/netboot.tar.gz",
}

func resolveNetboot(files *BootFiles) {
	if files.NetbootArch == "" {
		return
	}
	source, ok := netbootSources[files.Distro]
	files.NetbootRequired = ok
	files.NetbootURL = ""
	if ok {
		files.NetbootURL = fmt.Sprintf(source, files.NetbootArch)
	}
}

func (e *Extractor) detectFedoraRHELUnified(reader FileSystemReader) (*BootFiles, error) {
	kernel := "/images/pxeboot/vmlinuz"
	initrd := "/images/pxeboot/initrd.img"

	if reader.FileExists(kernel) && reader.FileExists(initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "fedora",
			BootParams: "",
		}, nil
	}

	return nil, fmt.Errorf("not Fedora/RHEL")
}

func (e *Extractor) detectCentOSUnified(reader FileSystemReader) (*BootFiles, error) {
	kernel := "/images/pxeboot/vmlinuz"
	initrd := "/images/pxeboot/initrd.img"

	if reader.FileExists(kernel) && reader.FileExists(initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "centos",
			BootParams: "",
		}, nil
	}

	return nil, fmt.Errorf("not CentOS/Rocky/Alma")
}

func (e *Extractor) detectArchUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel     string
		initrd     string
		bootParams string
	}{
		{"/arch/boot/x86_64/vmlinuz-linux", "/arch/boot/x86_64/initramfs-linux.img", ""},
		{"/boot/vmlinuz-linux", "/boot/initramfs-linux.img", ""},
		{"/arch/boot/x86_64/vmlinuz-linux", "/arch/boot/x86_64/archiso.img", ""},
		{"/boot/vmlinuz-linux-cachyos", "/boot/initramfs-linux-cachyos.img", ""},
		{"/boot/vmlinuz-linux-zen", "/boot/initramfs-linux-zen.img", ""},
		{"/boot/vmlinuz-linux-lts", "/boot/initramfs-linux-lts.img", ""},
		{"/boot/vmlinuz-x86_64", "/boot/initramfs-x86_64.img", ""},
		{"/manjaro/boot/x86_64/manjaro", "/manjaro/boot/x86_64/manjaro.img", ""},
	}

	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{
				Kernel:     p.kernel,
				Initrd:     p.initrd,
				Distro:     "arch",
				BootParams: p.bootParams,
			}, nil
		}
	}

	return nil, fmt.Errorf("not Arch Linux")
}

func (e *Extractor) detectFreeBSDUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel     string
		initrd     string
		bootParams string
	}{
		{"/boot/kernel/kernel", "/boot/mfsroot.gz", ""},
		{"/boot/kernel/kernel", "/boot/kernel/kernel", ""},
	}

	for _, p := range paths {
		if reader.FileExists(p.kernel) {
			initrd := p.initrd
			if !reader.FileExists(initrd) {
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

func (e *Extractor) detectOpenSUSEUnified(reader FileSystemReader) (*BootFiles, error) {
	kernel := "/boot/x86_64/loader/linux"
	initrd := "/boot/x86_64/loader/initrd"

	if reader.FileExists(kernel) && reader.FileExists(initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "opensuse",
			BootParams: "",
		}, nil
	}

	return nil, fmt.Errorf("not OpenSUSE")
}

func (e *Extractor) detectNixOSUnified(reader FileSystemReader) (*BootFiles, error) {
	return nil, fmt.Errorf("not NixOS")
}

func (e *Extractor) detectAlpineUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel     string
		initrd     string
		bootParams string
	}{
		{"/boot/vmlinuz-lts", "/boot/initramfs-lts", ""},
		{"/boot/vmlinuz-virt", "/boot/initramfs-virt", ""},
		{"/boot/vmlinuz", "/boot/initramfs-init", ""},
	}
	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{Kernel: p.kernel, Initrd: p.initrd, Distro: "alpine", BootParams: p.bootParams}, nil
		}
	}
	return nil, fmt.Errorf("not Alpine Linux")
}

func (e *Extractor) detectGentooUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel     string
		initrd     string
		bootParams string
	}{
		{"/boot/gentoo", "/boot/gentoo.igz", ""},
		{"/boot/vmlinuz", "/boot/initrd", ""},
		{"/isolinux/gentoo", "/isolinux/gentoo.igz", ""},
		{"/boot/gentoo64", "/boot/gentoo64.igz", ""},
	}
	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{Kernel: p.kernel, Initrd: p.initrd, Distro: "gentoo", BootParams: p.bootParams}, nil
		}
	}
	return nil, fmt.Errorf("not Gentoo")
}

func (e *Extractor) detectVoidUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel string
		initrd string
	}{
		{"/boot/vmlinuz", "/boot/initrd"},
		{"/live/vmlinuz", "/live/initrd"},
	}
	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{Kernel: p.kernel, Initrd: p.initrd, Distro: "void", BootParams: ""}, nil
		}
	}
	return nil, fmt.Errorf("not Void Linux")
}

func (e *Extractor) detectSlackwareUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel string
		initrd string
	}{
		{"/kernels/huge.s/bzImage", "/isolinux/initrd.img"},
		{"/kernels/hugesmp.s/bzImage", "/isolinux/initrd.img"},
		{"/boot/vmlinuz", "/boot/initrd.img"},
	}
	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{Kernel: p.kernel, Initrd: p.initrd, Distro: "slackware"}, nil
		}
	}
	return nil, fmt.Errorf("not Slackware")
}

func (e *Extractor) detectSolusUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel string
		initrd string
	}{
		{"/boot/kernel.current", "/boot/initrd.current"},
		{"/boot/vmlinuz", "/boot/initrd"},
	}
	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{Kernel: p.kernel, Initrd: p.initrd, Distro: "solus"}, nil
		}
	}
	return nil, fmt.Errorf("not Solus")
}

func (e *Extractor) detectTinyCoreUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel string
		initrd string
	}{
		{"/boot/vmlinuz", "/boot/core.gz"},
		{"/boot/vmlinuz64", "/boot/corepure64.gz"},
	}
	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{Kernel: p.kernel, Initrd: p.initrd, Distro: "tinycore"}, nil
		}
	}
	return nil, fmt.Errorf("not Tiny Core Linux")
}

func (e *Extractor) detectClearLinuxUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel string
		initrd string
	}{
		{"/kernel/kernel.org.clearlinux.native", "/initrd/initrd.img.clearlinux.native"},
		{"/EFI/org.clearlinux/kernel.org.clearlinux.native", "/EFI/org.clearlinux/initrd"},
	}
	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			return &BootFiles{Kernel: p.kernel, Initrd: p.initrd, Distro: "clearlinux"}, nil
		}
	}
	return nil, fmt.Errorf("not Clear Linux")
}

func (e *Extractor) detectFlatcarUnified(reader FileSystemReader) (*BootFiles, error) {
	kernel := "/flatcar/vmlinuz"
	initrd := "/flatcar/cpio.gz"

	if reader.FileExists(kernel) && reader.FileExists(initrd) {
		return &BootFiles{Kernel: kernel, Initrd: initrd, Distro: "flatcar"}, nil
	}

	return nil, fmt.Errorf("not Flatcar Container Linux")
}

func (e *Extractor) detectSystemRescueUnified(reader FileSystemReader) (*BootFiles, error) {
	kernel := "/sysresccd/boot/x86_64/vmlinuz"
	initrd := "/sysresccd/boot/x86_64/sysresccd.img"

	if reader.FileExists(kernel) && reader.FileExists(initrd) {
		return &BootFiles{
			Kernel: kernel,
			Initrd: initrd,
			Distro: "systemrescue",
		}, nil
	}

	return nil, fmt.Errorf("not SystemRescue")
}

func (e *Extractor) detectWindowsUnified(reader FileSystemReader) (*BootFiles, error) {
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

	installWimPaths := []string{
		"/sources/install.wim",
		"/SOURCES/INSTALL.WIM",
		"/sources/install.esd",
		"/SOURCES/INSTALL.ESD",
	}

	var bcdPath, bootSdiPath, bootWimPath, installWimPath string

	for _, path := range bcdPaths {
		if reader.FileExists(path) {
			bcdPath = path
			break
		}
	}

	for _, path := range bootSdiPaths {
		if reader.FileExists(path) {
			bootSdiPath = path
			break
		}
	}

	for _, path := range bootWimPaths {
		if reader.FileExists(path) {
			bootWimPath = path
			break
		}
	}

	for _, path := range installWimPaths {
		if reader.FileExists(path) {
			installWimPath = path
			break
		}
	}

	if bcdPath == "" || bootSdiPath == "" {
		return nil, fmt.Errorf("not Windows ISO (found: BCD=%v, boot.sdi=%v)", bcdPath != "", bootSdiPath != "")
	}

	wimPaths := findWIMFiles(reader)
	if bootWimPath == "" && len(wimPaths) > 0 {
		bootWimPath = wimPaths[0]
	}
	if bootWimPath == "" {
		return nil, fmt.Errorf("not Windows ISO (BCD and boot.sdi present but no .wim found)")
	}

	distro := "windows"
	if installWimPath == "" {
		distro = "winpe"
	}
	files := &BootFiles{
		Kernel:     bcdPath,
		Initrd:     bootSdiPath,
		Distro:     distro,
		BootWim:    bootWimPath,
		InstallWim: installWimPath,
	}
	for _, p := range wimPaths {
		files.WIMFiles = append(files.WIMFiles, WIMFile{Path: strings.TrimPrefix(p, "/")})
	}
	return files, nil
}

func (e *Extractor) cacheBootFilesUnified(files *BootFiles, reader FileSystemReader, isoPath string) error {
	isoBase := relativeISOBase(e.dataDir, isoPath)
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	if err := os.MkdirAll(bootFilesDir, 0755); err != nil {
		return fmt.Errorf("failed to create boot files subdirectory: %w", err)
	}

	if bsdboot.IsBSD(files.Distro) {
		return e.cacheBSDFiles(files, reader, bootFilesDir)
	}

	if isWindowsDistro(files.Distro) {
		return e.cacheWindowsFiles(files, reader, bootFilesDir)
	}

	kernelDest := filepath.Join(bootFilesDir, "vmlinuz")
	if err := reader.ExtractFile(files.Kernel, kernelDest); err != nil {
		return fmt.Errorf("failed to extract kernel: %w", err)
	}
	files.Kernel = kernelDest

	initrdDest := filepath.Join(bootFilesDir, "initrd")
	if err := reader.ExtractFile(files.Initrd, initrdDest); err != nil {
		return fmt.Errorf("failed to extract initrd: %w", err)
	}
	files.Initrd = initrdDest

	extractedDir := filepath.Join(bootFilesDir, "iso")
	if err := os.MkdirAll(extractedDir, 0755); err != nil {
		return fmt.Errorf("failed to create extracted ISO directory: %w", err)
	}

	log.Printf("Extracting full ISO contents to %s", extractedDir)
	if err := reader.ExtractAll(extractedDir); err != nil {
		return fmt.Errorf("failed to extract full ISO contents: %w", err)
	}

	files.ExtractedDir = extractedDir

	if files.SquashfsPath != "" {
		if rel := resolveExtractedRelPath(bootFilesDir, files.SquashfsPath); rel != "" {
			files.SquashfsPath = rel
		} else {
			log.Printf("Warning: squashfs %s not found in extracted ISO contents", files.SquashfsPath)
			files.SquashfsPath = ""
		}
	}

	return nil
}

func resolveExtractedRelPath(bootFilesDir, isoPath string) string {
	rel := "iso"
	cur := filepath.Join(bootFilesDir, "iso")
	for _, part := range strings.Split(strings.TrimPrefix(isoPath, "/"), "/") {
		if part == "" {
			continue
		}
		entries, err := os.ReadDir(cur)
		if err != nil {
			return ""
		}
		match := ""
		for _, entry := range entries {
			if entry.Name() == part {
				match = part
				break
			}
			if match == "" && strings.EqualFold(entry.Name(), part) {
				match = entry.Name()
			}
		}
		if match == "" {
			return ""
		}
		rel = rel + "/" + match
		cur = filepath.Join(cur, match)
	}
	return rel
}
