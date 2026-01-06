package udf

import (
	"time"
)

const (
	DescriptorPrimaryVolume           = 0x1
	DescriptorAnchorVolumePointer     = 0x2
	DescriptorVolumePointer           = 0x3
	DescriptorImplementationUseVolume = 0x4
	DescriptorPartition               = 0x5
	DescriptorLogicalVolume           = 0x6
	DescriptorUnallocated             = 0x7
	DescriptorTerminating             = 0x8
	DescriptorFileSet                 = 0x100
	DescriptorIdentifier              = 0x101
	DescriptorAllocationExtent        = 0x102
	DescriptorIndirectEntry           = 0x103
	DescriptorTerminalEntry           = 0x104
	DescriptorFileEntry               = 0x105
)

type Descriptor struct {
	TagIdentifier       uint16
	DescriptorVersion   uint16
	TagChecksum         uint8
	TagSerialNumber     uint16
	DescriptorCRC       uint16
	DescriptorCRCLength uint16
	TagLocation         uint32
	data                []byte
}

func NewDescriptor(b []byte) *Descriptor {
	d := &Descriptor{
		data: b,
	}
	d.TagIdentifier = readU16LE(b[0:])
	d.DescriptorVersion = readU16LE(b[2:])
	d.TagChecksum = readU8(b[4:])
	d.TagSerialNumber = readU16LE(b[6:])
	d.DescriptorCRC = readU16LE(b[8:])
	d.DescriptorCRCLength = readU16LE(b[10:])
	d.TagLocation = readU32LE(b[12:])
	return d
}

func (d *Descriptor) PrimaryVolumeDescriptor() *PrimaryVolumeDescriptor {
	return NewPrimaryVolumeDescriptor(d.data)
}

func (d *Descriptor) PartitionDescriptor() *PartitionDescriptor {
	return NewPartitionDescriptor(d.data)
}

func (d *Descriptor) LogicalVolumeDescriptor() *LogicalVolumeDescriptor {
	return NewLogicalVolumeDescriptor(d.data)
}

type AnchorVolumeDescriptorPointer struct {
	Descriptor                 Descriptor
	MainVolumeDescriptorSeq    Extent
	ReserveVolumeDescriptorSeq Extent
}

func NewAnchorVolumeDescriptorPointer(b []byte) *AnchorVolumeDescriptorPointer {
	ad := &AnchorVolumeDescriptorPointer{}
	ad.Descriptor = *NewDescriptor(b)
	ad.MainVolumeDescriptorSeq = NewExtent(b[16:])
	ad.ReserveVolumeDescriptorSeq = NewExtent(b[24:])
	return ad
}

type PrimaryVolumeDescriptor struct {
	Descriptor                                  Descriptor
	VolumeDescriptorSequenceNumber              uint32
	PrimaryVolumeDescriptorNumber               uint32
	VolumeIdentifier                            string
	VolumeSequenceNumber                        uint16
	MaximumVolumeSequenceNumber                 uint16
	InterchangeLevel                            uint16
	MaximumInterchangeLevel                     uint16
	CharacterSetList                            uint32
	MaximumCharacterSetList                     uint32
	VolumeSetIdentifier                         string
	VolumeAbstract                              Extent
	VolumeCopyrightNoticeExtent                 Extent
	ApplicationIdentifier                       EntityID
	RecordingDateTime                           time.Time
	ImplementationIdentifier                    EntityID
	ImplementationUse                           []byte
	PredecessorVolumeDescriptorSequenceLocation uint32
	Flags                                       uint16
}

func NewPrimaryVolumeDescriptor(b []byte) *PrimaryVolumeDescriptor {
	pvd := &PrimaryVolumeDescriptor{}
	pvd.Descriptor = *NewDescriptor(b)
	pvd.VolumeDescriptorSequenceNumber = readU32LE(b[16:])
	pvd.PrimaryVolumeDescriptorNumber = readU32LE(b[20:])
	pvd.VolumeIdentifier = readDString(b[24:], 32)
	pvd.VolumeSequenceNumber = readU16LE(b[56:])
	pvd.MaximumVolumeSequenceNumber = readU16LE(b[58:])
	pvd.InterchangeLevel = readU16LE(b[60:])
	pvd.MaximumInterchangeLevel = readU16LE(b[62:])
	pvd.CharacterSetList = readU32LE(b[64:])
	pvd.MaximumCharacterSetList = readU32LE(b[68:])
	pvd.VolumeSetIdentifier = readDString(b[72:], 128)
	pvd.VolumeAbstract = NewExtent(b[328:])
	pvd.VolumeCopyrightNoticeExtent = NewExtent(b[336:])
	pvd.ApplicationIdentifier = NewEntityID(b[344:])
	pvd.RecordingDateTime = readTimestamp(b[376:])
	pvd.ImplementationIdentifier = NewEntityID(b[388:])
	pvd.ImplementationUse = b[420:484]
	pvd.PredecessorVolumeDescriptorSequenceLocation = readU32LE(b[484:])
	pvd.Flags = readU16LE(b[488:])
	return pvd
}

type PartitionDescriptor struct {
	Descriptor                     Descriptor
	VolumeDescriptorSequenceNumber uint32
	PartitionFlags                 uint16
	PartitionNumber                uint16
	PartitionContents              EntityID
	PartitionContentsUse           []byte
	AccessType                     uint32
	PartitionStartingLocation      uint32
	PartitionLength                uint32
	ImplementationIdentifier       EntityID
	ImplementationUse              []byte
}

func NewPartitionDescriptor(b []byte) *PartitionDescriptor {
	pd := &PartitionDescriptor{}
	pd.Descriptor = *NewDescriptor(b)
	pd.VolumeDescriptorSequenceNumber = readU32LE(b[16:])
	pd.PartitionFlags = readU16LE(b[20:])
	pd.PartitionNumber = readU16LE(b[22:])
	pd.PartitionContents = NewEntityID(b[24:])
	pd.PartitionContentsUse = b[56:184]
	pd.AccessType = readU32LE(b[184:])
	pd.PartitionStartingLocation = readU32LE(b[188:])
	pd.PartitionLength = readU32LE(b[192:])
	pd.ImplementationIdentifier = NewEntityID(b[196:])
	pd.ImplementationUse = b[228:356]
	return pd
}

type LogicalVolumeDescriptor struct {
	Descriptor                     Descriptor
	VolumeDescriptorSequenceNumber uint32
	LogicalVolumeIdentifier        string
	LogicalBlockSize               uint32
	DomainIdentifier               EntityID
	LogicalVolumeContentsUse       ExtentLong
	MapTableLength                 uint32
	NumberOfPartitionMaps          uint32
	ImplementationIdentifier       EntityID
	ImplementationUse              []byte
	IntegritySequenceExtent        Extent
}

func NewLogicalVolumeDescriptor(b []byte) *LogicalVolumeDescriptor {
	lvd := &LogicalVolumeDescriptor{}
	lvd.Descriptor = *NewDescriptor(b)
	lvd.VolumeDescriptorSequenceNumber = readU32LE(b[16:])
	lvd.LogicalVolumeIdentifier = readDString(b[84:], 128)
	lvd.LogicalBlockSize = readU32LE(b[212:])
	lvd.DomainIdentifier = NewEntityID(b[216:])
	lvd.LogicalVolumeContentsUse = NewExtentLong(b[248:])
	lvd.MapTableLength = readU32LE(b[264:])
	lvd.NumberOfPartitionMaps = readU32LE(b[268:])
	lvd.ImplementationIdentifier = NewEntityID(b[272:])
	lvd.ImplementationUse = b[304:432]
	lvd.IntegritySequenceExtent = NewExtent(b[432:])
	return lvd
}

type FileSetDescriptor struct {
	Descriptor              Descriptor
	RecordingDateTime       time.Time
	InterchangeLevel        uint16
	MaximumInterchangeLevel uint16
	CharacterSetList        uint32
	MaximumCharacterSetList uint32
	FileSetNumber           uint32
	FileSetDescriptorNumber uint32
	LogicalVolumeIdentifier string
	FileSetIdentifier       string
	CopyrightFileIdentifier string
	AbstractFileIdentifier  string
	RootDirectoryICB        ExtentLong
	DomainIdentifier        EntityID
	NextExtent              ExtentLong
}

func NewFileSetDescriptor(b []byte) *FileSetDescriptor {
	fsd := &FileSetDescriptor{}
	fsd.Descriptor = *NewDescriptor(b)
	fsd.RecordingDateTime = readTimestamp(b[16:])
	fsd.InterchangeLevel = readU16LE(b[28:])
	fsd.MaximumInterchangeLevel = readU16LE(b[30:])
	fsd.CharacterSetList = readU32LE(b[32:])
	fsd.MaximumCharacterSetList = readU32LE(b[36:])
	fsd.FileSetNumber = readU32LE(b[40:])
	fsd.FileSetDescriptorNumber = readU32LE(b[44:])
	fsd.LogicalVolumeIdentifier = readDString(b[112:], 128)
	fsd.FileSetIdentifier = readDString(b[304:], 32)
	fsd.CopyrightFileIdentifier = readDString(b[336:], 32)
	fsd.AbstractFileIdentifier = readDString(b[368:], 32)
	fsd.RootDirectoryICB = NewExtentLong(b[400:])
	fsd.DomainIdentifier = NewEntityID(b[416:])
	fsd.NextExtent = NewExtentLong(b[448:])
	return fsd
}

type FileIdentifierDescriptor struct {
	Descriptor                Descriptor
	FileVersionNumber         uint16
	FileCharacteristics       uint8
	LengthOfFileIdentifier    uint8
	ICB                       ExtentLong
	LengthOfImplementationUse uint16
	ImplementationUse         EntityID
	FileIdentifier            string
}

func (fid *FileIdentifierDescriptor) Len() uint64 {
	l := 38 + uint64(fid.LengthOfImplementationUse) + uint64(fid.LengthOfFileIdentifier)
	return 4 * ((l + 3) / 4)
}

func NewFileIdentifierDescriptor(b []byte) *FileIdentifierDescriptor {
	fid := &FileIdentifierDescriptor{}
	fid.Descriptor = *NewDescriptor(b)
	fid.FileVersionNumber = readU16LE(b[16:])
	fid.FileCharacteristics = readU8(b[18:])
	fid.LengthOfFileIdentifier = readU8(b[19:])
	fid.ICB = NewExtentLong(b[20:])
	fid.LengthOfImplementationUse = readU16LE(b[36:])
	fid.ImplementationUse = NewEntityID(b[38:])
	identStart := 38 + fid.LengthOfImplementationUse
	fid.FileIdentifier = readDCharacters(b[identStart : fid.LengthOfFileIdentifier+uint8(identStart)])
	return fid
}

type FileEntry struct {
	Descriptor                    Descriptor
	ICBTag                        *ICBTag
	Uid                           uint32
	Gid                           uint32
	Permissions                   uint32
	FileLinkCount                 uint16
	RecordFormat                  uint8
	RecordDisplayAttributes       uint8
	RecordLength                  uint32
	InformationLength             uint64
	LogicalBlocksRecorded         uint64
	AccessTime                    time.Time
	ModificationTime              time.Time
	AttributeTime                 time.Time
	Checkpoint                    uint32
	ExtendedAttributeICB          ExtentLong
	ImplementationIdentifier      EntityID
	UniqueID                      uint64
	LengthOfExtendedAttributes    uint32
	LengthOfAllocationDescriptors uint32
	ExtendedAttributes            []byte
	AllocationDescriptors         []Extent
}

func NewFileEntry(b []byte) *FileEntry {
	fe := &FileEntry{}
	fe.Descriptor = *NewDescriptor(b)
	fe.ICBTag = NewICBTag(b[16:])
	fe.Uid = readU32LE(b[36:])
	fe.Gid = readU32LE(b[40:])
	fe.Permissions = readU32LE(b[44:])
	fe.FileLinkCount = readU16LE(b[48:])
	fe.RecordFormat = readU8(b[50:])
	fe.RecordDisplayAttributes = readU8(b[51:])
	fe.RecordLength = readU32LE(b[52:])
	fe.InformationLength = readU64LE(b[56:])
	fe.LogicalBlocksRecorded = readU64LE(b[64:])
	fe.AccessTime = readTimestamp(b[72:])
	fe.ModificationTime = readTimestamp(b[84:])
	fe.AttributeTime = readTimestamp(b[96:])
	fe.Checkpoint = readU32LE(b[108:])
	fe.ExtendedAttributeICB = NewExtentLong(b[112:])
	fe.ImplementationIdentifier = NewEntityID(b[128:])
	fe.UniqueID = readU64LE(b[160:])
	fe.LengthOfExtendedAttributes = readU32LE(b[168:])
	fe.LengthOfAllocationDescriptors = readU32LE(b[172:])
	allocDescStart := 176 + fe.LengthOfExtendedAttributes
	fe.ExtendedAttributes = b[176:allocDescStart]

	numDescriptors := fe.LengthOfAllocationDescriptors / 8
	fe.AllocationDescriptors = make([]Extent, numDescriptors)
	for i := range fe.AllocationDescriptors {
		offset := allocDescStart + uint32(i)*8
		fe.AllocationDescriptors[i] = NewExtent(b[offset:])
	}

	return fe
}
