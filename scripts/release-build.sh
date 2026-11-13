#!/usr/bin/env bash
# Cross-compiles dossier for all release targets and packages them into dist/.
# Usage: scripts/release-build.sh <version>   (e.g. v0.1.0)
set -euo pipefail

version="${1:?usage: $0 <version>}"
targets=(
  linux/amd64 linux/arm64
  darwin/amd64 darwin/arm64
  windows/amd64 windows/arm64
)

root="$(cd "$(dirname "$0")/.." && pwd)"
dist="$root/dist"
rm -rf "$dist"
mkdir -p "$dist"

for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  name="dossier_${version}_${os}_${arch}"
  bin="dossier"
  [[ "$os" == windows ]] && bin="dossier.exe"

  stage="$dist/$name"
  mkdir -p "$stage"
  echo "building $os/$arch"
  (cd "$root" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X dossier/internal/cli.version=$version" \
    -o "$stage/$bin" ./cmd/dossier)
  cp "$root/README.md" "$root/LICENSE" "$stage/"

  if [[ "$os" == windows ]]; then
    (cd "$dist" && zip -qr "$name.zip" "$name")
  else
    tar -C "$dist" -czf "$dist/$name.tar.gz" "$name"
  fi
  rm -rf "$stage"
done

cd "$dist"
if command -v sha256sum >/dev/null; then
  sha256sum -- *.tar.gz *.zip > checksums.txt
else
  shasum -a 256 -- *.tar.gz *.zip > checksums.txt
fi
ls -1 "$dist"
