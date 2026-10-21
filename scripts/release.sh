#!/bin/sh
set -eu
GO=${GO:-go}
version=0.1.0
mkdir -p dist
"$GO" mod download
for platform in linux/amd64 linux/arm64 darwin/arm64; do
    os=${platform%/*}
    arch=${platform#*/}
    name="infra-${version}-${os}-${arch}"
    staging=$(mktemp -d)
    mkdir -p "$staging/$name/bin"
    commands="infractl"
    if [ "$os" = linux ]; then commands="infra-server infra-agent infra-ui infractl"; fi
    for command in $commands; do
        GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 "$GO" build -trimpath -ldflags='-s -w' -o "$staging/$name/bin/$command" "./cmd/$command"
    done
    cp -R packaging docs skills api "$staging/$name/"
    cp README.md go.mod go.sum "$staging/$name/"
    "$GO" list -m all > "$staging/$name/dependencies.txt"
    GO="$GO" python3 scripts/dependency-notices.py "$staging/$name"
    tar -czf "dist/$name.tar.gz" -C "$staging" "$name"
    rm -rf "$staging"
done
(cd dist && sha256sum infra-*.tar.gz > SHA256SUMS)
