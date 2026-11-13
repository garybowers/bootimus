#!/usr/bin/env bash
# Shared version policy for GitHub releases and local Docker publishing.
set -euo pipefail
export LC_ALL=C

if [[ $# != 1 ]]; then
    echo 'Usage: release-metadata.sh VERSION (without the v prefix)' >&2
    exit 1
fi
version=$1
pattern='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if [[ ! $version =~ $pattern ]] || (( ${#version} > 128 )); then
    echo 'Use X.Y.Z or X.Y.Z-prerelease (maximum 128 characters; Docker tags do not support +build metadata).' >&2
    exit 1
fi

prerelease=false
latest=true
if [[ $version == *-* ]]; then
    IFS='.' read -r -a identifiers <<< "${version#*-}"
    for identifier in "${identifiers[@]}"; do
        if [[ $identifier =~ ^[0-9]+$ && $identifier == 0?* ]]; then
            echo 'Numeric prerelease identifiers must not have leading zeroes.' >&2
            exit 1
        fi
    done
    prerelease=true
    latest=false
fi

printf 'version=%s\nprerelease=%s\nlatest=%s\n' "$version" "$prerelease" "$latest"
