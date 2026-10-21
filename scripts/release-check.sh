#!/bin/sh
set -eu
(cd dist && sha256sum -c SHA256SUMS)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
for archive in dist/infra-*-linux-*.tar.gz; do
    tar -xzf "$archive" -C "$work"
done
for binary in "$work"/*/bin/*; do
    file "$binary"
    if readelf -l "$binary" | grep -q INTERP; then
        echo "unexpected dynamic interpreter: $binary" >&2
        exit 1
    fi
done
