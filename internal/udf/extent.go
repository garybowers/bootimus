package udf

type Extent struct {
	Length   uint32
	Location uint32
}

func NewExtent(b []byte) Extent {
	return Extent{
		Length:   readU32LE(b[0:]),
		Location: readU32LE(b[4:]),
	}
}

type ExtentSmall struct {
	Length   uint16
	Location uint64
}

func NewExtentSmall(b []byte) ExtentSmall {
	return ExtentSmall{
		Length:   readU16LE(b[0:]),
		Location: readU48LE(b[2:]),
	}
}

type ExtentLong struct {
	Length   uint32
	Location uint64
}

func NewExtentLong(b []byte) ExtentLong {
	return ExtentLong{
		Length:   readU32LE(b[0:]),
		Location: readU48LE(b[4:]),
	}
}
