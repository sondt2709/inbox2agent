#!/usr/bin/env bash
# Build the release archives into dist/.
# Usage: scripts/dist.sh <version>   (for example 1.2.0, without the "v")
# Each archive holds i2a, LICENSE and README.md. The archive names have no version,
# so that releases/latest/download/<name> is always the latest release.
set -euo pipefail

version="${1:?usage: scripts/dist.sh <version>}"
cd "$(git rev-parse --show-toplevel)"

platforms="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"
rm -rf dist
mkdir -p dist

for p in $platforms; do
  goos="${p%/*}"
  goarch="${p#*/}"
  name="i2a_${goos}_${goarch}"
  work="dist/$name"
  mkdir -p "$work"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/i2a" ./cmd/i2a
  cp LICENSE README.md "$work/"
  tar -czf "dist/$name.tar.gz" -C "$work" i2a LICENSE README.md
  rm -rf "$work"
  echo "dist: $name.tar.gz"
done

cd dist
if command -v sha256sum >/dev/null; then
  sha256sum ./*.tar.gz | sed 's| \./| |' > checksums.txt
else
  shasum -a 256 ./*.tar.gz | sed 's| \./| |' > checksums.txt
fi
cat checksums.txt
