package admin

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	maxDriverUpload   = 500 << 20
	maxDriverExpanded = 1 << 30
	maxDriverFiles    = 10000
)

// Hash the stored file too, so packs uploaded before duplicate checking work
// without a metadata migration. Size is only a shortcut; names are irrelevant.
func driverArchiveMatches(filename string, size int64, checksum []byte) (bool, error) {
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("not a regular file")
	}
	if info.Size() != size {
		return false, nil
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, size+1))
	return err == nil && written == size && bytes.Equal(hash.Sum(nil), checksum), err
}

// A pack keeps its original directory structure. Windows treats names as
// case-insensitive, even when the server's filesystem does not.
func driverArchiveName(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = strings.TrimSuffix(name, "/")
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part {
			return "", fmt.Errorf("unsafe ZIP path %q", name)
		}
		for _, character := range part {
			if unicode.IsControl(character) || strings.ContainsRune(`<>:"|?*`, character) {
				return "", fmt.Errorf("invalid Windows filename %q", name)
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
			return "", fmt.Errorf("reserved Windows filename %q", name)
		}
	}
	return name, nil
}

// An empty destination validates the archive without extracting it. Streaming
// every file checks its CRC and enforces the expansion limit before acceptance.
func unpackDriverArchive(filename, destination string) error {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return fmt.Errorf("invalid ZIP archive: %w", err)
	}
	defer archive.Close()
	if len(archive.File) > maxDriverFiles {
		return fmt.Errorf("driver packs may contain at most %d entries", maxDriverFiles)
	}
	paths := make(map[string]bool)
	var total uint64
	infFound := false
	for _, entry := range archive.File {
		name, err := driverArchiveName(entry.Name)
		if err != nil {
			return err
		}
		if !entry.FileInfo().IsDir() && !entry.Mode().IsRegular() {
			return fmt.Errorf("links and special files are not allowed in driver packs: %s", name)
		}
		key := strings.ToLower(name)
		if _, exists := paths[key]; exists {
			return fmt.Errorf("duplicate Windows path in ZIP: %s", name)
		}
		paths[key] = entry.FileInfo().IsDir()
		if entry.UncompressedSize64 > maxDriverExpanded-total {
			return fmt.Errorf("expanded driver pack exceeds 1 GiB")
		}
		total += entry.UncompressedSize64
		if !entry.FileInfo().IsDir() && strings.EqualFold(filepath.Ext(name), ".inf") {
			infFound = true
		}
	}
	if !infFound {
		return fmt.Errorf("driver pack must contain an INF file and its supporting files")
	}
	for name := range paths {
		for parent := filepath.ToSlash(filepath.Dir(name)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if directory, exists := paths[parent]; exists && !directory {
				return fmt.Errorf("ZIP path is both a file and a directory: %s", parent)
			}
		}
	}
	for _, entry := range archive.File {
		name, _ := driverArchiveName(entry.Name)
		if entry.FileInfo().IsDir() {
			continue
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		var output io.Writer = io.Discard
		var file *os.File
		if destination != "" {
			target := filepath.Join(destination, filepath.FromSlash(name))
			if err = os.MkdirAll(filepath.Dir(target), 0755); err == nil {
				file, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			}
			if err != nil {
				input.Close()
				return err
			}
			output = file
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, int64(entry.UncompressedSize64)+1))
		input.Close()
		if file != nil {
			if closeErr := file.Close(); copyErr == nil {
				copyErr = closeErr
			}
		}
		if copyErr != nil {
			return fmt.Errorf("cannot read %s: %w", name, copyErr)
		}
		if uint64(written) != entry.UncompressedSize64 {
			return fmt.Errorf("incorrect ZIP size for %s", name)
		}
	}
	return nil
}
