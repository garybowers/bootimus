package extractor

import "fmt"

// The update payload distinguishes SCALE's live installer from Debian and
// FreeBSD-based TrueNAS CORE, including when the ISO has been renamed.
func isTrueNASScale(fileExists func(string) bool) bool {
	for _, path := range []string{"/vmlinuz", "/initrd.img", "/live/filesystem.squashfs", "/TrueNAS-SCALE.update"} {
		if !fileExists(path) {
			return false
		}
	}
	return true
}

func (e *Extractor) detectTrueNASUnified(reader FileSystemReader) (*BootFiles, error) {
	if !isTrueNASScale(reader.FileExists) {
		return nil, fmt.Errorf("not TrueNAS SCALE")
	}
	return &BootFiles{
		Kernel:       "/vmlinuz",
		Initrd:       "/initrd.img",
		SquashfsPath: "/live/filesystem.squashfs",
		Distro:       "truenas-scale",
	}, nil
}
