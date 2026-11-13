package models

import (
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const BootSourceNativeTFTP = "native_tftp"

// ValidateNativeTFTPBootFile requires a client-specific relative request path.
// An empty value disables accounting. The filename is case-sensitive, as in TFTP.
func ValidateNativeTFTPBootFile(filename string) error {
	if filename == "" {
		return nil
	}
	if len(filename) > 512 || !utf8.ValidString(filename) ||
		strings.ContainsAny(filename, `\`) || strings.IndexFunc(filename, unicode.IsSpace) >= 0 ||
		strings.IndexFunc(filename, unicode.IsControl) >= 0 || path.IsAbs(filename) ||
		path.Clean(filename) != filename || strings.HasPrefix(filename, "../") ||
		!strings.Contains(filename, "/") {
		return fmt.Errorf("native TFTP boot file must be a relative, client-specific path without spaces or dot segments, e.g. ec84083c/kernel7.img (maximum 512 bytes)")
	}
	return nil
}
