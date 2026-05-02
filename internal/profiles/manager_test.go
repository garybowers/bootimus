package profiles

import (
	"encoding/json"
	"testing"

	"bootimus/internal/models"
)

func TestMatchProfile_RealISOFilenames(t *testing.T) {
	profiles := loadEmbeddedForTest(t)

	tests := []struct {
		filename string
		wantID   string
	}{
		{"ubuntu-24.04.1-desktop-amd64.iso", "ubuntu"},
		{"kubuntu-24.04-desktop-amd64.iso", "ubuntu"},
		{"linuxmint-22-cinnamon-64bit.iso", "mint"},
		{"pop-os_22.04_amd64_intel_54.iso", "popos"},
		{"debian-12.7.0-amd64-netinst.iso", "debian"},
		{"proxmox-ve_8.2-1.iso", "debian"},
		{"archlinux-2025.04.01-x86_64.iso", "arch"},
		{"cachyos-desktop-linux-250101.iso", "arch"},
		{"manjaro-kde-24.0.0-240416-linux69.iso", "manjaro"},
		{"Fedora-Workstation-Live-x86_64-41-1.4.iso", "fedora"},
		{"Rocky-9.4-x86_64-minimal.iso", "centos"},
		{"AlmaLinux-9.4-x86_64-minimal.iso", "centos"},
		{"openSUSE-Leap-15.6-DVD-x86_64-Media.iso", "opensuse"},
		{"alpine-standard-3.20.3-x86_64.iso", "alpine"},
		{"kali-linux-2024.3-installer-amd64.iso", "kali"},
		{"systemrescue-11.00-amd64.iso", "systemrescue"},
		{"Win11_24H2_English_x64.iso", "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got, err := matchProfile(profiles, tt.filename)
			if err != nil {
				t.Fatalf("matchProfile(%q) returned error: %v", tt.filename, err)
			}
			if got.ProfileID != tt.wantID {
				t.Errorf("matchProfile(%q) = %s, want %s", tt.filename, got.ProfileID, tt.wantID)
			}
		})
	}
}

func TestMatchProfile_NoMatch(t *testing.T) {
	profiles := loadEmbeddedForTest(t)
	_, err := matchProfile(profiles, "totally-unknown-os-1.0.iso")
	if err == nil {
		t.Fatal("expected error for unknown filename, got nil")
	}
}

func TestMatchProfile_CaseInsensitive(t *testing.T) {
	profiles := loadEmbeddedForTest(t)
	got, err := matchProfile(profiles, "UBUNTU-24.04-DESKTOP.ISO")
	if err != nil {
		t.Fatalf("matchProfile error: %v", err)
	}
	if got.ProfileID != "ubuntu" {
		t.Errorf("got %s, want ubuntu", got.ProfileID)
	}
}

func TestMatchProfile_CustomBeatsBuiltin(t *testing.T) {
	profiles := []*models.DistroProfile{
		{ProfileID: "ubuntu", Custom: false, FilenamePatterns: models.StringSlice{"ubuntu"}},
		{ProfileID: "my-custom-ubuntu", Custom: true, FilenamePatterns: models.StringSlice{"ubuntu"}},
	}
	got, err := matchProfile(profiles, "ubuntu-24.04.iso")
	if err != nil {
		t.Fatalf("matchProfile error: %v", err)
	}
	if got.ProfileID != "my-custom-ubuntu" {
		t.Errorf("got %s, want my-custom-ubuntu (custom must beat built-in)", got.ProfileID)
	}
}

func TestMatchProfile_PatternBeatsIDMatch(t *testing.T) {
	profiles := []*models.DistroProfile{
		{ProfileID: "debian", Custom: false, FilenamePatterns: models.StringSlice{"debian"}},
		{ProfileID: "kali", Custom: false, FilenamePatterns: models.StringSlice{"kali"}},
	}
	got, err := matchProfile(profiles, "kali-debian-derivative-2024.iso")
	if err != nil {
		t.Fatalf("matchProfile error: %v", err)
	}
	if got.ProfileID != "debian" {
		t.Errorf("got %s, want debian (first pattern match in slice wins)", got.ProfileID)
	}
}

func TestMatchProfile_FallsBackToFamily(t *testing.T) {
	profiles := []*models.DistroProfile{
		{ProfileID: "obscure", Family: "redhat", FilenamePatterns: models.StringSlice{"obscure"}},
	}
	got, err := matchProfile(profiles, "some-redhat-derivative.iso")
	if err != nil {
		t.Fatalf("matchProfile error: %v", err)
	}
	if got.ProfileID != "obscure" {
		t.Errorf("got %s, want obscure via family match", got.ProfileID)
	}
}

func loadEmbeddedForTest(t *testing.T) []*models.DistroProfile {
	t.Helper()
	data, err := embeddedProfiles.ReadFile("distro-profiles.json")
	if err != nil {
		t.Fatalf("read embedded profiles: %v", err)
	}

	var pf ProfileFile
	if err := json.Unmarshal(data, &pf); err != nil {
		t.Fatalf("parse embedded profiles: %v", err)
	}

	out := make([]*models.DistroProfile, 0, len(pf.Profiles))
	for _, p := range pf.Profiles {
		out = append(out, profileDataToModel(p, pf.Version))
	}
	return out
}
