#!/bin/bash
set -euo pipefail

# Generates a Bootimus Secure Boot signing key + self-signed X.509 cert.
# Run this ONCE per project. The private key must be kept secret — do not
# commit it to the repo. Store it in 1Password / encrypted backup / HSM.

OUT_DIR="${1:-./signing}"
DAYS="${SIGNING_VALID_DAYS:-3650}"
SUBJECT="/CN=Bootimus Secure Boot Signing/O=Bootimus"

if [[ -e "$OUT_DIR/bootimus-signing.key" ]]; then
    echo "Refusing to overwrite existing key at $OUT_DIR/bootimus-signing.key" >&2
    echo "If you really mean to rotate, move the old key out of the way first." >&2
    exit 1
fi

mkdir -p "$OUT_DIR"

openssl req -new -x509 -newkey rsa:2048 -nodes -days "$DAYS" \
    -keyout "$OUT_DIR/bootimus-signing.key" \
    -out "$OUT_DIR/bootimus-signing.crt" \
    -subj "$SUBJECT"

chmod 600 "$OUT_DIR/bootimus-signing.key"

cat <<EOF

Generated:
  $OUT_DIR/bootimus-signing.key   (PRIVATE — keep secret, never commit)
  $OUT_DIR/bootimus-signing.crt   (public — distribute alongside signed binaries)

Next steps:
  1. Move bootimus-signing.key to durable secret storage (1Password, HSM, etc.)
  2. Set environment variables before running build-secureboot-set.sh:
       export BOOTIMUS_SIGNING_KEY=/path/to/bootimus-signing.key
       export BOOTIMUS_SIGNING_CERT=/path/to/bootimus-signing.crt
  3. The .crt file should be checked into a public location so users can verify
     and MOK-enroll it.
EOF
