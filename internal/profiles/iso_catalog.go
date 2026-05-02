package profiles

import (
	"encoding/json"
	"fmt"
)

type ISOCatalog struct {
	Version string     `json:"version"`
	Distros []ISOEntry `json:"distros"`
}

type ISOEntry struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Mirrors  []ISOMirror  `json:"mirrors"`
	Releases []ISORelease `json:"releases"`
}

type ISOMirror struct {
	Region string `json:"region"`
	Base   string `json:"base"`
}

type ISORelease struct {
	Label    string `json:"label"`
	Path     string `json:"path"`
	SizeHint string `json:"size_hint,omitempty"`
}

func LoadISOCatalog() (*ISOCatalog, error) {
	data, err := embeddedProfiles.ReadFile("distro-profiles.json")
	if err != nil {
		return nil, fmt.Errorf("read distro-profiles: %w", err)
	}
	var pf ProfileFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse distro-profiles: %w", err)
	}
	var distros []ISOEntry
	for _, p := range pf.Profiles {
		if len(p.Releases) == 0 {
			continue
		}
		distros = append(distros, ISOEntry{
			ID:       p.ID,
			Name:     p.DisplayName,
			Mirrors:  p.Mirrors,
			Releases: p.Releases,
		})
	}
	return &ISOCatalog{Version: pf.Version, Distros: distros}, nil
}
