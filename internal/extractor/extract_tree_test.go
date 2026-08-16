package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kdomanski/iso9660"
)

func TestExtractTree(t *testing.T) {
	files := map[string]string{
		"sysresccd/boot/x86_64/vmlinuz":       "kernel-data",
		"sysresccd/boot/x86_64/sysresccd.img": "initrd-data",
		"top.txt":                             "hello",
	}

	writer, err := iso9660.NewWriter()
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer writer.Cleanup()
	for path, content := range files {
		if err := writer.AddFile(strings.NewReader(content), path); err != nil {
			t.Fatalf("AddFile(%s): %v", path, err)
		}
	}

	isoPath := filepath.Join(t.TempDir(), "test.iso")
	isoFile, err := os.Create(isoPath)
	if err != nil {
		t.Fatalf("create iso: %v", err)
	}
	if err := writer.WriteTo(isoFile, "TEST"); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if err := isoFile.Close(); err != nil {
		t.Fatalf("close iso: %v", err)
	}

	destDir := t.TempDir()
	if err := ExtractTree(isoPath, destDir); err != nil {
		t.Fatalf("ExtractTree: %v", err)
	}

	for path, content := range files {
		data, err := os.ReadFile(filepath.Join(destDir, path))
		if err != nil {
			t.Fatalf("expected %s to be extracted: %v", path, err)
		}
		if string(data) != content {
			t.Errorf("%s content = %q, want %q", path, data, content)
		}
	}
}
