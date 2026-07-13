#!/bin/sh
set -e

# Seed the zero-enrollment Secure Boot bootloader set (baked into the image at
# build time) into the data volume as an on-disk set, unless one already
# exists. Delete <data>/bootloaders/secureboot-official to re-seed from a
# newer image.
DATA_DIR="${BOOTIMUS_DATA_DIR:-/data}"
SEED_SRC="/usr/share/bootimus/secureboot-official"
SEED_DST="$DATA_DIR/bootloaders/secureboot-official"

if [ -d "$SEED_SRC" ] && [ ! -d "$SEED_DST" ]; then
    mkdir -p "$DATA_DIR/bootloaders"
    cp -r "$SEED_SRC" "$SEED_DST"
    echo "Seeded Secure Boot bootloader set into $SEED_DST"
fi

exec /bootimus "$@"
