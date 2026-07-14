package bootloaders

import (
	"encoding/json"
	"path"
)

type Manifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ShimVersion string `json:"shim_version,omitempty"`
	Bootfiles   struct {
		BIOS  string `json:"bios"`
		UEFI  string `json:"uefi"`
		ARM64 string `json:"arm64"`
	} `json:"bootfiles"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func LoadManifest(setName string) (*Manifest, error) {
	if setName == "" {
		setName = DefaultSet
	}
	data, err := Bootloaders.ReadFile(path.Join(setName, "manifest.json"))
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}
