#!/bin/bash
set -euo pipefail

# Builds the embedded Raspberry Pi netboot asset bundle from the pftf UEFI
# releases (https://github.com/pftf) so Pi 3 and Pi 4 family boards can
# netboot straight into the standard UEFI/iPXE flow with no SD card:
#
#   Pi EEPROM/bootcode netboot -> TFTP: start*.elf + config.txt + RPI_EFI_*.fd
#     -> EDK2 UEFI -> UEFI PXE -> ipxe-arm64.efi -> Bootimus menu
#
# One shared TFTP namespace serves every board: config.txt uses [pi3]/[pi4]
# conditional filters so each model loads its own armstub. The pftf zips both
# name their firmware RPI_EFI.fd, so they are renamed per family here.
#
# The Broadcom boot blobs (bootcode.bin, start*.elf, fixup*.dat) are
# redistributable under Broadcom's licence for use with Raspberry Pi devices
# only — the licence text ships alongside in the bundle.
#
# Supported: Pi 3B / 3B+ / CM3, Pi 4B / 400 / CM4.
# Not supported: Pi 5 (no mature EDK2 port yet), Pi 2 and earlier.
#
# Requirements on the host: curl, bsdtar, sha256sum

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/raspberrypi/assets"

PFTF_VERSION="${PFTF_VERSION:-v1.52}"
RPI3_SHA256="${RPI3_SHA256:-20b894d15cdb42e055872a7ee6a850aa9373aedcfefd27f19767d975ce4f7371}"
RPI4_SHA256="${RPI4_SHA256:-3322ae85b1a74b1e67cb7bcf9e4af65299ae76e2db0af451e4ac6cd007c7a608}"

verify_sha256() {
    [ -z "$2" ] && return 0
    actual="$(sha256sum "$1" | awk '{print $1}')"
    if [ "$actual" != "$2" ]; then
        echo "Checksum mismatch for $1" >&2
        echo "  expected: $2" >&2
        echo "  actual:   $actual" >&2
        exit 1
    fi
}

for tool in curl bsdtar sha256sum; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "Required tool not found in PATH: $tool" >&2
        exit 1
    fi
done

STAGING="$(mktemp -d)"
trap 'rm -rf "$STAGING"' EXIT

for family in RPi3 RPi4; do
    echo "==> Downloading pftf/${family} ${PFTF_VERSION}"
    url="https://github.com/pftf/${family}/releases/download/${PFTF_VERSION}/${family}_UEFI_Firmware_${PFTF_VERSION}.zip"
    curl -fsSL -o "$STAGING/${family}.zip" "$url"
    case "$family" in
        RPi3) verify_sha256 "$STAGING/${family}.zip" "$RPI3_SHA256" ;;
        RPi4) verify_sha256 "$STAGING/${family}.zip" "$RPI4_SHA256" ;;
    esac
    mkdir -p "$STAGING/${family}"
    bsdtar -xf "$STAGING/${family}.zip" -C "$STAGING/${family}"
done

echo "==> Assembling $OUT_DIR"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/overlays"

cp "$STAGING/RPi3/RPI_EFI.fd"    "$OUT_DIR/RPI_EFI_3.fd"
cp "$STAGING/RPi3/bootcode.bin"  "$OUT_DIR/"
cp "$STAGING/RPi3/start.elf"     "$OUT_DIR/"
cp "$STAGING/RPi3/fixup.dat"     "$OUT_DIR/"
cp "$STAGING/RPi3"/bcm2710-*.dtb "$OUT_DIR/"

cp "$STAGING/RPi4/RPI_EFI.fd"    "$OUT_DIR/RPI_EFI_4.fd"
cp "$STAGING/RPi4/start4.elf"    "$OUT_DIR/"
cp "$STAGING/RPi4/fixup4.dat"    "$OUT_DIR/"
cp "$STAGING/RPi4"/bcm2711-*.dtb "$OUT_DIR/"
cp "$STAGING/RPi4/overlays/miniuart-bt.dtbo"   "$OUT_DIR/overlays/"
cp "$STAGING/RPi4/overlays/upstream-pi4.dtbo"  "$OUT_DIR/overlays/"
cp "$STAGING/RPi4/firmware/LICENCE.txt" "$OUT_DIR/LICENCE.broadcom.txt"

# Unified config.txt: merged from the per-family configs the pftf releases
# ship, with model filters selecting the matching armstub. The device tree
# windows intentionally differ per family — they mirror upstream pftf.
cat > "$OUT_DIR/config.txt" <<'EOF'
enable_uart=1
uart_2ndstage=1
disable_overscan=1

[pi3]
arm_64bit=1
disable_commandline_tags=2
armstub=RPI_EFI_3.fd
device_tree_address=0x1f0000
device_tree_end=0x200000

[pi4]
arm_64bit=1
arm_boost=1
enable_gic=1
disable_commandline_tags=1
armstub=RPI_EFI_4.fd
device_tree_address=0x3e0000
device_tree_end=0x400000
dtoverlay=miniuart-bt
dtoverlay=upstream-pi4

[all]
EOF

echo
echo "Done. Raspberry Pi netboot assets in $OUT_DIR:"
ls -lhR "$OUT_DIR"
echo
echo "These assets are embedded into the bootimus binary via go:embed — commit"
echo "the refreshed $OUT_DIR and rebuild. Per-machine overrides go in"
echo "<data_dir>/raspberrypi/<serial>/ on the server."
