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
}

type ISO9660Reader struct {
	img     *iso9660.Image
	extract *Extractor
}

func (r *ISO9660Reader) FileExists(path string) bool {
	return fileExists(r.img, path)
}

func (r *ISO9660Reader) ExtractFile(isoPath, destPath string) error {
	return extractFile(r.img, isoPath, destPath)
}

func (r *ISO9660Reader) ExtractAll(destDir string) error {
	return r.extract.extractISOContents(r.img, destDir)
}

func (r *ISO9660Reader) ReadFileContent(path string) string {
	return readFileContent(r.img, path)
}

type UDFReader struct {
	reader  *udf.Reader
	extract *Extractor
}

func (r *UDFReader) FileExists(path string) bool {
	return fileExistsUDF(r.reader, path)
}

func (r *UDFReader) ExtractFile(isoPath, destPath string) error {
	return extractFileUDF(r.reader, isoPath, destPath)
}

func (r *UDFReader) ExtractAll(destDir string) error {
	return r.extract.extractUDFContents(r.reader, destDir)
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
