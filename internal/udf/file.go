package udf

import (
	"fmt"
	"io"
	"os"
	"time"
)

type File struct {
	reader            *Reader
	fid               *FileIdentifierDescriptor
	fe                *FileEntry
	fileEntryPosition uint64
}

func (f *File) Name() string {
	return f.fid.FileIdentifier
}

func (f *File) Size() int64 {
	return int64(f.FileEntry().InformationLength)
}

func (f *File) ModTime() time.Time {
	return f.FileEntry().ModificationTime
}

func (f *File) IsDir() bool {
	return f.FileEntry().ICBTag.FileType == 4
}

func (f *File) Mode() os.FileMode {
	var mode os.FileMode

	perms := os.FileMode(f.FileEntry().Permissions)
	mode |= ((perms >> 0) & 7) << 0
	mode |= ((perms >> 5) & 7) << 3
	mode |= ((perms >> 10) & 7) << 6

	if f.IsDir() {
		mode |= os.ModeDir
	}

	return mode
}

func (f *File) FileEntry() *FileEntry {
	if f.fe == nil {
		f.fileEntryPosition = f.fid.ICB.Location
		feData, err := f.reader.ReadSector(f.reader.PartitionStart() + f.fileEntryPosition)
		if err != nil {
			return &FileEntry{}
		}
		f.fe = NewFileEntry(feData)
	}
	return f.fe
}

func (f *File) GetFileOffset() int64 {
	fe := f.FileEntry()
	if len(fe.AllocationDescriptors) == 0 {
		return 0
	}
	return SectorSize * (int64(fe.AllocationDescriptors[0].Location) + int64(f.reader.PartitionStart()))
}

func (f *File) NewReader() *io.SectionReader {
	return io.NewSectionReader(f.reader.r, f.GetFileOffset(), f.Size())
}

func (f *File) ReadDir() ([]*File, error) {
	if !f.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", f.Name())
	}
	return f.reader.ReadDir(f.FileEntry())
}

func (f *File) Open() (io.Reader, error) {
	if f.IsDir() {
		return nil, fmt.Errorf("cannot open directory: %s", f.Name())
	}
	return f.NewReader(), nil
}

func (f *File) Sys() interface{} {
	return f.fid
}
