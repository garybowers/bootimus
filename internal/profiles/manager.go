package profiles

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"bootimus/internal/models"
	"bootimus/internal/storage"
)

//go:embed distro-profiles.json
var embeddedProfiles embed.FS

const RemoteProfilesURL = "https://raw.githubusercontent.com/garybowers/bootimus/main/distro-profiles.json"

type ProfileFile struct {
	Version  string        `json:"version"`
	Profiles []ProfileData `json:"profiles"`
}

type ProfileData struct {
	ID                     string       `json:"id"`
	DisplayName            string       `json:"display_name"`
	Family                 string       `json:"family"`
	FilenamePatterns       []string     `json:"filename_patterns"`
	KernelPaths            []string     `json:"kernel_paths"`
	InitrdPaths            []string     `json:"initrd_paths"`
	SquashfsPaths          []string     `json:"squashfs_paths"`
	DefaultBootParams      string       `json:"default_boot_params"`
	BootParamsWithSquashfs string       `json:"boot_params_with_squashfs,omitempty"`
	AutoInstallType        string       `json:"auto_install_type,omitempty"`
	BootMethod             string       `json:"boot_method,omitempty"`
	Mirrors                []ISOMirror  `json:"mirrors,omitempty"`
	Releases               []ISORelease `json:"releases,omitempty"`
}

type Manager struct {
	store              storage.Storage
	DisableRemoteCheck bool
}

func NewManager(store storage.Storage) *Manager {
	return &Manager{store: store}
}

func (m *Manager) SeedProfiles() error {
	data, err := embeddedProfiles.ReadFile("distro-profiles.json")
	if err != nil {
		return fmt.Errorf("failed to read embedded profiles: %w", err)
	}

	var pf ProfileFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return fmt.Errorf("failed to parse embedded profiles: %w", err)
	}

	count := 0
	for _, p := range pf.Profiles {
		existing, err := m.store.GetDistroProfile(p.ID)
		if err != nil {
			profile := profileDataToModel(p, pf.Version)
			if err := m.store.SaveDistroProfile(profile); err != nil {
				log.Printf("Profiles: Failed to seed %s: %v", p.ID, err)
			} else {
				count++
			}
		} else if !existing.Custom {
			updated := profileDataToModel(p, pf.Version)
			updated.ID = existing.ID
			updated.CreatedAt = existing.CreatedAt
			if err := m.store.SaveDistroProfile(updated); err != nil {
				log.Printf("Profiles: Failed to update %s: %v", p.ID, err)
			} else {
				count++
			}
		}
	}

	if count > 0 {
		log.Printf("Profiles: Seeded/updated %d distro profiles (version: %s)", count, pf.Version)
	} else {
		log.Printf("Profiles: %d distro profiles loaded (version: %s)", len(pf.Profiles), pf.Version)
	}

	return nil
}

func (m *Manager) UpdateFromRemote() (added int, updated int, version string, err error) {
	if m.DisableRemoteCheck {
		return 0, 0, "", fmt.Errorf("remote profile updates are disabled")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(RemoteProfilesURL)
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to fetch remote profiles: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("remote profiles returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to read response: %w", err)
	}

	var pf ProfileFile
	if err := json.Unmarshal(body, &pf); err != nil {
		return 0, 0, "", fmt.Errorf("failed to parse remote profiles: %w", err)
	}

	for _, p := range pf.Profiles {
		existing, err := m.store.GetDistroProfile(p.ID)
		if err != nil {
			profile := profileDataToModel(p, pf.Version)
			if err := m.store.SaveDistroProfile(profile); err == nil {
				added++
			}
		} else if !existing.Custom {
			updatedProfile := profileDataToModel(p, pf.Version)
			updatedProfile.ID = existing.ID
			updatedProfile.CreatedAt = existing.CreatedAt
			if err := m.store.SaveDistroProfile(updatedProfile); err == nil {
				updated++
			}
		}
	}

	log.Printf("Profiles: Remote update complete (version: %s, added: %d, updated: %d)", pf.Version, added, updated)
	return added, updated, pf.Version, nil
}

func (m *Manager) MatchProfile(filename string) (*models.DistroProfile, error) {
	allProfiles, err := m.store.ListDistroProfiles()
	if err != nil {
		return nil, err
	}
	return matchProfile(allProfiles, filename)
}

func matchProfile(profiles []*models.DistroProfile, filename string) (*models.DistroProfile, error) {
	filenameLower := strings.ToLower(filename)

	for _, p := range profiles {
		if p.Custom {
			for _, pattern := range p.FilenamePatterns {
				if strings.Contains(filenameLower, strings.ToLower(pattern)) {
					return p, nil
				}
			}
		}
	}

	for _, p := range profiles {
		if !p.Custom {
			for _, pattern := range p.FilenamePatterns {
				if strings.Contains(filenameLower, strings.ToLower(pattern)) {
					return p, nil
				}
			}
		}
	}

	for _, p := range profiles {
		if strings.Contains(filenameLower, strings.ToLower(p.ProfileID)) {
			return p, nil
		}
	}

	for _, p := range profiles {
		if p.Family != "" && strings.Contains(filenameLower, strings.ToLower(p.Family)) {
			return p, nil
		}
	}

	return nil, fmt.Errorf("no matching profile for %s", filename)
}

func (m *Manager) GetBootParams(distroID string, hasSquashfs bool) string {
	profile, err := m.store.GetDistroProfile(distroID)
	if err != nil {
		return ""
	}

	if hasSquashfs && profile.BootParamsWithSquashfs != "" {
		return profile.BootParamsWithSquashfs
	}
	return profile.DefaultBootParams
}

func profileDataToModel(p ProfileData, version string) *models.DistroProfile {
	return &models.DistroProfile{
		ProfileID:              p.ID,
		DisplayName:            p.DisplayName,
		Family:                 p.Family,
		FilenamePatterns:       models.StringSlice(p.FilenamePatterns),
		KernelPaths:            models.StringSlice(p.KernelPaths),
		InitrdPaths:            models.StringSlice(p.InitrdPaths),
		SquashfsPaths:          models.StringSlice(p.SquashfsPaths),
		DefaultBootParams:      p.DefaultBootParams,
		BootParamsWithSquashfs: p.BootParamsWithSquashfs,
		AutoInstallType:        p.AutoInstallType,
		BootMethod:             p.BootMethod,
		Custom:                 false,
		Version:                version,
	}
}
