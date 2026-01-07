package server

import (
	"bootimus/internal/models"
	"fmt"
	"net/url"
	"strings"
)

type MenuBuilder struct {
	images       []models.Image
	groups       []*models.ImageGroup
	macAddress   string
	serverAddr   string
	httpPort     int
	groupStack   []uint
}

func (s *Server) generateIPXEMenuWithGroups(images []models.Image, macAddress string) string {
	groups, err := s.config.Storage.ListImageGroups()
	if err != nil {
		return s.generateIPXEMenu(images, macAddress)
	}

	mb := &MenuBuilder{
		images:     images,
		groups:     groups,
		macAddress: macAddress,
		serverAddr: s.config.ServerAddr,
		httpPort:   s.config.HTTPPort,
	}

	return mb.Build()
}

func (mb *MenuBuilder) Build() string {
	var sb strings.Builder

	sb.WriteString("#!ipxe\n\n")
	sb.WriteString(mb.buildMainMenu())
	sb.WriteString(mb.buildGroupMenus())
	sb.WriteString(mb.buildImageBootSections())
	sb.WriteString(mb.buildFooter())

	return sb.String()
}

func (mb *MenuBuilder) buildMainMenu() string {
	var sb strings.Builder

	sb.WriteString(":start\n")
	sb.WriteString("menu Bootimus - Boot Menu\n")

	rootGroups := mb.getRootGroups()
	ungroupedImages := mb.getUngroupedImages()

	if len(rootGroups) > 0 {
		sb.WriteString("item --gap -- Groups:\n")
		for _, group := range rootGroups {
			if group.Enabled {
				sb.WriteString(fmt.Sprintf("item group%d %s\n", group.ID, group.Name))
			}
		}
	}

	if len(ungroupedImages) > 0 {
		sb.WriteString("item --gap -- Images:\n")
		for idx, img := range ungroupedImages {
			sizeStr := formatSize(img.Size)
			extractedTag := ""
			if img.Extracted {
				extractedTag = " [kernel]"
			}
			sb.WriteString(fmt.Sprintf("item iso%d %s (%s)%s\n", idx, img.Name, sizeStr, extractedTag))
		}
	}

	sb.WriteString("item --gap -- Options:\n")
	sb.WriteString("item shell Drop to iPXE shell\n")
	sb.WriteString("item reboot Reboot\n")
	sb.WriteString("choose --default iso0 --timeout 30000 selected || goto start\n")
	sb.WriteString("goto ${selected}\n\n")

	return sb.String()
}

func (mb *MenuBuilder) buildGroupMenus() string {
	var sb strings.Builder

	for _, group := range mb.groups {
		if !group.Enabled {
			continue
		}

		sb.WriteString(fmt.Sprintf(":group%d\n", group.ID))
		sb.WriteString(fmt.Sprintf("menu Bootimus - %s\n", group.Name))

		childGroups := mb.getChildGroups(group.ID)
		groupImages := mb.getGroupImages(group.ID)

		if len(childGroups) > 0 {
			sb.WriteString("item --gap -- Subgroups:\n")
			for _, child := range childGroups {
				if child.Enabled {
					sb.WriteString(fmt.Sprintf("item group%d %s\n", child.ID, child.Name))
				}
			}
		}

		if len(groupImages) > 0 {
			sb.WriteString("item --gap -- Images:\n")
			for _, img := range groupImages {
				sizeStr := formatSize(img.Size)
				extractedTag := ""
				if img.Extracted {
					extractedTag = " [kernel]"
				}
				sb.WriteString(fmt.Sprintf("item iso%d %s (%s)%s\n", img.ID, img.Name, sizeStr, extractedTag))
			}
		}

		sb.WriteString("item --gap -- Navigation:\n")
		if group.ParentID != nil {
			sb.WriteString(fmt.Sprintf("item group%d Back to %s\n", *group.ParentID, group.Parent.Name))
		} else {
			sb.WriteString("item start Back to Main Menu\n")
		}
		sb.WriteString("item shell Drop to iPXE shell\n")
		sb.WriteString("item reboot Reboot\n")
		sb.WriteString(fmt.Sprintf("choose --timeout 30000 selected || goto group%d\n", group.ID))
		sb.WriteString("goto ${selected}\n\n")
	}

	return sb.String()
}

func (mb *MenuBuilder) buildImageBootSections() string {
	var sb strings.Builder

	for _, img := range mb.images {
		if !img.Enabled {
			continue
		}

		sb.WriteString(fmt.Sprintf(":iso%d\n", img.ID))
		sb.WriteString(fmt.Sprintf("echo Booting %s...\n", img.Name))

		encodedFilename := url.PathEscape(img.Filename)
		cacheDir := strings.TrimSuffix(img.Filename, ".iso")

		switch img.BootMethod {
		case "memdisk":
			sb.WriteString("echo Using Thin OS memdisk loader...\n")
			sb.WriteString(fmt.Sprintf("kernel http://%s:%d/thinos-kernel\n", mb.serverAddr, mb.httpPort))
			sb.WriteString(fmt.Sprintf("initrd http://%s:%d/thinos-initrd.gz\n", mb.serverAddr, mb.httpPort))
			sb.WriteString(fmt.Sprintf("imgargs thinos-kernel ISO_NAME=%s BOOTIMUS_SERVER=%s console=tty0 console=ttyS0,115200n8 earlyprintk=vga,keep debug loglevel=8 rdinit=/init\n", encodedFilename, mb.serverAddr))
			sb.WriteString("boot || goto failed\n")

		case "kernel":
			sb.WriteString("echo Loading kernel and initrd...\n")
			if img.AutoInstallEnabled {
				sb.WriteString("echo Auto-install enabled for this image\n")
			}

			sb.WriteString(mb.buildKernelBootSection(&img, encodedFilename, cacheDir))

		default:
			sb.WriteString(fmt.Sprintf("sanboot --no-describe --drive 0x80 http://%s:%d/isos/%s?mac=%s\n", mb.serverAddr, mb.httpPort, encodedFilename, mb.macAddress))
		}

		if img.GroupID != nil {
			sb.WriteString(fmt.Sprintf("goto group%d\n", *img.GroupID))
		} else {
			sb.WriteString("goto start\n")
		}
	}

	return sb.String()
}

func (mb *MenuBuilder) buildKernelBootSection(img *models.Image, encodedFilename, cacheDir string) string {
	var sb strings.Builder

	autoInstallParam := ""
	if img.AutoInstallEnabled {
		autoInstallParam = " autoinstall"
	}

	bootParams := img.BootParams
	if bootParams != "" {
		bootParams = " " + bootParams
	}

	baseURL := fmt.Sprintf("http://%s:%d", mb.serverAddr, mb.httpPort)

	switch img.Distro {
	case "windows":
		sb.WriteString("echo Loading Windows boot files via wimboot...\n")
		sb.WriteString(fmt.Sprintf("kernel %s/wimboot\n", baseURL))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/bcd BCD\n", baseURL, cacheDir))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/boot.sdi boot.sdi\n", baseURL, cacheDir))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/boot.wim @boot.wim\n", baseURL, cacheDir))
		if img.InstallWimPath != "" {
			sb.WriteString(fmt.Sprintf("initrd --name install.wim %s/boot/%s/install.wim\n", baseURL, cacheDir))
		}
		sb.WriteString("boot || goto failed\n")

	case "arch":
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%sarchiso_http_srv=%s/boot/%s/iso/ ip=dhcp\n", baseURL, cacheDir, autoInstallParam, bootParams, baseURL, cacheDir))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")

	case "nixos":
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%sip=dhcp\n", baseURL, cacheDir, autoInstallParam, bootParams))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")

	case "fedora", "centos":
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%sroot=live:%s/isos/%s rd.live.image inst.repo=%s/boot/%s/iso/ inst.stage2=%s/boot/%s/iso/ rd.neednet=1 ip=dhcp\n", baseURL, cacheDir, autoInstallParam, bootParams, baseURL, encodedFilename, baseURL, cacheDir, baseURL, cacheDir))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")

	case "debian":
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%sinitrd=initrd ip=dhcp priority=critical\n", baseURL, cacheDir, autoInstallParam, bootParams))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")

	case "ubuntu":
		if img.NetbootAvailable {
			sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%sinitrd=initrd ip=dhcp\n", baseURL, cacheDir, autoInstallParam, bootParams))
		} else if img.SquashfsPath != "" {
			sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%sinitrd=initrd ip=dhcp fetch=%s/boot/%s/%s\n", baseURL, cacheDir, autoInstallParam, bootParams, baseURL, cacheDir, img.SquashfsPath))
		} else {
			sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%sinitrd=initrd ip=dhcp url=%s/isos/%s\n", baseURL, cacheDir, autoInstallParam, bootParams, baseURL, encodedFilename))
		}
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")

	case "freebsd":
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz vfs.root.mountfrom=cd9660:/dev/md0 kernelname=/boot/kernel/kernel\n", baseURL, cacheDir))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")

	default:
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%siso-url=%s/isos/%s ip=dhcp\n", baseURL, cacheDir, autoInstallParam, bootParams, baseURL, encodedFilename))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")
	}

	return sb.String()
}

func (mb *MenuBuilder) buildFooter() string {
	return `:shell
echo Dropping to iPXE shell...
shell

:reboot
reboot

:failed
echo Boot failed, returning to menu in 5 seconds...
sleep 5
goto start
`
}

func (mb *MenuBuilder) getRootGroups() []*models.ImageGroup {
	var result []*models.ImageGroup
	for _, group := range mb.groups {
		if group.ParentID == nil && group.Enabled {
			result = append(result, group)
		}
	}
	return result
}

func (mb *MenuBuilder) getChildGroups(parentID uint) []*models.ImageGroup {
	var result []*models.ImageGroup
	for _, group := range mb.groups {
		if group.ParentID != nil && *group.ParentID == parentID && group.Enabled {
			result = append(result, group)
		}
	}
	return result
}

func (mb *MenuBuilder) getUngroupedImages() []models.Image {
	var result []models.Image
	for _, img := range mb.images {
		if img.GroupID == nil && img.Enabled {
			result = append(result, img)
		}
	}
	return result
}

func (mb *MenuBuilder) getGroupImages(groupID uint) []models.Image {
	var result []models.Image
	for _, img := range mb.images {
		if img.GroupID != nil && *img.GroupID == groupID && img.Enabled {
			result = append(result, img)
		}
	}
	return result
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
