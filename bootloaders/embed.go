package bootloaders

import "embed"

//go:embed *.efi *.kpxe wimboot thinos-kernel thinos-initrd.gz
var Bootloaders embed.FS
