package server

import (
	"bootimus/internal/models"
	"bootimus/internal/profiles"
	"bootimus/internal/tools"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

type MenuBuilder struct {
	images           []models.Image
	groups           []*models.ImageGroup
	theme            *models.MenuTheme
	macAddress       string
	serverAddr       string
	httpPort         int
	groupStack       []uint
	enabledTools     []tools.EnabledTool
	nextBootImageID  uint
	profileManager   *profiles.Manager
}

func (s *Server) generateIPXEMenuWithGroups(images []models.Image, macAddress string, nextBootImageID ...uint) string {
	groups, err := s.config.Storage.ListImageGroups()
	if err != nil {
		return s.generateIPXEMenu(images, macAddress)
	}

	theme, err := s.config.Storage.GetMenuTheme()
	if err != nil {
		log.Printf("Warning: Failed to load menu theme: %v", err)
	}

	serverURL := fmt.Sprintf("http://%s:%d", s.config.ServerAddr, s.config.HTTPPort)
	enabledTools := s.toolsManager.GetEnabledTools(serverURL)

	var nbID uint
	if len(nextBootImageID) > 0 {
		nbID = nextBootImageID[0]
	}

	mb := &MenuBuilder{
		images:          images,
		groups:          groups,
		theme:           theme,
		macAddress:      macAddress,
		serverAddr:      s.config.ServerAddr,
		httpPort:        s.config.HTTPPort,
		enabledTools:    enabledTools,
		nextBootImageID: nbID,
		profileManager:  s.config.ProfileManager,
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

func (mb *MenuBuilder) menuTimeoutMs() int {
	if mb.theme != nil && mb.theme.MenuTimeout == 0 {
		return 0
	}
	if mb.theme != nil && mb.theme.MenuTimeout > 0 {
		return mb.theme.MenuTimeout * 1000
	}
	return 30000
}

func (mb *MenuBuilder) menuTitle() string {
	if mb.theme != nil && mb.theme.Title != "" {
		return mb.theme.Title
	}
	return "Bootimus - Boot Menu"
}

// encodePathSegments URL-encodes each segment of a path (handling spaces etc.)
// while preserving / separators, so "linux/Ubuntu Server.iso" becomes "linux/Ubuntu%20Server.iso"
func encodePathSegments(path string) string {
	segments := strings.Split(filepath.ToSlash(path), "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

func (mb *MenuBuilder) buildMainMenu() string {
	var sb strings.Builder

	sb.WriteString(":start\n")
	sb.WriteString(fmt.Sprintf("menu %s\n", mb.menuTitle()))

	rootGroups := mb.getRootGroups()
	ungroupedImages := mb.getUngroupedImages()

	var visibleGroups []*models.ImageGroup
	for _, group := range rootGroups {
		if group.Enabled && mb.groupHasImages(group.ID) {
			visibleGroups = append(visibleGroups, group)
		}
	}

	if len(mb.enabledTools) > 0 {
		sb.WriteString("item --gap -- Tools:\n")
		sb.WriteString("item tools Tools >>\n")
	}

	if len(visibleGroups) > 0 {
		sb.WriteString("item --gap -- Groups:\n")
		for _, group := range visibleGroups {
			sb.WriteString(fmt.Sprintf("item group%d %s\n", group.ID, group.Name))
		}
	}

	if len(ungroupedImages) > 0 {
		sb.WriteString("item --gap -- Images:\n")
		for _, img := range ungroupedImages {
			sizeStr := formatSize(img.Size)
			extractedTag := ""
			if img.Extracted {
				extractedTag = " [kernel]"
			}
			sb.WriteString(fmt.Sprintf("item iso%d %s (%s)%s\n", img.ID, img.Name, sizeStr, extractedTag))
		}
	}

	sb.WriteString("item --gap -- Options:\n")
	sb.WriteString("item shell Drop to iPXE shell\n")
	sb.WriteString("item reboot Reboot\n")
	defaultItem := "exit"
	if mb.nextBootImageID > 0 {
		defaultItem = fmt.Sprintf("iso%d", mb.nextBootImageID)
	} else if len(visibleGroups) > 0 {
		defaultItem = fmt.Sprintf("group%d", visibleGroups[0].ID)
	} else if len(ungroupedImages) > 0 {
		defaultItem = fmt.Sprintf("iso%d", ungroupedImages[0].ID)
	}

	timeoutMs := mb.menuTimeoutMs()
	if mb.nextBootImageID > 0 && timeoutMs == 0 {
		timeoutMs = 10000 // 10s override when next boot is set but global timeout is disabled
	}

	if timeoutMs > 0 {
		sb.WriteString(fmt.Sprintf("choose --default %s --timeout %d selected || goto start\n", defaultItem, timeoutMs))
	} else {
		sb.WriteString(fmt.Sprintf("choose --default %s selected || goto start\n", defaultItem))
	}
	sb.WriteString("goto ${selected}\n\n")

	return sb.String()
}

func (mb *MenuBuilder) buildGroupMenus() string {
	var sb strings.Builder

	for _, group := range mb.groups {
		if !group.Enabled || !mb.groupHasImages(group.ID) {
			continue
		}

		sb.WriteString(fmt.Sprintf(":group%d\n", group.ID))
		sb.WriteString(fmt.Sprintf("menu %s - %s\n", mb.menuTitle(), group.Name))

		childGroups := mb.getChildGroups(group.ID)
		groupImages := mb.getGroupImages(group.ID)

		if len(childGroups) > 0 {
			var visibleChildren []*models.ImageGroup
			for _, child := range childGroups {
				if child.Enabled && mb.groupHasImages(child.ID) {
					visibleChildren = append(visibleChildren, child)
				}
			}
			if len(visibleChildren) > 0 {
				sb.WriteString("item --gap -- Subgroups:\n")
				for _, child := range visibleChildren {
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
		if timeoutMs := mb.menuTimeoutMs(); timeoutMs > 0 {
			sb.WriteString(fmt.Sprintf("choose --timeout %d selected || goto group%d\n", timeoutMs, group.ID))
		} else {
			sb.WriteString(fmt.Sprintf("choose selected || goto group%d\n", group.ID))
		}
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

		encodedFilename := encodePathSegments(img.Filename)
		cacheDir := encodePathSegments(strings.TrimSuffix(img.Filename, ".iso"))

		switch img.BootMethod {
		case "nbd":
			sb.WriteString("echo Using NBD (Network Block Device) mount...\n")
			sb.WriteString(fmt.Sprintf("kernel http://%s:%d/bootenv/vmlinuz-lts\n", mb.serverAddr, mb.httpPort))
			sb.WriteString(fmt.Sprintf("initrd http://%s:%d/bootenv/initramfs-bootimus\n", mb.serverAddr, mb.httpPort))
			sb.WriteString(fmt.Sprintf("imgargs vmlinuz-lts init=/init iso=%s server=%s nbdport=10809 console=tty0 console=ttyS0\n", encodedFilename, mb.serverAddr))
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

	baseURL := fmt.Sprintf("http://%s:%d", mb.serverAddr, mb.httpPort)

	autoInstallParam := ""
	if img.AutoInstallEnabled {
		autoInstallParam = " autoinstall"
	}

	// Resolve boot params with placeholder substitution
	bootParams := mb.resolveBootParams(img, baseURL, encodedFilename, cacheDir)
	if bootParams != "" {
		bootParams = " " + bootParams
	}

	switch img.Distro {
	case "windows":
		sb.WriteString("echo Loading Windows boot files via wimboot...\n")
		sb.WriteString(fmt.Sprintf("kernel %s/wimboot\n", baseURL))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/iso/boot/bcd BCD || initrd %s/boot/%s/iso/BOOT/BCD BCD\n", baseURL, cacheDir, baseURL, cacheDir))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/iso/boot/boot.sdi boot.sdi || initrd %s/boot/%s/iso/BOOT/BOOT.SDI boot.sdi\n", baseURL, cacheDir, baseURL, cacheDir))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/iso/sources/boot.wim boot.wim || initrd %s/boot/%s/iso/SOURCES/BOOT.WIM boot.wim\n", baseURL, cacheDir, baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")

	default:
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/vmlinuz%s%s\n", baseURL, cacheDir, autoInstallParam, bootParams))
		sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/initrd\n", baseURL, cacheDir))
		sb.WriteString("boot || goto failed\n")
	}

	return sb.String()
}

// resolveBootParams returns the kernel boot parameters for an image.
// If the image has user-set boot_params, those are used with placeholder substitution.
// Otherwise, defaults are looked up from distro profiles, falling back to a generic default.
func (mb *MenuBuilder) resolveBootParams(img *models.Image, baseURL, encodedFilename, cacheDir string) string {
	params := img.BootParams

	// If no user params, look up from distro profile
	if params == "" && mb.profileManager != nil && img.Distro != "" {
		hasSquashfs := img.SquashfsPath != ""
		params = mb.profileManager.GetBootParams(img.Distro, hasSquashfs)
	}

	// Final fallback — generic default
	if params == "" {
		params = fmt.Sprintf("iso-url=%s/isos/%s ip=dhcp", baseURL, encodedFilename)
	}

	// Substitute placeholders
	params = strings.ReplaceAll(params, "{{BASE_URL}}", baseURL)
	params = strings.ReplaceAll(params, "{{CACHE_DIR}}", cacheDir)
	params = strings.ReplaceAll(params, "{{FILENAME}}", encodedFilename)
	if img.SquashfsPath != "" {
		params = strings.ReplaceAll(params, "{{SQUASHFS}}", fmt.Sprintf("%s/boot/%s/%s", baseURL, cacheDir, img.SquashfsPath))
	}

	return strings.TrimSpace(params)
}

func (mb *MenuBuilder) buildFooter() string {
	var sb strings.Builder

	// Tools submenu
	if len(mb.enabledTools) > 0 {
		sb.WriteString(":tools\n")
		sb.WriteString(fmt.Sprintf("menu %s - Tools\n", mb.menuTitle()))
		for _, t := range mb.enabledTools {
			sb.WriteString(fmt.Sprintf("item tool-%s %s\n", t.Name, t.DisplayName))
		}
		sb.WriteString("item --gap --\n")
		sb.WriteString("item back << Back to main menu\n")
		sb.WriteString("choose selected || goto start\n")
		sb.WriteString("goto ${selected}\n\n")

		sb.WriteString(":back\n")
		sb.WriteString("goto start\n\n")
	}

	// Tool boot sections
	for _, t := range mb.enabledTools {
		sb.WriteString(fmt.Sprintf(":tool-%s\n", t.Name))
		sb.WriteString(fmt.Sprintf("echo Booting %s...\n", t.DisplayName))

		switch t.BootMethod {
		case "chain":
			sb.WriteString(fmt.Sprintf("chain %s || goto failed\n\n", t.KernelURL))
		case "memdisk":
			sb.WriteString(fmt.Sprintf("initrd %s\n", t.KernelURL))
			sb.WriteString("chain memdisk raw || goto failed\n\n")
		default: // "kernel"
			sb.WriteString(fmt.Sprintf("kernel %s %s\n", t.KernelURL, t.BootParams))
			if t.InitrdURL != "" {
				sb.WriteString(fmt.Sprintf("initrd %s\n", t.InitrdURL))
			}
			sb.WriteString("boot || goto failed\n\n")
		}
	}

	sb.WriteString(`:shell
echo Dropping to iPXE shell...
shell

:reboot
reboot

:failed
echo Boot failed, returning to menu in 5 seconds...
sleep 5
goto start
`)
	return sb.String()
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
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (mb *MenuBuilder) groupHasImages(groupID uint) bool {
	if len(mb.getGroupImages(groupID)) > 0 {
		return true
	}
	for _, child := range mb.getChildGroups(groupID) {
		if child.Enabled && mb.groupHasImages(child.ID) {
			return true
		}
	}
	return false
}

func (mb *MenuBuilder) getGroupImages(groupID uint) []models.Image {
	var result []models.Image
	for _, img := range mb.images {
		if img.GroupID != nil && *img.GroupID == groupID && img.Enabled {
			result = append(result, img)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
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
