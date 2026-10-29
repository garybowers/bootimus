package server

import (
	"bootimus/internal/autoinstall"
	"bootimus/internal/bsdboot"
	"bootimus/internal/diskimage"
	"bootimus/internal/imagescan"
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
	images            []models.Image
	groups            []*models.ImageGroup
	theme             *models.MenuTheme
	macAddress        string
	serverAddr        string
	httpPort          int
	nfsPort           int
	groupStack        []uint
	enabledTools      []tools.EnabledTool
	nextBootImageID   uint
	profileManager    *profiles.Manager
	autoInstallTypes  map[uint]string
	forceLocalDefault bool
	remoteBase        string
	remoteImages      map[uint]bool
	diskBoot          *diskimage.Boot
	sanTarget         *models.SANTarget
	sanDefault        bool
	nextBootSAN       bool
	profileImageID    uint
	bootAuthURL       string
	bootAccess        bootMenuAccess
}

func (s *Server) resolveSANTarget(client *models.Client) (*models.SANTarget, bool) {
	if client == nil || s.config.Storage == nil {
		return nil, false
	}
	eff, _ := s.effectiveFor(client)
	targetID := eff.SANTargetID
	bootDefault := eff.BootFromSAN
	if targetID == nil {
		return nil, false
	}
	target, err := s.config.Storage.GetSANTarget(*targetID)
	if err != nil {
		log.Printf("Warning: SAN target %d for client %s not found: %v", *targetID, client.MACAddress, err)
		return nil, false
	}
	return target, bootDefault
}

func (s *Server) generateIPXEMenuWithAccess(images []models.Image, macAddress string, nbID uint, nextBootSANID uint, access bootMenuAccess) string {
	var groups []*models.ImageGroup
	var theme *models.MenuTheme
	var client *models.Client

	if s.config.Storage != nil {
		var err error
		groups, err = s.config.Storage.ListImageGroups()
		if err != nil {
			log.Printf("Warning: Failed to load image groups: %v", err)
			groups = nil
		}

		theme, err = s.config.Storage.GetMenuTheme()
		if err != nil {
			log.Printf("Warning: Failed to load menu theme: %v", err)
			return bootProtectionUnavailable()
		}

		if c, err := s.config.Storage.GetClient(macAddress); err == nil {
			client = c
		}
	}

	var enabledTools []tools.EnabledTool
	if s.toolsManager != nil {
		serverURL := fmt.Sprintf("http://%s:%d", s.config.ServerAddr, s.config.HTTPPort)
		enabledTools = s.toolsManager.GetEnabledTools(serverURL)
	}

	sanTarget, sanDefault := s.resolveSANTarget(client)
	nextBootSAN := false
	if nextBootSANID != 0 && s.config.Storage != nil {
		if t, err := s.config.Storage.GetSANTarget(nextBootSANID); err == nil {
			sanTarget = t
			nextBootSAN = true
		}
	}

	var diskBoot *diskimage.Boot
	if s.diskImages != nil {
		if job := s.diskImages.ActiveJob(macAddress); job != nil {
			boot, err := s.diskImages.Boot(job, enabledTools)
			if err != nil {
				log.Printf("Disk imaging: cannot boot job %d for %s: %v", job.ID, macAddress, err)
			} else {
				diskBoot = boot
			}
		}
	}

	var profileImageID uint
	if client != nil {
		eff, _ := s.effectiveFor(client)
		profileImageID = s.profileInstallImageID(client, eff, images)
	}

	autoInstallTypes := make(map[uint]string)
	for i := range images {
		if images[i].BootMethod != "kernel" {
			continue
		}
		if t := s.resolveAutoInstallType(&images[i], client); t != "" {
			autoInstallTypes[images[i].ID] = t
		}
	}

	mb := &MenuBuilder{
		images:            images,
		groups:            groups,
		theme:             theme,
		macAddress:        macAddress,
		serverAddr:        s.config.ServerAddr,
		httpPort:          s.config.HTTPPort,
		nfsPort:           s.config.NFSPort,
		enabledTools:      enabledTools,
		nextBootImageID:   nbID,
		profileManager:    s.config.ProfileManager,
		autoInstallTypes:  autoInstallTypes,
		forceLocalDefault: client != nil && client.EnrollmentState == models.EnrollmentStateInstalled,
		diskBoot:          diskBoot,
		sanTarget:         sanTarget,
		sanDefault:        sanDefault,
		nextBootSAN:       nextBootSAN,
		profileImageID:    profileImageID,
		bootAuthURL:       s.bootAuthenticationURL(),
		bootAccess:        access,
	}
	mb.remoteBase, mb.remoteImages = s.remoteImages(images)

	return mb.Build()
}

func (mb *MenuBuilder) Build() string {
	var sb strings.Builder

	sb.WriteString("#!ipxe\n\n")
	if mb.bootAccess.imageID != 0 {
		fmt.Fprintf(&sb, "goto iso%d\n", mb.bootAccess.imageID)
		sb.WriteString(mb.buildImageBootSections())
		sb.WriteString(":uki_needs_uefi\necho This image needs UEFI.\n:failed\necho Boot failed. Return to the menu and try again.\nsleep 2\n:start\nexit\n")
		return sb.String()
	}
	if mb.bootAccess.session != "" {
		fmt.Fprintf(&sb, "set bootimus_session %s\n", mb.bootAccess.session)
	}
	if mb.diskBoot != nil {
		sb.WriteString("goto diskimaging\n\n")
	} else if mb.hasEnabledImage(mb.nextBootImageID) {
		sb.WriteString(fmt.Sprintf("goto iso%d\n\n", mb.nextBootImageID))
	} else if mb.nextBootSAN && mb.sanTarget != nil {
		sb.WriteString("goto san\n\n")
	}
	sb.WriteString(mb.buildMainMenu())
	sb.WriteString(mb.buildGroupMenus())
	sb.WriteString(mb.buildImageBootSections())
	sb.WriteString(mb.buildDiskImagingSection())
	sb.WriteString(mb.buildFooter())

	return sb.String()
}

func (mb *MenuBuilder) buildDiskImagingSection() string {
	if mb.diskBoot == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(":diskimaging\n")
	sb.WriteString(fmt.Sprintf("echo %s...\n", mb.diskBoot.Label))
	params := mb.diskBoot.Params
	if !hasInitrdParam(params) {
		params = strings.TrimSpace("initrd=initrd " + params)
	}
	sb.WriteString(fmt.Sprintf("kernel %s %s\n", mb.diskBoot.KernelURL, params))
	sb.WriteString(fmt.Sprintf("initrd --name initrd %s\n", mb.diskBoot.InitrdURL))
	sb.WriteString("boot || goto failed\n\n")
	return sb.String()
}

func (mb *MenuBuilder) hasEnabledImage(id uint) bool {
	if id == 0 {
		return false
	}
	for i := range mb.images {
		if mb.images[i].ID == id && mb.images[i].Enabled {
			return true
		}
	}
	return false
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

func (mb *MenuBuilder) resolveDefaultItem(visibleGroups []*models.ImageGroup, ungroupedImages []models.Image) string {
	if mb.nextBootImageID > 0 {
		return fmt.Sprintf("iso%d", mb.nextBootImageID)
	}
	if mb.sanTarget != nil && (mb.nextBootSAN || mb.sanDefault) {
		return "san"
	}
	if mb.hasEnabledImage(mb.profileImageID) {
		return fmt.Sprintf("iso%d", mb.profileImageID)
	}
	if mb.forceLocalDefault {
		return "local"
	}
	if mb.theme != nil {
		switch mb.theme.DefaultMenuItem {
		case "local", "shell", "reboot":
			return mb.theme.DefaultMenuItem
		}
	}
	if len(visibleGroups) > 0 {
		return fmt.Sprintf("group%d", visibleGroups[0].ID)
	}
	if len(ungroupedImages) > 0 {
		return fmt.Sprintf("iso%d", ungroupedImages[0].ID)
	}
	return "local"
}

func (mb *MenuBuilder) menuTitle() string {
	if mb.theme != nil && mb.theme.Title != "" {
		return mb.theme.Title
	}
	return "Bootimus - Boot Menu"
}

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

	if mb.sanTarget != nil {
		sb.WriteString("item --gap -- SAN:\n")
		sb.WriteString(fmt.Sprintf("item san Boot from SAN: %s\n", mb.sanTarget.Name))
	}

	sb.WriteString("item --gap -- Options:\n")
	if mb.bootAccess.session != "" {
		sb.WriteString("item lock Lock network-boot menu\n")
	}
	sb.WriteString("item local Boot from Local Disk\n")
	sb.WriteString("item shell Drop to iPXE shell\n")
	sb.WriteString("item reboot Reboot\n")
	defaultItem := mb.resolveDefaultItem(visibleGroups, ungroupedImages)

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
		sb.WriteString("item local Boot from Local Disk\n")
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
		if (img.BootPasswordRequired || img.BootPasswordHash != "") && mb.bootAccess.imageID != img.ID {
			sb.WriteString(mb.buildImagePasswordGate(img.ID))
			continue
		}
		sb.WriteString(fmt.Sprintf("echo Booting %s...\n", img.Name))

		encodedFilename := encodePathSegments(img.Filename)
		cacheDir := encodePathSegments(strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename)))

		switch img.BootMethod {
		case "nbd":
			sb.WriteString("echo Using NBD (Network Block Device) mount...\n")
			sb.WriteString(fmt.Sprintf("kernel http://%s:%d/bootenv/vmlinuz-lts\n", mb.serverAddr, mb.httpPort))
			sb.WriteString(fmt.Sprintf("initrd http://%s:%d/bootenv/initramfs-bootimus\n", mb.serverAddr, mb.httpPort))
			sb.WriteString(fmt.Sprintf("imgargs vmlinuz-lts init=/init iso=%s server=%s nbdport=10809 console=tty0 console=ttyS0\n", encodedFilename, mb.serverAddr))
			sb.WriteString("boot || goto failed\n")

		case "nfs":
			sb.WriteString("echo Using NFS root (streamed, low memory)...\n")
			nfsPath := strings.TrimSuffix(img.Filename, filepath.Ext(img.Filename))
			sb.WriteString(fmt.Sprintf("kernel http://%s:%d/boot/%s/vmlinuz initrd=initrd root=/dev/nfs boot=casper netboot=nfs nfsroot=%s:/%s/iso,vers=3,tcp,port=%d,mountport=%d,nolock ip=dhcp\n", mb.serverAddr, mb.httpPort, cacheDir, mb.serverAddr, nfsPath, mb.nfsPort, mb.nfsPort))
			sb.WriteString(fmt.Sprintf("initrd http://%s:%d/boot/%s/initrd\n", mb.serverAddr, mb.httpPort, cacheDir))
			sb.WriteString("boot || goto failed\n")

		case "kernel":
			sb.WriteString("echo Loading kernel and initrd...\n")
			if mb.autoInstallTypes[img.ID] != "" {
				sb.WriteString("echo Auto-install enabled for this image\n")
			}

			sb.WriteString(mb.buildKernelBootSection(&img, encodedFilename, cacheDir))

		case imagescan.BootMethodUKI:
			baseURL := mb.assetBase(&img)
			params := mb.resolveBootParams(&img, baseURL, encodedFilename, cacheDir)
			if params != "" {
				params = " " + params
			}
			sb.WriteString("iseq ${platform} efi || goto uki_needs_uefi\n")
			sb.WriteString(fmt.Sprintf("kernel %s/isos/%s?mac=%s%s\n", baseURL, encodedFilename, mb.macAddress, params))
			sb.WriteString("boot || goto failed\n")

		default:
			sb.WriteString(fmt.Sprintf("sanboot --no-describe --drive 0x80 %s/isos/%s?mac=%s\n", mb.assetBase(&img), encodedFilename, mb.macAddress))
		}

		if mb.bootAccess.imageID != 0 {
			sb.WriteString("exit\n")
		} else if img.GroupID != nil {
			sb.WriteString(fmt.Sprintf("goto group%d\n", *img.GroupID))
		} else {
			sb.WriteString("goto start\n")
		}
	}

	return sb.String()
}

func (mb *MenuBuilder) buildKernelBootSection(img *models.Image, encodedFilename, cacheDir string) string {
	var sb strings.Builder

	localURL := fmt.Sprintf("http://%s:%d", mb.serverAddr, mb.httpPort)
	baseURL := mb.assetBase(img)

	autoInstallParam := mb.buildAutoInstallParam(img, localURL, encodedFilename)

	bootParams := mb.resolveBootParams(img, baseURL, encodedFilename, cacheDir)
	if strings.Contains(autoInstallParam, "ds=nocloud-net") {
		fields := strings.Fields(bootParams)
		kept := fields[:0]
		for _, f := range fields {
			if f != "ds=nocloud" {
				kept = append(kept, f)
			}
		}
		bootParams = strings.Join(kept, " ")
	}
	if bootParams != "" {
		bootParams = " " + bootParams
	}

	switch {
	case bsdboot.IsBSD(img.Distro):
		sb.WriteString(fmt.Sprintf("echo %s boots through its own network loader served over TFTP\n", bsdboot.DisplayName(img.Distro)))
		sb.WriteString(fmt.Sprintf("chain %s/bsd/%s/boot.ipxe?mac=%s&platform=${platform}&buildarch=${buildarch} || goto failed\n", baseURL, cacheDir, mb.macAddress))

	case models.IsWindowsDistro(img.Distro):
		bootParams = wimbootArgs(bootParams)
		if img.WIMIndex > 0 && !hasParam(bootParams, "index") {
			bootParams = strings.TrimSpace(bootParams + fmt.Sprintf(" index=%d", img.WIMIndex))
		}
		if bootParams != "" {
			bootParams = " " + bootParams
		}
		sb.WriteString("echo Loading Windows boot files via wimboot...\n")
		sb.WriteString(fmt.Sprintf("kernel %s/wimboot%s\n", localURL, bootParams))
		if mb.autoInstallTypes[img.ID] == "autounattend" {
			sb.WriteString(fmt.Sprintf("initrd %s/autoinstall/%s?mac=%s AutoUnattend.xml\n", localURL, encodedFilename, mb.macAddress))
		}
		// Ship only boot.wim and let wimboot synthesize the ramdisk BCD +
		// boot.sdi (the documented minimal setup). Feeding the ISO's DVD BCD
		// hangs 24H2/25H2 media on a black screen after the loading bar.
		if img.WIMPath != "" {
			sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/iso/%s boot.wim\n", baseURL, cacheDir, encodePathSegments(img.WIMPath)))
		} else {
			sb.WriteString(fmt.Sprintf("initrd %s/boot/%s/iso/sources/boot.wim boot.wim || initrd %s/boot/%s/iso/SOURCES/BOOT.WIM boot.wim\n", baseURL, cacheDir, baseURL, cacheDir))
		}
		sb.WriteString("boot || goto failed\n")

	default:
		kernelPath := "vmlinuz"
		if img.KernelOverride != "" {
			kernelPath = encodePathSegments(img.KernelOverride)
		}
		initrdPath := "initrd"
		initrdName := ""
		if img.InitrdOverride != "" {
			initrdPath = encodePathSegments(img.InitrdOverride)
			initrdName = "--name initrd "
		}
		sb.WriteString(fmt.Sprintf("kernel %s/boot/%s/%s%s%s\n", baseURL, cacheDir, kernelPath, autoInstallParam, bootParams))
		sb.WriteString(fmt.Sprintf("initrd %s%s/boot/%s/%s\n", initrdName, baseURL, cacheDir, initrdPath))
		if img.Distro == autoinstall.DistroProxmox {
			sb.WriteString(fmt.Sprintf("initrd %s/isos/%s?mac=%s proxmox.iso\n", baseURL, encodedFilename, mb.macAddress))
		}
		if img.ShimPath != "" {
			sb.WriteString(fmt.Sprintf("iseq ${platform} efi && shim %s/boot/%s/%s ||\n", baseURL, cacheDir, encodePathSegments(img.ShimPath)))
		}
		sb.WriteString("boot || goto failed\n")
	}

	return sb.String()
}

func (mb *MenuBuilder) buildAutoInstallParam(img *models.Image, baseURL, encodedFilename string) string {
	scriptType := mb.autoInstallTypes[img.ID]
	if scriptType == "" {
		return ""
	}

	installURL := fmt.Sprintf("%s/autoinstall/%s?mac=%s", baseURL, encodedFilename, mb.macAddress)

	switch scriptType {
	case autoinstall.TypePreseed:
		return fmt.Sprintf(" auto=true priority=critical url=%s", installURL)
	case autoinstall.TypeKickstart:
		return fmt.Sprintf(" inst.ks=%s", installURL)
	case autoinstall.TypeAutoinstall:
		return fmt.Sprintf(" autoinstall ds=nocloud-net;s=%s/autoinstall/%s/mac/%s/", baseURL, encodedFilename, mb.macAddress)
	case autoinstall.TypeAutoYaST:
		return fmt.Sprintf(" autoyast=%s", installURL)
	case autoinstall.TypeAgama:
		return fmt.Sprintf(" inst.auto=%s", installURL)
	case autoinstall.TypeIgnition:
		if img.Distro == autoinstall.DistroFlatcar {
			return fmt.Sprintf(" flatcar.first_boot=1 ignition.config.url=%s", installURL)
		}
		return fmt.Sprintf(" ignition.config.url=%s ignition.firstboot ignition.platform.id=metal", installURL)
	default:
		return ""
	}
}

var wimbootKeys = map[string]bool{
	"rawbcd":     true,
	"rawwim":     true,
	"gui":        true,
	"linear":     true,
	"quiet":      true,
	"pause":      true,
	"index":      true,
	"initrdfile": true,
}

func wimbootArgs(params string) string {
	var kept []string
	for _, f := range strings.Fields(params) {
		key, _, _ := strings.Cut(f, "=")
		if wimbootKeys[key] {
			kept = append(kept, f)
		}
	}
	return strings.Join(kept, " ")
}

func hasInitrdParam(params string) bool {
	return hasParam(params, "initrd")
}

func hasParam(params, key string) bool {
	for _, f := range strings.Fields(params) {
		if strings.HasPrefix(f, key+"=") {
			return true
		}
	}
	return false
}

func (mb *MenuBuilder) resolveBootParams(img *models.Image, baseURL, encodedFilename, cacheDir string) string {
	params := img.BootParams

	if params == "" && mb.profileManager != nil && img.Distro != "" {
		hasSquashfs := img.SquashfsPath != ""
		params = mb.profileManager.GetBootParams(img.Distro, hasSquashfs)
	}

	if params == "" && !models.IsWindowsDistro(img.Distro) && img.BootMethod != imagescan.BootMethodUKI {
		params = fmt.Sprintf("iso-url=%s/isos/%s ip=dhcp", baseURL, encodedFilename)
	}

	params = strings.ReplaceAll(params, "{{BASE_URL}}", baseURL)
	params = strings.ReplaceAll(params, "{{CACHE_DIR}}", cacheDir)
	params = strings.ReplaceAll(params, "{{FILENAME}}", encodedFilename)
	params = strings.ReplaceAll(params, "{{IMAGE_FILENAME}}", encodedFilename)
	params = strings.ReplaceAll(params, "{{SERVER_ADDR}}", mb.serverAddr)
	params = strings.ReplaceAll(params, "{{MAC}}", mb.macAddress)
	if img.SquashfsPath != "" {
		params = strings.ReplaceAll(params, "{{SQUASHFS}}", fmt.Sprintf("%s/boot/%s/%s", baseURL, cacheDir, img.SquashfsPath))
	}

	return params
}

func (mb *MenuBuilder) buildFooter() string {
	var sb strings.Builder

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

	for _, t := range mb.enabledTools {
		sb.WriteString(fmt.Sprintf(":tool-%s\n", t.Name))
		sb.WriteString(fmt.Sprintf("echo Booting %s...\n", t.DisplayName))

		switch t.BootMethod {
		case "chain":
			if t.KernelURLBIOS != "" {
				sb.WriteString(fmt.Sprintf("iseq ${platform} efi && chain %s || chain %s || goto failed\n\n", t.KernelURL, t.KernelURLBIOS))
			} else {
				sb.WriteString(fmt.Sprintf("chain %s || goto failed\n\n", t.KernelURL))
			}
		case "memdisk":
			sb.WriteString(fmt.Sprintf("initrd %s\n", t.KernelURL))
			sb.WriteString("chain memdisk raw || goto failed\n\n")
		default:
			params := t.BootParams
			if t.InitrdURL != "" && !hasInitrdParam(params) {
				params = strings.TrimSpace("initrd=initrd " + params)
			}
			sb.WriteString(fmt.Sprintf("kernel %s %s\n", t.KernelURL, params))
			if t.InitrdURL != "" {
				sb.WriteString(fmt.Sprintf("initrd --name initrd %s\n", t.InitrdURL))
			}
			sb.WriteString("boot || goto failed\n\n")
		}
	}

	sb.WriteString(mb.buildSANBootSection())
	if mb.bootAccess.session != "" {
		sb.WriteString(":lock\nclear bootimus_password\nclear bootimus_session\nexit\n")
	}

	sb.WriteString(`:local
clear bootimus_password
clear bootimus_session
echo Booting from local disk...
set localboot 1
iseq ${platform} pcbios && sanboot --no-describe --drive 0x80 ||
exit

:shell
clear bootimus_password
clear bootimus_session
echo Dropping to iPXE shell...
shell

:reboot
clear bootimus_password
clear bootimus_session
reboot

:uki_needs_uefi
echo This image is a unified kernel image and needs a UEFI client.
goto failed

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

func (mb *MenuBuilder) buildSANBootSection() string {
	if mb.sanTarget == nil {
		return ""
	}
	t := mb.sanTarget
	var sb strings.Builder
	sb.WriteString(":san\n")
	sb.WriteString(fmt.Sprintf("echo Booting from SAN target %s...\n", t.Name))
	if iqn := t.ResolveInitiatorIQN(mb.macAddress); iqn != "" {
		sb.WriteString(fmt.Sprintf("set initiator-iqn %s\n", iqn))
	}
	if t.CHAPUsername != "" {
		sb.WriteString(fmt.Sprintf("set username %s\n", t.CHAPUsername))
		sb.WriteString(fmt.Sprintf("set password %s\n", t.CHAPPassword))
	}
	if t.ReverseCHAPUsername != "" {
		sb.WriteString(fmt.Sprintf("set reverse-username %s\n", t.ReverseCHAPUsername))
		sb.WriteString(fmt.Sprintf("set reverse-password %s\n", t.ReverseCHAPPassword))
	}
	sb.WriteString(fmt.Sprintf("sanboot --no-describe --drive 0x80 %s || goto failed\n\n", t.URI))
	return sb.String()
}

func (mb *MenuBuilder) assetBase(img *models.Image) string {
	if mb.remoteBase != "" && img != nil && mb.remoteImages[img.ID] {
		return mb.remoteBase
	}
	return fmt.Sprintf("http://%s:%d", mb.serverAddr, mb.httpPort)
}

const ipxeLocalBootCheck = "isset ${localboot} && goto localboot ||\n"

const ipxeLocalBootTail = `
:localboot
isset ${bootimus-embed} && exit ||
echo Returning to the firmware boot order...
exit 1
`
