#!/bin/bash
set -euo pipefail

scripts_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
engine="${CONTAINER_ENGINE:-docker}"
secure_boot="${IPXE_SECURE_BOOT:-0}"
if [[ "$secure_boot" != 0 && "$secure_boot" != 1 ]]; then
    echo "IPXE_SECURE_BOOT must be 0 or 1" >&2
    exit 2
fi
if [[ $# -lt 2 ]]; then
    echo "Usage: $0 OUTPUT_DIRECTORY TRUST_CERTIFICATE [TRUST_CERTIFICATE ...]" >&2
    exit 2
fi
output_dir="$1"
shift
if [[ -e "$output_dir" ]]; then
    echo "Output directory already exists: $output_dir" >&2
    exit 2
fi
build_context="$(mktemp -d)"
container_id=""
image_id=""
cleanup() {
    local result="$?"
    trap - EXIT
    if [[ -n "$container_id" ]] && ! "$engine" rm "$container_id" >/dev/null; then result=1; fi
    if [[ -n "$image_id" ]] && ! "$engine" image rm --no-prune "$image_id" >/dev/null; then result=1; fi
    rm -rf "$build_context" || result=1
    exit "$result"
}
trap cleanup EXIT
mkdir "$build_context/certificates"
trust_files=()
for certificate in "$@"; do
    if grep -q -- 'PRIVATE KEY' "$certificate"; then
        echo "Pass public certificates only, never private keys: $certificate" >&2
        exit 2
    fi
    if [[ "$(grep -c -- 'BEGIN CERTIFICATE' "$certificate")" != 1 ]]; then
        echo "Pass one PEM certificate per file: $certificate" >&2
        exit 2
    fi
    target="certificates/trust-${#trust_files[@]}.pem"
    openssl x509 -in "$certificate" -out "$build_context/$target"
    trust_files+=("/build/$target")
done
(IFS=,; printf '%s\n' "${trust_files[*]}") > "$build_context/trust.list"
{
    printf '%s\n' '#!ipxe' 'imgfree autoexec.ipxe ||'
    tail -n +2 "$scripts_dir/../bootloaders/default/embed.ipxe"
} > "$build_context/embed.ipxe"
cp "$scripts_dir/ipxe-http-range.patch" "$build_context/"
"$engine" build --iidfile "$build_context/image.id" --label "org.bootimus.build=$(basename "$build_context")" --build-arg "SECURE_BOOT=$secure_boot" -f "$scripts_dir/Dockerfile.ipxe-modern" "$build_context"
image_id="$(cat "$build_context/image.id")"
container_id="$("$engine" create "$image_id")"
mkdir -p -- "$(dirname "$output_dir")"
mkdir -- "$output_dir"
"$engine" cp "$container_id:/output/." "$output_dir/"
cp "$scripts_dir/../bootloaders/default/wimboot" "$output_dir/wimboot"
(
    cd "$output_dir"
    sha256sum ./*.kpxe ./*.efi ./*.usb ./wimboot > SHA256SUMS
)
printf 'Built unsigned, opt-in loaders in %s. Existing bundled loaders are unchanged.\n' "$output_dir"
if [[ "$secure_boot" == 1 ]]; then
    printf 'UEFI binaries use restricted Secure Boot targets. Sign and enrol before use; the USB image is still unsigned.\n'
fi
