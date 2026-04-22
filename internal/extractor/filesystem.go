package extractor

import (
	"io"

	"bootimus/internal/udf"

	"github.com/kdomanski/iso9660"
)

type FileSystemReader interface {
	FileExists(path string) bool
	ExtractFile(isoPath, destPath string) error
	ExtractAll(destDir string) error
	ReadFileContent(path string) string
	ListDirectory(path string) ([]DirEntry, error)
}

type DirEntry struct {
	Name  string
	IsDir bool
}

type ISO9660Reader struct {
	img     *iso9660.Image
	extract *Extractor
}

func (r *ISO9660Reader) FileExists(path string) bool {
	return fileExists(r.img, path)
}

func (r *ISO9660Reader) ExtractFile(isoPath, destPath string) error {
	return r.extract.extractNamedFile(r.img, isoPath, destPath)
}

func (r *ISO9660Reader) ExtractAll(destDir string) error {
	return r.extract.extractISOContents(r.img, destDir)
}

func (r *ISO9660Reader) ReadFileContent(path string) string {
	return readFileContent(r.img, path)
}

func (r *ISO9660Reader) ListDirectory(path string) ([]DirEntry, error) {
	file, err := findFile(r.img, path)
	if err != nil {
		return nil, err
	}
	if !file.IsDir() {
		return nil, nil
	}
	children, err := safeGetChildren(file)
	if err != nil {
		return nil, err
	}
	var entries []DirEntry
	for _, c := range children {
		entries = append(entries, DirEntry{Name: c.Name(), IsDir: c.IsDir()})
	}
	return entries, nil
}

type UDFReader struct {
	reader  *udf.Reader
	extract *Extractor
}

func (r *UDFReader) FileExists(path string) bool {
	return fileExistsUDF(r.reader, path)
}

func (r *UDFReader) ExtractFile(isoPath, destPath string) error {
	return r.extract.extractNamedFileUDF(r.reader, isoPath, destPath)
}

func (r *UDFReader) ExtractAll(destDir string) error {
	return r.extract.extractUDFContents(r.reader, destDir)
}

func (r *UDFReader) ListDirectory(path string) ([]DirEntry, error) {
	file, err := findFileUDF(r.reader, path)
	if err != nil {
		return nil, err
	}
	if !file.IsDir() {
		return nil, nil
	}
	children, err := file.ReadDir()
	if err != nil {
		return nil, err
	}
	var entries []DirEntry
	for _, c := range children {
		entries = append(entries, DirEntry{Name: c.Name(), IsDir: c.IsDir()})
	}
	return entries, nil
}

func (r *UDFReader) ReadFileContent(path string) string {
	file, err := findFileUDF(r.reader, path)
	if err != nil {
		return ""
	}

	if file.IsDir() {
		return ""
	}

	reader, err := file.Open()
	if err != nil {
		return ""
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return ""
	}

	return string(content)
}
