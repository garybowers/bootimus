#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BOOTLOADERS_DIR="$PROJECT_DIR/bootloaders"
EMBED_SCRIPT="$BOOTLOADERS_DIR/embed.ipxe"

if [ ! -f "$EMBED_SCRIPT" ]; then
    echo "Error: $EMBED_SCRIPT not found"
    exit 1
fi

echo "Building iPXE bootloaders with embedded script..."
echo "Embed script contents:"
cat "$EMBED_SCRIPT"
echo ""

docker build -t ipxe-builder -f - "$BOOTLOADERS_DIR" <<'DOCKERFILE'
FROM debian:bookworm

RUN apt-get update && apt-get install -y \
    git make gcc libc6-dev liblzma-dev mtools isolinux \
    gcc-aarch64-linux-gnu binutils-aarch64-linux-gnu \
    libc6-dev-arm64-cross ca-certificates

WORKDIR /build
RUN git clone --depth 1 https://github.com/ipxe/ipxe.git

COPY embed.ipxe /build/ipxe/src/embed.ipxe

WORKDIR /build/ipxe/src

RUN make bin/undionly.kpxe EMBED=embed.ipxe
RUN make bin-x86_64-efi/ipxe.efi EMBED=embed.ipxe
RUN make CROSS=aarch64-linux-gnu- bin-arm64-efi/ipxe.efi EMBED=embed.ipxe
DOCKERFILE

echo ""
echo "Extracting bootloaders..."

CONTAINER_ID=$(docker create ipxe-builder echo)
docker cp "$CONTAINER_ID:/build/ipxe/src/bin/undionly.kpxe" "$BOOTLOADERS_DIR/undionly.kpxe"
docker cp "$CONTAINER_ID:/build/ipxe/src/bin-x86_64-efi/ipxe.efi" "$BOOTLOADERS_DIR/ipxe.efi"
docker cp "$CONTAINER_ID:/build/ipxe/src/bin-arm64-efi/ipxe.efi" "$BOOTLOADERS_DIR/ipxe-arm64.efi"
docker rm "$CONTAINER_ID" > /dev/null

echo ""
echo "Built bootloaders:"
ls -lh "$BOOTLOADERS_DIR"/*.kpxe "$BOOTLOADERS_DIR"/*.efi
echo ""
echo "Done!"
