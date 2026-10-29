package replica

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bootimus/internal/autoinstall"
	"bootimus/internal/models"
)

func (s *Syncer) syncSANTargets(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.SANTarget
	if err := s.primary.getJSON(ctx, "/api/san-targets", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListSANTargets()
	if err != nil {
		return KindResult{}, err
	}
	byName := map[string]*models.SANTarget{}
	for _, t := range local {
		byName[t.Name] = t
	}
	res := KindResult{Count: len(remote)}
	seen := map[string]bool{}
	for i := range remote {
		r := remote[i]
		seen[r.Name] = true
		if l, ok := byName[r.Name]; ok {
			changed := l.Description != r.Description || l.URI != r.URI || l.InitiatorIQN != r.InitiatorIQN ||
				l.CHAPUsername != r.CHAPUsername || l.ReverseCHAPUsername != r.ReverseCHAPUsername ||
				(r.CHAPPassword != "" && l.CHAPPassword != r.CHAPPassword) || (r.ReverseCHAPPassword != "" && l.ReverseCHAPPassword != r.ReverseCHAPPassword)
			run.sanTargets[r.ID] = l.ID
			if !changed {
				continue
			}
			upd := *l
			upd.Description, upd.URI, upd.InitiatorIQN = r.Description, r.URI, r.InitiatorIQN
			upd.CHAPUsername, upd.ReverseCHAPUsername = r.CHAPUsername, r.ReverseCHAPUsername
			if r.CHAPPassword != "" {
				upd.CHAPPassword = r.CHAPPassword
			}
			if r.ReverseCHAPPassword != "" {
				upd.ReverseCHAPPassword = r.ReverseCHAPPassword
			}
			if err := s.deps.Store.UpdateSANTarget(l.ID, &upd); err != nil {
				return res, err
			}
			res.Changed++
			continue
		}
		created := models.SANTarget{Name: r.Name, Description: r.Description, URI: r.URI, InitiatorIQN: r.InitiatorIQN,
			CHAPUsername: r.CHAPUsername, CHAPPassword: r.CHAPPassword, ReverseCHAPUsername: r.ReverseCHAPUsername, ReverseCHAPPassword: r.ReverseCHAPPassword}
		if err := s.deps.Store.CreateSANTarget(&created); err != nil {
			return res, err
		}
		run.sanTargets[r.ID] = created.ID
		res.Changed++
	}
	for _, l := range local {
		if seen[l.Name] {
			continue
		}
		if err := s.deps.Store.DeleteSANTarget(l.ID); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

func (s *Syncer) syncImageGroups(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.ImageGroup
	if err := s.primary.getJSON(ctx, "/api/groups", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListImageGroups()
	if err != nil {
		return KindResult{}, err
	}
	remoteByID := map[uint]models.ImageGroup{}
	for _, g := range remote {
		remoteByID[g.ID] = g
	}
	depth := func(g models.ImageGroup) int {
		d := 0
		for g.ParentID != nil {
			parent, ok := remoteByID[*g.ParentID]
			if !ok || d > 32 {
				break
			}
			g = parent
			d++
		}
		return d
	}
	sort.SliceStable(remote, func(i, j int) bool { return depth(remote[i]) < depth(remote[j]) })

	findLocal := func(name string, parent *uint) *models.ImageGroup {
		for _, l := range local {
			if l.Name != name {
				continue
			}
			if (l.ParentID == nil) != (parent == nil) {
				continue
			}
			if parent != nil && *l.ParentID != *parent {
				continue
			}
			return l
		}
		return nil
	}

	res := KindResult{Count: len(remote)}
	matched := map[uint]bool{}
	for _, r := range remote {
		var parent *uint
		if r.ParentID != nil {
			if mapped, ok := run.imageGroups[*r.ParentID]; ok {
				parent = &mapped
			}
		}
		if l := findLocal(r.Name, parent); l != nil {
			matched[l.ID] = true
			run.imageGroups[r.ID] = l.ID
			if l.Description != r.Description || l.Order != r.Order || l.Enabled != r.Enabled {
				upd := *l
				upd.Parent = nil
				upd.Description, upd.Order, upd.Enabled = r.Description, r.Order, r.Enabled
				if err := s.deps.Store.UpdateImageGroup(l.ID, &upd); err != nil {
					return res, err
				}
				res.Changed++
			}
			continue
		}
		created := models.ImageGroup{Name: r.Name, Description: r.Description, ParentID: parent, Order: r.Order, Enabled: r.Enabled}
		if err := s.deps.Store.CreateImageGroup(&created); err != nil {
			return res, err
		}
		local = append(local, &created)
		matched[created.ID] = true
		run.imageGroups[r.ID] = created.ID
		res.Changed++
	}
	for _, l := range local {
		if matched[l.ID] {
			continue
		}
		if err := s.deps.Store.DeleteImageGroup(l.ID); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

func imageCacheDir(isoDir, filename string) string {
	return filepath.Join(isoDir, filepath.FromSlash(strings.TrimSuffix(filename, filepath.Ext(filename))))
}

func copyImageMetadata(dst *models.Image, src models.Image, groupID *uint, bootPasswordHash string) {
	dst.Name = src.Name
	dst.Description = src.Description
	dst.Size = src.Size
	dst.Enabled = src.Enabled
	dst.Public = src.Public
	dst.BootPasswordRequired = src.BootPasswordRequired
	dst.BootPasswordHash = bootPasswordHash
	dst.BootPasswordRevision = src.BootPasswordRevision
	dst.BootPasswordSourceAt = src.UpdatedAt
	dst.GroupID = groupID
	dst.Group = nil
	dst.Order = src.Order
	dst.Distro = src.Distro
	dst.BootMethod = src.BootMethod
	dst.BootParams = src.BootParams
	dst.KernelOverride = src.KernelOverride
	dst.InitrdOverride = src.InitrdOverride
	dst.SanbootCompatible = src.SanbootCompatible
	dst.SanbootHint = src.SanbootHint
	dst.NetbootRequired = src.NetbootRequired
	dst.NetbootURL = src.NetbootURL
	dst.AutoInstallScript = src.AutoInstallScript
	dst.AutoInstallEnabled = src.AutoInstallEnabled
	dst.AutoInstallScriptType = src.AutoInstallScriptType
	dst.AutoInstallFile = src.AutoInstallFile
	dst.WIMPath = src.WIMPath
	dst.WIMIndex = src.WIMIndex
	dst.WIMFiles = src.WIMFiles
	dst.SquashfsPath = src.SquashfsPath
	dst.ShimPath = src.ShimPath
	dst.InstallWimPath = src.InstallWimPath
}

func (s *Syncer) syncImages(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.Image
	if err := s.primary.getJSON(ctx, "/api/images", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListImages()
	if err != nil {
		return KindResult{}, err
	}
	needsVerifiers := false
	for _, image := range remote {
		needsVerifiers = needsVerifiers || image.BootPasswordRequired
	}
	for _, image := range local {
		needsVerifiers = needsVerifiers || image.BootPasswordRequired || image.BootPasswordHash != ""
	}
	bootHashes := map[string]string{}
	if needsVerifiers {
		feed, err := s.bootVerifiers(ctx)
		if err != nil {
			if guardErr := s.guardImageBootPolicies(remote, local); guardErr != nil {
				return KindResult{}, guardErr
			}
			return KindResult{}, err
		}
		bootHashes, err = feed.imageHashes(remote)
		if err != nil {
			if guardErr := s.guardImageBootPolicies(remote, local); guardErr != nil {
				return KindResult{}, guardErr
			}
			return KindResult{}, err
		}
	}
	byFilename := map[string]*models.Image{}
	for _, img := range local {
		byFilename[img.Filename] = img
	}
	res := KindResult{Count: len(remote)}
	seen := map[string]bool{}
	var notes []string
	for _, r := range remote {
		seen[r.Filename] = true
		run.imageIDs[r.ID] = r.Filename
		var groupID *uint
		if r.GroupID != nil {
			if mapped, ok := run.imageGroups[*r.GroupID]; ok {
				groupID = &mapped
			}
		}
		l, exists := byFilename[r.Filename]
		if !exists {
			l = &models.Image{Filename: r.Filename, BootMethod: r.BootMethod}
		}
		downloaded := false
		haveFile := s.ImageIsLocal(r.Filename)
		if s.cfg.SyncISOs && r.Enabled {
			dest := filepath.Join(s.deps.ISODir, filepath.FromSlash(r.Filename))
			needs := !haveFile
			if haveFile {
				if info, err := os.Stat(dest); err == nil && r.Size > 0 && info.Size() != r.Size {
					needs = true
				}
			}
			if needs {
				if _, err := s.primary.download(ctx, assetPath("iso", map[string]string{"filename": r.Filename}), dest); err != nil {
					notes = append(notes, fmt.Sprintf("%s: %v", r.Filename, err))
					if isUnreachable(err) {
						res.Note = strings.Join(notes, "; ")
						return res, err
					}
				} else {
					downloaded = true
					haveFile = true
				}
			}
		}
		if !exists {
			copyImageMetadata(l, r, groupID, bootHashes[r.Filename])
			if err := s.deps.Store.CreateImage(l); err != nil {
				return res, err
			}
			res.Changed++
		}
		if haveFile && s.deps.Extract != nil && r.BootMethod == "kernel" && (downloaded || !l.Extracted) {
			if err := s.deps.Extract(r.Filename); err != nil {
				notes = append(notes, fmt.Sprintf("%s: extraction failed: %v", r.Filename, err))
			}
			if fresh, err := s.deps.Store.GetImage(r.Filename); err == nil {
				l = fresh
			}
		}
		before := *l
		copyImageMetadata(l, r, groupID, bootHashes[r.Filename])
		if !haveFile {
			l.Extracted = r.Extracted
			l.KernelPath = r.KernelPath
			l.InitrdPath = r.InitrdPath
			l.ExtractedAt = r.ExtractedAt
		}
		if exists && !imageEqual(before, *l) {
			res.Changed++
		}
		if err := s.deps.Store.UpdateImage(r.Filename, l); err != nil {
			return res, err
		}
	}
	for _, l := range local {
		if seen[l.Filename] {
			continue
		}
		if s.cfg.SyncISOs {
			os.Remove(filepath.Join(s.deps.ISODir, filepath.FromSlash(l.Filename)))
			os.RemoveAll(imageCacheDir(s.deps.ISODir, l.Filename))
		}
		if err := s.deps.Store.DeleteImage(l.Filename); err != nil {
			return res, err
		}
		res.Removed++
	}
	if len(notes) > 0 {
		res.Note = strings.Join(notes, "; ")
	}
	return res, nil
}

func imageEqual(a, b models.Image) bool {
	a.UpdatedAt = b.UpdatedAt
	a.Group, b.Group = nil, nil
	a.Clients, b.Clients = nil, nil
	return fmt.Sprintf("%+v", a) == fmt.Sprintf("%+v", b)
}

func (s *Syncer) syncClientGroups(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.ClientGroup
	if err := s.primary.getJSON(ctx, "/api/client-groups", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListClientGroups()
	if err != nil {
		return KindResult{}, err
	}
	byName := map[string]*models.ClientGroup{}
	for _, g := range local {
		byName[g.Name] = g
	}
	res := KindResult{Count: len(remote)}
	seen := map[string]bool{}
	for _, r := range remote {
		seen[r.Name] = true
		var san *uint
		if r.SANTargetID != nil {
			if mapped, ok := run.sanTargets[*r.SANTargetID]; ok {
				san = &mapped
			}
		}
		if l, ok := byName[r.Name]; ok {
			run.clientGroups[r.ID] = l.ID
			upd := *l
			upd.Clients = nil
			applyClientGroup(&upd, r, san)
			if clientGroupEqual(*l, upd) {
				continue
			}
			if err := s.deps.Store.UpdateClientGroup(l.ID, &upd); err != nil {
				return res, err
			}
			res.Changed++
			continue
		}
		created := models.ClientGroup{Name: r.Name}
		applyClientGroup(&created, r, san)
		created.IPMIPassword = r.IPMIPassword
		if err := s.deps.Store.CreateClientGroup(&created); err != nil {
			return res, err
		}
		run.clientGroups[r.ID] = created.ID
		res.Changed++
	}
	for _, l := range local {
		if seen[l.Name] {
			continue
		}
		if err := s.deps.Store.DeleteClientGroup(l.ID); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

func applyClientGroup(dst *models.ClientGroup, src models.ClientGroup, san *uint) {
	dst.Description = src.Description
	dst.Enabled = src.Enabled
	dst.AllowedImages = src.AllowedImages
	dst.BootloaderSet = src.BootloaderSet
	dst.WOLBroadcastAddr = src.WOLBroadcastAddr
	dst.StaggerDelayMillis = src.StaggerDelayMillis
	dst.BMCProtocol = src.BMCProtocol
	dst.IPMIPort = src.IPMIPort
	dst.IPMIUsername = src.IPMIUsername
	if src.IPMIPassword != "" {
		dst.IPMIPassword = src.IPMIPassword
	}
	dst.IPMIInsecure = src.IPMIInsecure
	dst.AutoInstallFile = src.AutoInstallFile
	dst.SANTargetID = san
	dst.BootFromSAN = src.BootFromSAN
}

func clientGroupEqual(a, b models.ClientGroup) bool {
	a.UpdatedAt = b.UpdatedAt
	a.Clients, b.Clients = nil, nil
	return fmt.Sprintf("%+v", a) == fmt.Sprintf("%+v", b)
}

func (s *Syncer) syncClients(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.Client
	if err := s.primary.getJSON(ctx, "/api/clients", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListClients()
	if err != nil {
		return KindResult{}, err
	}
	byMAC := map[string]*models.Client{}
	for _, c := range local {
		byMAC[strings.ToLower(c.MACAddress)] = c
	}
	res := KindResult{Count: len(remote)}
	seen := map[string]bool{}
	for _, r := range remote {
		mac := strings.ToLower(r.MACAddress)
		if mac == "" {
			continue
		}
		seen[mac] = true
		var group *uint
		if r.ClientGroupID != nil {
			if mapped, ok := run.clientGroups[*r.ClientGroupID]; ok {
				group = &mapped
			}
		}
		var san *uint
		if r.SANTargetID != nil {
			if mapped, ok := run.sanTargets[*r.SANTargetID]; ok {
				san = &mapped
			}
		}
		allowed := r.AllowedImages
		if len(allowed) == 0 && len(r.Images) > 0 {
			for _, img := range r.Images {
				allowed = append(allowed, img.Filename)
			}
		}
		if l, ok := byMAC[mac]; ok {
			upd := *l
			upd.Images, upd.ClientGroup, upd.Cluster = nil, nil, nil
			applyClient(&upd, r, group, san)
			changed := !clientEqual(*l, upd)
			if changed {
				if err := s.deps.Store.UpdateClient(mac, &upd); err != nil {
					return res, err
				}
			}
			if !stringSliceEqual(l.AllowedImages, allowed) {
				if err := s.deps.Store.AssignImagesToClient(mac, allowed); err != nil {
					return res, err
				}
				changed = true
			}
			if changed {
				res.Changed++
			}
			continue
		}
		created := models.Client{MACAddress: mac}
		applyClient(&created, r, group, san)
		created.IPMIPassword = r.IPMIPassword
		created.AllowedImages = allowed
		if err := s.deps.Store.CreateClient(&created); err != nil {
			return res, err
		}
		if len(allowed) > 0 {
			if err := s.deps.Store.AssignImagesToClient(mac, allowed); err != nil {
				return res, err
			}
		}
		res.Changed++
	}
	for _, l := range local {
		if seen[strings.ToLower(l.MACAddress)] || !l.Static {
			continue
		}
		if err := s.deps.Store.DeleteClient(l.MACAddress); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

func applyClient(dst *models.Client, src models.Client, group, san *uint) {
	dst.Name = src.Name
	dst.Description = src.Description
	dst.Enabled = src.Enabled
	dst.ShowPublicImages = src.ShowPublicImages
	dst.BootloaderSet = src.BootloaderSet
	dst.Static = src.Static
	dst.ClientGroupID = group
	dst.BMCProtocol = src.BMCProtocol
	dst.IPMIHost = src.IPMIHost
	dst.IPMIPort = src.IPMIPort
	dst.IPMIUsername = src.IPMIUsername
	if src.IPMIPassword != "" {
		dst.IPMIPassword = src.IPMIPassword
	}
	dst.IPMIInsecure = src.IPMIInsecure
	dst.AutoInstallFile = src.AutoInstallFile
	dst.ProxyDHCPNoVendorOption = src.ProxyDHCPNoVendorOption
	dst.SANTargetID = san
	dst.BootFromSAN = src.BootFromSAN
}

func clientEqual(a, b models.Client) bool {
	a.UpdatedAt = b.UpdatedAt
	a.Images, b.Images = nil, nil
	a.ClientGroup, b.ClientGroup = nil, nil
	a.Cluster, b.Cluster = nil, nil
	return fmt.Sprintf("%+v", a) == fmt.Sprintf("%+v", b)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Syncer) syncTheme(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote models.MenuTheme
	if err := s.primary.getJSON(ctx, "/api/theme", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.GetMenuTheme()
	if err != nil {
		return KindResult{}, err
	}
	if run.imagePolicyFailed && !remote.BootPasswordRequired {
		return KindResult{Count: 1, Skipped: true, Note: "image sync failed"}, nil
	}
	bootHash := ""
	if remote.BootPasswordRequired || local.BootPasswordRequired || local.BootPasswordHash != "" {
		feed, err := s.bootVerifiers(ctx)
		if err != nil {
			if guardErr := s.guardThemeBootPolicy(remote, local); guardErr != nil {
				return KindResult{}, guardErr
			}
			return KindResult{}, err
		}
		if !validVerifier(feed.Theme, remote.UpdatedAt, remote.BootPasswordRequired, remote.BootPasswordRevision) {
			if guardErr := s.guardThemeBootPolicy(remote, local); guardErr != nil {
				return KindResult{}, guardErr
			}
			return KindResult{}, fmt.Errorf("boot verifier feed does not match menu metadata")
		}
		bootHash = feed.Theme.Hash
	}
	res := KindResult{Count: 1}
	if local.Title == remote.Title && local.MenuTimeout == remote.MenuTimeout && local.DefaultMenuItem == remote.DefaultMenuItem && local.BootPasswordRequired == remote.BootPasswordRequired && local.BootPasswordHash == bootHash && local.BootPasswordRevision == remote.BootPasswordRevision && local.BootPasswordSourceAt.Equal(remote.UpdatedAt) {
		return res, nil
	}
	local.Title, local.MenuTimeout, local.DefaultMenuItem = remote.Title, remote.MenuTimeout, remote.DefaultMenuItem
	local.BootPasswordRequired, local.BootPasswordHash = remote.BootPasswordRequired, bootHash
	local.BootPasswordRevision = remote.BootPasswordRevision
	local.BootPasswordSourceAt = remote.UpdatedAt
	if err := s.deps.Store.UpdateMenuTheme(local); err != nil {
		return res, err
	}
	res.Changed = 1
	return res, nil
}

func (s *Syncer) syncProfiles(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.DistroProfile
	if err := s.primary.getJSON(ctx, "/api/profiles", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListDistroProfiles()
	if err != nil {
		return KindResult{}, err
	}
	byID := map[string]*models.DistroProfile{}
	for _, p := range local {
		byID[p.ProfileID] = p
	}
	res := KindResult{}
	seen := map[string]bool{}
	for _, r := range remote {
		if !r.Custom {
			continue
		}
		res.Count++
		seen[r.ProfileID] = true
		upd := r
		upd.ID = 0
		if l, ok := byID[r.ProfileID]; ok {
			upd.ID = l.ID
			upd.CreatedAt = l.CreatedAt
			cmp := *l
			cmp.UpdatedAt = upd.UpdatedAt
			if fmt.Sprintf("%+v", cmp) == fmt.Sprintf("%+v", upd) {
				continue
			}
		}
		if err := s.deps.Store.SaveDistroProfile(&upd); err != nil {
			return res, err
		}
		res.Changed++
	}
	for _, l := range local {
		if !l.Custom || seen[l.ProfileID] {
			continue
		}
		if err := s.deps.Store.DeleteDistroProfile(l.ProfileID); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

type toolFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func (s *Syncer) syncTools(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.BootTool
	if err := s.primary.getJSON(ctx, "/api/tools", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListBootTools()
	if err != nil {
		return KindResult{}, err
	}
	byName := map[string]*models.BootTool{}
	for _, t := range local {
		byName[t.Name] = t
	}
	res := KindResult{Count: len(remote)}
	seen := map[string]bool{}
	var notes []string
	for _, r := range remote {
		seen[r.Name] = true
		l, ok := byName[r.Name]
		if !ok {
			l = &models.BootTool{Name: r.Name}
		}
		upd := *l
		upd.DisplayName, upd.Description, upd.Version = r.DisplayName, r.Description, r.Version
		upd.Enabled, upd.Order, upd.DownloadURL, upd.Custom = r.Enabled, r.Order, r.DownloadURL, r.Custom
		upd.KernelPath, upd.InitrdPath, upd.BootParams, upd.BootMethod, upd.ArchiveType = r.KernelPath, r.InitrdPath, r.BootParams, r.BootMethod, r.ArchiveType
		upd.DownloadURLBIOS, upd.KernelPathBIOS = r.DownloadURLBIOS, r.KernelPathBIOS
		upd.Downloaded = l.Downloaded
		cmp := *l
		cmp.UpdatedAt = upd.UpdatedAt
		if !ok || fmt.Sprintf("%+v", cmp) != fmt.Sprintf("%+v", upd) {
			if err := s.deps.Store.SaveBootTool(&upd); err != nil {
				return res, err
			}
			res.Changed++
		}
		if r.Downloaded && r.Enabled && s.deps.ToolDir != nil {
			if err := s.syncToolFiles(ctx, r.Name); err != nil {
				notes = append(notes, fmt.Sprintf("%s: %v", r.Name, err))
				if isUnreachable(err) {
					res.Note = strings.Join(notes, "; ")
					return res, err
				}
			}
		}
	}
	for _, l := range local {
		if seen[l.Name] || !l.Custom {
			continue
		}
		if err := s.deps.Store.DeleteBootTool(l.Name); err != nil {
			return res, err
		}
		res.Removed++
	}
	if len(notes) > 0 {
		res.Note = strings.Join(notes, "; ")
	}
	return res, nil
}

func (s *Syncer) syncToolFiles(ctx context.Context, name string) error {
	var files []toolFile
	if err := s.primary.getJSON(ctx, "/api/sync/tool-files?name="+url.QueryEscape(name), &files); err != nil {
		return err
	}
	dir := s.deps.ToolDir(name)
	for _, f := range files {
		rel, err := safeRelative(f.Path)
		if err != nil {
			return err
		}
		dest := filepath.Join(dir, rel)
		if info, err := os.Stat(dest); err == nil && info.Size() == f.Size {
			continue
		}
		if _, err := s.primary.download(ctx, assetPath("tool", map[string]string{"name": name, "path": f.Path}), dest); err != nil {
			return err
		}
	}
	return nil
}

func safeRelative(p string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(p))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("unsafe path %q", p)
	}
	return clean, nil
}

func (s *Syncer) syncAutoInstallFiles(ctx context.Context, run *syncRun) (KindResult, error) {
	if s.deps.AutoInstall == nil {
		return KindResult{Skipped: true, Note: "auto-install library unavailable"}, nil
	}
	var remote []autoinstall.File
	if err := s.primary.getJSON(ctx, "/api/autoinstall-files", &remote); err != nil {
		if errors.Is(err, ErrForbidden) {
			return KindResult{Skipped: true, Note: "needs an admin-scope token on the primary"}, nil
		}
		return KindResult{}, err
	}
	local, err := s.deps.AutoInstall.List()
	if err != nil {
		return KindResult{}, err
	}
	res := KindResult{Count: len(remote)}
	seen := map[string]bool{}
	for _, r := range remote {
		seen[r.Path] = true
		var body struct {
			Content string `json:"content"`
		}
		q := url.Values{"distro": {r.Distro}, "filename": {r.Filename}}
		if err := s.primary.getJSON(ctx, "/api/autoinstall-files/get?"+q.Encode(), &body); err != nil {
			return res, err
		}
		if current, err := s.deps.AutoInstall.Read(r.Distro, r.Filename); err == nil && current == body.Content {
			continue
		}
		if err := s.deps.AutoInstall.Write(r.Distro, r.Filename, body.Content); err != nil {
			return res, err
		}
		res.Changed++
	}
	for _, l := range local {
		if seen[l.Path] {
			continue
		}
		if err := s.deps.AutoInstall.Delete(l.Distro, l.Filename); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

func (s *Syncer) customFilePath(f models.CustomFile, imageFilename string) string {
	if f.Public || imageFilename == "" {
		return filepath.Join(s.deps.DataDir, "files", f.Filename)
	}
	return filepath.Join(imageCacheDir(s.deps.ISODir, imageFilename), "files", f.Filename)
}

func (s *Syncer) syncCustomFiles(ctx context.Context, run *syncRun) (KindResult, error) {
	var remote []models.CustomFile
	if err := s.primary.getJSON(ctx, "/api/files", &remote); err != nil {
		return KindResult{}, err
	}
	local, err := s.deps.Store.ListCustomFiles()
	if err != nil {
		return KindResult{}, err
	}
	localImages, err := s.deps.Store.ListImages()
	if err != nil {
		return KindResult{}, err
	}
	imageByFilename := map[string]*models.Image{}
	for _, img := range localImages {
		imageByFilename[img.Filename] = img
	}
	key := func(filename string, imageFilename string, public bool) string {
		return fmt.Sprintf("%s|%s|%t", filename, imageFilename, public)
	}
	localByKey := map[string]*models.CustomFile{}
	for _, f := range local {
		name := ""
		if f.ImageID != nil {
			for _, img := range localImages {
				if img.ID == *f.ImageID {
					name = img.Filename
				}
			}
		}
		localByKey[key(f.Filename, name, f.Public)] = f
	}
	res := KindResult{Count: len(remote)}
	seen := map[string]bool{}
	for _, r := range remote {
		if _, err := safeRelative(r.Filename); err != nil {
			continue
		}
		imageFilename := ""
		var imageID *uint
		if r.ImageID != nil {
			imageFilename = run.imageIDs[*r.ImageID]
			if img, ok := imageByFilename[imageFilename]; ok {
				id := img.ID
				imageID = &id
			} else if imageFilename == "" || !r.Public {
				continue
			}
		}
		k := key(r.Filename, imageFilename, r.Public)
		seen[k] = true
		dest := s.customFilePath(r, imageFilename)
		if info, err := os.Stat(dest); err != nil || info.Size() != r.Size {
			if _, err := s.primary.download(ctx, assetPath("file", map[string]string{"id": fmt.Sprint(r.ID)}), dest); err != nil {
				return res, err
			}
			res.Changed++
		}
		if l, ok := localByKey[k]; ok {
			upd := *l
			upd.Image = nil
			upd.OriginalName, upd.Description, upd.Size, upd.ContentType = r.OriginalName, r.Description, r.Size, r.ContentType
			upd.DestinationPath, upd.AutoInstall, upd.ImageID = r.DestinationPath, r.AutoInstall, imageID
			cmp := *l
			cmp.Image = nil
			cmp.UpdatedAt = upd.UpdatedAt
			if fmt.Sprintf("%+v", cmp) != fmt.Sprintf("%+v", upd) {
				if err := s.deps.Store.UpdateCustomFile(l.ID, &upd); err != nil {
					return res, err
				}
				res.Changed++
			}
			continue
		}
		created := models.CustomFile{Filename: r.Filename, OriginalName: r.OriginalName, Description: r.Description, Size: r.Size, ContentType: r.ContentType,
			Public: r.Public, ImageID: imageID, DestinationPath: r.DestinationPath, AutoInstall: r.AutoInstall}
		if err := s.deps.Store.CreateCustomFile(&created); err != nil {
			return res, err
		}
		res.Changed++
	}
	for k, l := range localByKey {
		if seen[k] {
			continue
		}
		name := ""
		if l.ImageID != nil {
			for _, img := range localImages {
				if img.ID == *l.ImageID {
					name = img.Filename
				}
			}
		}
		os.Remove(s.customFilePath(*l, name))
		if err := s.deps.Store.DeleteCustomFile(l.ID); err != nil {
			return res, err
		}
		res.Removed++
	}
	return res, nil
}

func (s *Syncer) syncBootloaderSet(ctx context.Context, run *syncRun) (KindResult, error) {
	if run.manifest == nil {
		return KindResult{Skipped: true, Note: "manifest unavailable"}, nil
	}
	active := run.manifest.BootloaderSet
	if active == "built-in" {
		active = ""
	}
	res := KindResult{Count: 1}
	if s.deps.SelectBootloaderSet == nil {
		return res, nil
	}
	if active != "" && s.deps.BootloaderSetExists != nil && !s.deps.BootloaderSetExists(active) {
		res.Note = fmt.Sprintf("bootloader set %q is not present on this replica; keeping the built-in set", active)
		return res, nil
	}
	if err := s.deps.SelectBootloaderSet(active); err != nil {
		return res, err
	}
	return res, nil
}
