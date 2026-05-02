package extractor

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func detectDistroNameUnified(reader FileSystemReader, isoPath string) string {
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
		"cachyos":     "arch",
		"cachy":       "arch",
		"artix":       "arch",
		"blackarch":   "arch",
		"parabola":    "arch",
		"truenas":     "debian",
		"proxmox":     "debian",
		"devuan":      "debian",
		"antix":       "debian",
		"mx-":         "debian",
		"mxlinux":     "debian",
		"pureos":      "debian",
		"deepin":      "debian",
		"lmde":        "debian",
		"sparky":      "debian",
		"bunsenlabs":  "debian",
		"xubuntu":     "ubuntu",
		"kubuntu":     "ubuntu",
		"lubuntu":     "ubuntu",
		"edubuntu":    "ubuntu",
		"budgie":      "ubuntu",
		"ubuntumate":  "ubuntu",
		"ubuntustudio": "ubuntu",
		"noble":       "ubuntu",
		"jammy":       "ubuntu",
		"mageia":      "fedora",
		"nobara":      "fedora",
		"ultramarine": "fedora",
		"eurolinux":   "centos",
		"springdale":  "centos",
		"oracle":      "centos",
		"oraclelinux": "centos",
		"scientificlinux": "centos",
		"gentoo":      "gentoo",
		"calculate":   "gentoo",
		"void":        "void",
		"slackware":   "slackware",
		"solus":       "solus",
		"steamos":     "arch",
		"tinycore":    "tinycore",
		"alpine":      "alpine",
		"clearlinux":  "clearlinux",
		"clear-linux": "clearlinux",
	}

	for pattern, distro := range distroPatterns {
		if strings.Contains(filename, pattern) {
			return distro
		}
	}

	if reader.FileExists("/.disk/info") {
		if content := reader.ReadFileContent("/.disk/info"); content != "" {
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

func (e *Extractor) detectUbuntuDebianUnified(reader FileSystemReader) (*BootFiles, error) {
	paths := []struct {
		kernel     string
		initrd     string
		distro     string
		bootParams string
		netboot    bool
		netbootURL string
	}{
		{"/casper/vmlinuz", "/casper/initrd", "ubuntu", "", false, ""},
		{"/casper/vmlinuz", "/casper/initrd.lz", "ubuntu", "", false, ""},
		{"/casper/vmlinuz", "/casper/initrd.gz", "ubuntu", "", false, ""},
		{"/casper/vmlinuz.efi", "/casper/initrd.lz", "ubuntu", "", false, ""},
		{"/casper/vmlinuz.efi", "/casper/initrd", "ubuntu", "", false, ""},
		{"/casper/vmlinuz.efi", "/casper/initrd.gz", "ubuntu", "", false, ""},
		{"/install/vmlinuz", "/install/initrd.gz", "debian", "", true, "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "debian", "", true, "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"},
		{"/live/vmlinuz", "/live/initrd.img", "debian", "", false, ""},
		{"/live/vmlinuz1", "/live/initrd1.img", "debian", "", false, ""},
		{"/vmlinuz", "/initrd.img", "debian", "", false, ""},
		{"/boot/linux26", "/boot/initrd.img", "debian", "", false, ""},
	}

	for _, p := range paths {
		if reader.FileExists(p.kernel) && reader.FileExists(p.initrd) {
			bootFiles := &BootFiles{
				Kernel:          p.kernel,
				Initrd:          p.initrd,
				Distro:          p.distro,
				BootParams:      p.bootParams,
				NetbootRequired: p.netboot,
				NetbootURL:      p.netbootURL,
			}
			return bootFiles, nil
		}
	}

	return nil, fmt.Errorf("kernel/initrd not found in common Ubuntu/Debian paths")
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

func (e *Extractor) detectSystemRescueUnified(reader FileSystemReader) (*BootFiles, error) {
	kernel := "/sysresccd/boot/x86_64/vmlinuz"
	initrd := "/sysresccd/boot/intel_ucode.img"

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

	if bcdPath != "" && bootSdiPath != "" && bootWimPath != "" {
		return &BootFiles{
			Kernel:     bcdPath,
			Initrd:     bootSdiPath,
			Distro:     "windows",
			BootParams: bootWimPath,
			InstallWim: installWimPath,
		}, nil
	}

	return nil, fmt.Errorf("not Windows ISO")
}

func (e *Extractor) cacheBootFilesUnified(files *BootFiles, reader FileSystemReader, isoPath string) error {
	isoBase := relativeISOBase(e.dataDir, isoPath)
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	if err := os.MkdirAll(bootFilesDir, 0755); err != nil {
		return fmt.Errorf("failed to create boot files subdirectory: %w", err)
	}

	if files.Distro == "windows" {
		extractedDir := filepath.Join(bootFilesDir, "iso")
		if err := os.MkdirAll(extractedDir, 0755); err != nil {
			return fmt.Errorf("failed to create extracted ISO directory: %w", err)
		}

		log.Printf("Extracting full Windows ISO contents to %s", extractedDir)
		if err := reader.ExtractAll(extractedDir); err != nil {
			return fmt.Errorf("failed to extract full ISO contents: %w", err)
		}

		files.ExtractedDir = extractedDir

		bcdDest := filepath.Join(extractedDir, strings.TrimPrefix(files.Kernel, "/"))
		files.Kernel = bcdDest

		bootSdiDest := filepath.Join(extractedDir, strings.TrimPrefix(files.Initrd, "/"))
		files.Initrd = bootSdiDest

		bootWimDest := filepath.Join(extractedDir, strings.TrimPrefix(files.BootParams, "/"))
		files.BootParams = bootWimDest

		if files.InstallWim != "" {
			installDest := filepath.Join(extractedDir, strings.TrimPrefix(files.InstallWim, "/"))
			files.InstallWim = installDest
		}

		log.Printf("Windows ISO extraction complete")
		return nil
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

	return nil
}
