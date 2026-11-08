package admin

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type driverZipEntry struct {
	name, content string
	mode          os.FileMode
}

func driverZIP(t *testing.T, entries ...driverZipEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestDriverArchiveValidation(t *testing.T) {
	good := driverZipEntry{"NetKVM/netkvm.inf", "[Version]\nSignature=\"$Windows NT$\"", 0}
	cases := []struct {
		name    string
		entries []driverZipEntry
		want    string
	}{
		{"parent", []driverZipEntry{good, {"../outside.sys", "bad", 0}}, "unsafe"},
		{"windows-parent", []driverZipEntry{good, {`..\outside.sys`, "bad", 0}}, "unsafe"},
		{"absolute", []driverZipEntry{good, {"/outside.sys", "bad", 0}}, "unsafe"},
		{"drive", []driverZipEntry{good, {"C:/outside.sys", "bad", 0}}, "invalid"},
		{"stream", []driverZipEntry{good, {"driver.sys:hidden", "bad", 0}}, "invalid"},
		{"device", []driverZipEntry{good, {"NUL.sys", "bad", 0}}, "reserved"},
		{"trailing-dot", []driverZipEntry{good, {"driver.sys.", "bad", 0}}, "unsafe"},
		{"duplicate-case", []driverZipEntry{good, {"NETKVM/NETKVM.INF", "other", 0}}, "duplicate"},
		{"file-directory", []driverZipEntry{good, {"netkvm", "file", 0}}, "both a file"},
		{"link", []driverZipEntry{good, {"link", "../outside", os.ModeSymlink | 0777}}, "links"},
		{"no-inf", []driverZipEntry{{"readme.txt", "no drivers", 0}}, "INF"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "pack.zip")
			if err := os.WriteFile(filename, driverZIP(t, test.entries...), 0600); err != nil {
				t.Fatal(err)
			}
			if err := unpackDriverArchive(filename, ""); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
	t.Run("nested-windows-paths", func(t *testing.T) {
		root := t.TempDir()
		filename := filepath.Join(root, "pack.zip")
		content := driverZIP(t, good, driverZipEntry{`NetKVM\amd64\driver.sys`, "payload", 0})
		if err := os.WriteFile(filename, content, 0600); err != nil {
			t.Fatal(err)
		}
		if err := unpackDriverArchive(filename, filepath.Join(root, "extracted")); err != nil {
			t.Fatal(err)
		}
		result, err := os.ReadFile(filepath.Join(root, "extracted/NetKVM/amd64/driver.sys"))
		if err != nil || string(result) != "payload" {
			t.Fatal("supporting file was not preserved", err)
		}
	})
	t.Run("corrupt-content", func(t *testing.T) {
		content := driverZIP(t, good)
		content[bytes.Index(content, []byte("[Version]"))] ^= 1
		filename := filepath.Join(t.TempDir(), "pack.zip")
		os.WriteFile(filename, content, 0600)
		if err := unpackDriverArchive(filename, ""); err == nil {
			t.Fatal("corrupt driver accepted")
		}
	})
	t.Run("expansion-limit", func(t *testing.T) {
		content := driverZIP(t, good)
		central := bytes.Index(content, []byte("PK\x01\x02"))
		// A declared size larger than the extraction budget must be rejected
		// before decompression or creation of any destination file.
		copy(content[central+24:central+28], []byte{0xff, 0xff, 0xff, 0x7f})
		filename := filepath.Join(t.TempDir(), "pack.zip")
		os.WriteFile(filename, content, 0600)
		if err := unpackDriverArchive(filename, ""); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatal(err)
		}
	})
}
