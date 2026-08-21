#!/usr/bin/env bash
set -euo pipefail

# The caller must first verify the complete release archive set.
dist=$1
version=$2
destination=$3
for arch in amd64 arm64; do
  install -d "$destination/out/$arch"
  tar -xOf "$dist/actionlint_${version}_linux_${arch}.tar.gz" actionlint > "$destination/out/$arch/actionlint"
  chmod 755 "$destination/out/$arch/actionlint"
done
