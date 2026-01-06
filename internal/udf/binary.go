package udf

import (
	"encoding/binary"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)


func readU8(b []byte) uint8 {
	return b[0]
}

func readI8(b []byte) int8 {
	return int8(readU8(b))
}

var readU64LE = binary.LittleEndian.Uint64

func readU48LE(b []byte) uint64 {
	var buf [8]byte
	copy(buf[:], b[:6])
	return readU64LE(buf[:])
}

var readU32LE = binary.LittleEndian.Uint32
var readU16LE = binary.LittleEndian.Uint16

func readI64LE(b []byte) int64 {
	return int64(readU64LE(b))
}

func readI32LE(b []byte) int32 {
	return int32(readU32LE(b))
}

func readI16LE(b []byte) int16 {
	return int16(readU16LE(b))
}

var readU64BE = binary.BigEndian.Uint64
var readU32BE = binary.BigEndian.Uint32
var readU16BE = binary.BigEndian.Uint16

func readU8BE(b []byte) uint8 {
	return b[0]
}

func readI64BE(b []byte) int64 {
	return int64(readU64BE(b))
}

func readI32BE(b []byte) int32 {
	return int32(readU32BE(b))
}

func readI16BE(b []byte) int16 {
	return int16(readU16BE(b))
}

func readDString(b []byte, fieldlen int) string {
	if fieldlen == 0 {
		return ""
	}
	strlen := int(b[fieldlen-1])
	if strlen > fieldlen-1 {
		strlen = fieldlen - 1
	}
	return string(b[:strlen])
}

func readDCharacters(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	switch b[0] {
	case 8:
		s, _, err := transform.Bytes(charmap.Windows1252.NewDecoder(), b[1:])
		if err != nil {
			return ""
		}
		return string(s)
	case 16:
		s, _, err := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder(), b[1:])
		if err != nil {
			return ""
		}
		return string(s)
	default:
		return ""
	}
}

func readTimestamp(b []byte) time.Time {
	year := int(readU16LE(b[2:]))
	month := int(b[4])
	day := int(b[5])
	hour := int(b[6])
	minute := int(b[7])
	second := int(b[8])

	if year < 1970 || year > 3000 {
		return time.Time{}
	}
	if month < 1 || month > 12 {
		return time.Time{}
	}
	if day < 1 || day > 31 {
		return time.Time{}
	}

	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	return t
}
