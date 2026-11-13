package bootloaders

import (
	"bytes"
	"debug/pe"
	"testing"
)

func TestOfficialSNPSet(t *testing.T) {
	const set = "secureboot-official-snp"
	if !IsBuiltIn(set) {
		t.Fatal("SNP set is not embedded")
	}
	manifest, err := LoadManifest(set)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Bootfiles.UEFI != "ipxe-shimx64.efi" || manifest.Bootfiles.ARM64 != "ipxe-arm64-shim.efi" {
		t.Fatalf("shim filenames do not match their second stages: %+v", manifest.Bootfiles)
	}
	for _, tc := range []struct {
		name    string
		machine uint16
	}{
		{manifest.Bootfiles.UEFI, pe.IMAGE_FILE_MACHINE_AMD64},
		{"ipxe.efi", pe.IMAGE_FILE_MACHINE_AMD64},
		{manifest.Bootfiles.ARM64, pe.IMAGE_FILE_MACHINE_ARM64},
		{"ipxe-arm64.efi", pe.IMAGE_FILE_MACHINE_ARM64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, resolved, err := Resolve(set, tc.name)
			if err != nil || resolved != set {
				t.Fatalf("Resolve = %q, %v", resolved, err)
			}
			file, err := pe.NewFile(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if file.Machine != tc.machine {
				t.Fatalf("machine = %x, want %x", file.Machine, tc.machine)
			}
			header, ok := file.OptionalHeader.(*pe.OptionalHeader64)
			if !ok {
				t.Fatal("missing PE32+ header")
			}
			cert := header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_SECURITY]
			if cert.Size == 0 || uint64(cert.VirtualAddress)+uint64(cert.Size) > uint64(len(data)) {
				t.Fatal("missing or truncated Authenticode certificate table")
			}
		})
	}
	for _, name := range []string{manifest.Bootfiles.BIOS, "wimboot"} {
		data, resolved, err := Resolve(set, name)
		sourceSet := "secureboot-official"
		if name == manifest.Bootfiles.BIOS {
			sourceSet = "default"
		}
		original, _, originalErr := Resolve(sourceSet, name)
		if err != nil || originalErr != nil || resolved != set || !bytes.Equal(data, original) {
			t.Fatalf("%s differs from its source set", name)
		}
	}
}
