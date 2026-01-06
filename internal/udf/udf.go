package udf

import (
	"fmt"
	"io"
)

const SectorSize = 2048

type Reader struct {
	r        io.ReaderAt
	isInited bool
	pvd      *PrimaryVolumeDescriptor
	pd       *PartitionDescriptor
	lvd      *LogicalVolumeDescriptor
	fsd      *FileSetDescriptor
	rootFE   *FileEntry
}

func NewReader(r io.ReaderAt) *Reader {
	return &Reader{
		r:        r,
		isInited: false,
	}
}

func (u *Reader) ReadSector(sectorNumber uint64) ([]byte, error) {
	buf := make([]byte, SectorSize)
	n, err := u.r.ReadAt(buf, int64(SectorSize*sectorNumber))
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read sector %d: %w", sectorNumber, err)
	}
	if n != SectorSize {
		return nil, fmt.Errorf("incomplete sector read: got %d bytes, expected %d", n, SectorSize)
	}
	return buf, nil
}

func (u *Reader) ReadSectors(sectorNumber uint64, count uint64) ([]byte, error) {
	buf := make([]byte, SectorSize*count)
	n, err := u.r.ReadAt(buf, int64(SectorSize*sectorNumber))
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read sectors starting at %d: %w", sectorNumber, err)
	}
	if n != int(SectorSize*count) {
		return nil, fmt.Errorf("incomplete sectors read: got %d bytes, expected %d", n, SectorSize*count)
	}
	return buf, nil
}

func (u *Reader) PartitionStart() uint64 {
	if u.pd == nil {
		return 0
	}
	return uint64(u.pd.PartitionStartingLocation)
}

func (u *Reader) init() error {
	if u.isInited {
		return nil
	}

	anchorData, err := u.ReadSector(256)
	if err != nil {
		return fmt.Errorf("failed to read anchor descriptor: %w", err)
	}

	anchorDesc := NewAnchorVolumeDescriptorPointer(anchorData)
	if anchorDesc.Descriptor.TagIdentifier != DescriptorAnchorVolumePointer {
		return fmt.Errorf("invalid anchor descriptor tag: 0x%x", anchorDesc.Descriptor.TagIdentifier)
	}

	for sector := uint64(anchorDesc.MainVolumeDescriptorSeq.Location); ; sector++ {
		descData, err := u.ReadSector(sector)
		if err != nil {
			return fmt.Errorf("failed to read volume descriptor at sector %d: %w", sector, err)
		}

		desc := NewDescriptor(descData)
		if desc.TagIdentifier == DescriptorTerminating {
			break
		}

		switch desc.TagIdentifier {
		case DescriptorPrimaryVolume:
			u.pvd = desc.PrimaryVolumeDescriptor()
		case DescriptorPartition:
			u.pd = desc.PartitionDescriptor()
		case DescriptorLogicalVolume:
			u.lvd = desc.LogicalVolumeDescriptor()
		}
	}

	if u.pd == nil || u.lvd == nil {
		return fmt.Errorf("missing required volume descriptors")
	}

	partitionStart := u.PartitionStart()

	fsdSector := partitionStart + u.lvd.LogicalVolumeContentsUse.Location
	fsdData, err := u.ReadSector(fsdSector)
	if err != nil {
		return fmt.Errorf("failed to read file set descriptor: %w", err)
	}
	u.fsd = NewFileSetDescriptor(fsdData)

	rootFESector := partitionStart + u.fsd.RootDirectoryICB.Location
	rootFEData, err := u.ReadSector(rootFESector)
	if err != nil {
		return fmt.Errorf("failed to read root file entry: %w", err)
	}
	u.rootFE = NewFileEntry(rootFEData)

	u.isInited = true
	return nil
}

func (u *Reader) ReadDir(fe *FileEntry) ([]*File, error) {
	if err := u.init(); err != nil {
		return nil, err
	}

	if fe == nil {
		fe = u.rootFE
	}

	if len(fe.AllocationDescriptors) == 0 {
		return nil, fmt.Errorf("no allocation descriptors in file entry")
	}

	ps := u.PartitionStart()
	adPos := fe.AllocationDescriptors[0]
	fdLen := uint64(adPos.Length)

	sectorsNeeded := (fdLen + SectorSize - 1) / SectorSize
	fdBuf, err := u.ReadSectors(ps+uint64(adPos.Location), sectorsNeeded)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory contents: %w", err)
	}

	var files []*File
	fdOff := uint64(0)

	for uint32(fdOff) < adPos.Length {
		fid := NewFileIdentifierDescriptor(fdBuf[fdOff:])
		if fid.FileIdentifier != "" {
			files = append(files, &File{
				reader: u,
				fid:    fid,
			})
		}
		fdOff += fid.Len()
	}

	return files, nil
}

func (u *Reader) Root() ([]*File, error) {
	return u.ReadDir(nil)
}
