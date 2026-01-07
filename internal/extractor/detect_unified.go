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
		{"/casper/vmlinuz", "/casper/initrd", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ", false, ""},
		{"/casper/vmlinuz", "/casper/initrd.lz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ", false, ""},
		{"/casper/vmlinuz", "/casper/initrd.gz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ", false, ""},
		{"/casper/vmlinuz.efi", "/casper/initrd.lz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ", false, ""},
		{"/casper/vmlinuz.efi", "/casper/initrd", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ", false, ""},
		{"/casper/vmlinuz.efi", "/casper/initrd.gz", "ubuntu", "boot=casper root=/dev/ram0 ramdisk_size=1500000 cloud-init=disabled ", false, ""},
		{"/install/vmlinuz", "/install/initrd.gz", "debian", "", true, "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"},
		{"/install.amd/vmlinuz", "/install.amd/initrd.gz", "debian", "", true, "http://ftp.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/netboot.tar.gz"},
		{"/live/vmlinuz", "/live/initrd.img", "debian", "boot=live fetch= ", false, ""},
		{"/live/vmlinuz1", "/live/initrd1.img", "debian", "boot=live fetch= ", false, ""},
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
	kernel := "/arch/boot/x86_64/vmlinuz-linux"
	initrd := "/arch/boot/x86_64/initramfs-linux.img"

	if reader.FileExists(kernel) && reader.FileExists(initrd) {
		return &BootFiles{
			Kernel:     kernel,
			Initrd:     initrd,
			Distro:     "arch",
			BootParams: "archisobasedir=arch ",
		}, nil
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
			BootParams: "install=",
		}, nil
	}

	return nil, fmt.Errorf("not OpenSUSE")
}

func (e *Extractor) detectNixOSUnified(reader FileSystemReader) (*BootFiles, error) {
	return nil, fmt.Errorf("not NixOS")
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
	isoBase := strings.TrimSuffix(filepath.Base(isoPath), filepath.Ext(isoPath))
	bootFilesDir := filepath.Join(e.dataDir, isoBase)

	if err := os.MkdirAll(bootFilesDir, 0755); err != nil {
		return fmt.Errorf("failed to create boot files subdirectory: %w", err)
	}

	if files.Distro == "windows" {
		bcdDest := filepath.Join(bootFilesDir, "bcd")
		if err := reader.ExtractFile(files.Kernel, bcdDest); err != nil {
			return fmt.Errorf("failed to extract BCD: %w", err)
		}
		files.Kernel = bcdDest

		bootSdiDest := filepath.Join(bootFilesDir, "boot.sdi")
		if err := reader.ExtractFile(files.Initrd, bootSdiDest); err != nil {
			return fmt.Errorf("failed to extract boot.sdi: %w", err)
		}
		files.Initrd = bootSdiDest

		bootWimDest := filepath.Join(bootFilesDir, "boot.wim")
		if err := reader.ExtractFile(files.BootParams, bootWimDest); err != nil {
			return fmt.Errorf("failed to extract boot.wim: %w", err)
		}
		files.BootParams = bootWimDest

		if files.InstallWim != "" {
			ext := filepath.Ext(files.InstallWim)
			installDest := filepath.Join(bootFilesDir, "install"+ext)
			log.Printf("Extracting Windows install image: %s", files.InstallWim)
			if err := reader.ExtractFile(files.InstallWim, installDest); err != nil {
				log.Printf("Warning: Failed to extract install image: %v", err)
			} else {
				files.InstallWim = installDest
				log.Printf("Successfully extracted install image to: %s", installDest)
			}
		}

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
