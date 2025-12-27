package bootloaders

import "embed"

//go:embed ipxe.efi undionly.kpxe autoexec.ipxe
var Bootloaders embed.FS
