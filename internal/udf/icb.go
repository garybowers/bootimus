package udf

type ICBTag struct {
	PriorRecordedNumberOfDirectEntries uint32
	StrategyType                       uint16
	StrategyParameter                  uint16
	MaximumNumberOfEntries             uint16
	FileType                           uint8
	ParentICBLocation                  uint64
	Flags                              uint16
}

func NewICBTag(b []byte) *ICBTag {
	itag := &ICBTag{}
	itag.PriorRecordedNumberOfDirectEntries = readU32LE(b[0:])
	itag.StrategyType = readU16LE(b[4:])
	itag.StrategyParameter = readU16LE(b[6:])
	itag.MaximumNumberOfEntries = readU16LE(b[8:])
	itag.FileType = readU8(b[11:])
	itag.ParentICBLocation = readU48LE(b[12:])
	itag.Flags = readU16LE(b[18:])
	return itag
}
