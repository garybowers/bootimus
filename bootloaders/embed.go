package bootloaders

import "embed"

//go:embed ipxe.efi undionly.kpxe
var Bootloaders embed.FS
