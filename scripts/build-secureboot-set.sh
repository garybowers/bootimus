#!/bin/bash
set -euo pipefail

# Builds the "secureboot" bootloader set:
#   1. Compiles iPXE with our embed.ipxe (in Docker, same pattern as build-bootloaders.sh)
#   2. Signs the resulting EFI binaries with the Bootimus signing key (on host, key never enters Docker)
#   3. Downloads Microsoft-signed shim binaries from the ipxe/shim release
#   4. Assembles bootloaders-secureboot/ with everything plus a manifest.json
#
# Requirements on the host:
#   - docker
#   - sbsigntool (provides sbsign)
#   - curl
#
# Required environment variables:
#   BOOTIMUS_SIGNING_KEY   path to the Bootimus signing private key (.key)
#   BOOTIMUS_SIGNING_CERT  path to the matching public X.509 cert (.crt)
#
# Generate the key with: scripts/generate-signing-key.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BOOTLOADERS_DEFAULT="$ROOT_DIR/bootloaders/default"
OUT_DIR="$ROOT_DIR/bootloaders/secureboot"

# Match the iPXE pin in build-bootloaders.sh — keyboard regression in v2.0 menus.
IPXE_COMMIT="988d2c13cdf0f0b4140685af35ced70ac5b3283c"
SHIM_VERSION="${SHIM_VERSION:-16.1}"
# ipxe/shim release tags are prefixed with "ipxe-" (e.g. ipxe-16.1).
SHIM_RELEASE_BASE="https://github.com/ipxe/shim/releases/download/ipxe-${SHIM_VERSION}"

# --- preflight ---------------------------------------------------------------

KEY_PATH="${BOOTIMUS_SIGNING_KEY:-}"
CERT_PATH="${BOOTIMUS_SIGNING_CERT:-}"

if [[ -z "$KEY_PATH" || -z "$CERT_PATH" ]]; then
    echo "BOOTIMUS_SIGNING_KEY and BOOTIMUS_SIGNING_CERT must be set." >&2
    echo "Generate a fresh key+cert with: scripts/generate-signing-key.sh" >&2
    exit 1
fi

if [[ ! -f "$KEY_PATH" ]]; then
    echo "Signing key not found: $KEY_PATH" >&2
    exit 1
fi

if [[ ! -f "$CERT_PATH" ]]; then
    echo "Signing cert not found: $CERT_PATH" >&2
    exit 1
fi

for tool in docker sbsign curl; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "Required tool not found in PATH: $tool" >&2
        if [[ "$tool" == "sbsign" ]]; then
            echo "Install with: pacman -S sbsigntools  (Arch)" >&2
            echo "         or: apt install sbsigntool (Debian/Ubuntu)" >&2
        fi
        exit 1
    fi
done

# --- build iPXE in Docker ----------------------------------------------------

echo "==> Building iPXE in Docker"
docker build -t ipxe-builder -f - "$BOOTLOADERS_DEFAULT" <<DOCKERFILE
FROM debian:bookworm
RUN apt-get update && apt-get install -y git make gcc libc6-dev liblzma-dev \
    gcc-aarch64-linux-gnu binutils-aarch64-linux-gnu libc6-dev-arm64-cross ca-certificates \
    mtools
RUN git clone https://github.com/ipxe/ipxe.git /build/ipxe && \
    cd /build/ipxe && git checkout ${IPXE_COMMIT}
COPY embed.ipxe /build/ipxe/src/embed.ipxe
WORKDIR /build/ipxe/src
# Same Range header signedness fix as build-bootloaders.sh — see issue #56.
RUN sed -i 's/"bytes=%zd-%zd"/"bytes=%zu-%zu"/' net/tcp/httpcore.c && \
    grep -q '"bytes=%zu-%zu"' net/tcp/httpcore.c
RUN make NO_WERROR=1 bin-x86_64-efi/ipxe.efi EMBED=embed.ipxe
RUN make NO_WERROR=1 CROSS=aarch64-linux-gnu- bin-arm64-efi/ipxe.efi EMBED=embed.ipxe
RUN make NO_WERROR=1 bin/undionly.kpxe EMBED=embed.ipxe
DOCKERFILE

STAGING="$(mktemp -d)"
trap 'rm -rf "$STAGING"' EXIT

CID=$(docker create ipxe-builder echo)
docker cp "$CID:/build/ipxe/src/bin-x86_64-efi/ipxe.efi" "$STAGING/ipxe-x64.efi.unsigned"
docker cp "$CID:/build/ipxe/src/bin-arm64-efi/ipxe.efi"  "$STAGING/ipxe-aa64.efi.unsigned"
docker cp "$CID:/build/ipxe/src/bin/undionly.kpxe"       "$STAGING/undionly.kpxe"
docker rm "$CID" > /dev/null

# --- sign the EFI binaries on the host (key never enters Docker) -------------

echo "==> Signing iPXE binaries with Bootimus key"
sbsign --key "$KEY_PATH" --cert "$CERT_PATH" \
    --output "$STAGING/ipxe.efi" \
    "$STAGING/ipxe-x64.efi.unsigned"

sbsign --key "$KEY_PATH" --cert "$CERT_PATH" \
    --output "$STAGING/ipxe-arm64.efi" \
    "$STAGING/ipxe-aa64.efi.unsigned"

# --- download Microsoft-signed shim binaries ---------------------------------

echo "==> Downloading ipxe/shim ${SHIM_VERSION} signed binaries"
curl -fsSL -o "$STAGING/ipxe-shimx64.efi"  "$SHIM_RELEASE_BASE/ipxe-shimx64.efi"
curl -fsSL -o "$STAGING/ipxe-shimaa64.efi" "$SHIM_RELEASE_BASE/ipxe-shimaa64.efi"

# --- assemble the output set -------------------------------------------------

echo "==> Assembling $OUT_DIR"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

cp "$STAGING/ipxe-shimx64.efi"  "$OUT_DIR/"
cp "$STAGING/ipxe-shimaa64.efi" "$OUT_DIR/"
cp "$STAGING/ipxe.efi"          "$OUT_DIR/"
cp "$STAGING/ipxe-arm64.efi"    "$OUT_DIR/"
cp "$STAGING/undionly.kpxe"     "$OUT_DIR/"
cp "$CERT_PATH"                 "$OUT_DIR/bootimus-signing.crt"

cat > "$OUT_DIR/manifest.json" <<EOF
{
  "name": "secureboot",
  "description": "UEFI Secure Boot via ipxe/shim ${SHIM_VERSION}. Requires one-time MOK enrollment of bootimus-signing.crt per machine.",
  "shim_version": "${SHIM_VERSION}",
  "bootfiles": {
    "bios": "undionly.kpxe",
    "uefi": "ipxe-shimx64.efi",
    "arm64": "ipxe-shimaa64.efi"
  }
}
EOF

echo
echo "Done. Secureboot bootloader set in $OUT_DIR:"
ls -lh "$OUT_DIR"
echo
echo "To activate: copy $OUT_DIR to <BootDir>/secureboot/ and select via the bootloader-sets API/UI."
