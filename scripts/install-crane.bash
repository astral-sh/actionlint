#!/usr/bin/env bash
set -euo pipefail

# This publisher runs on Linux AMD64. Keep the release and checksum together.
version=0.21.9
checksum=5c16d8ddb971cb1d5e6ed8b1e743da8224414eeba2c2762d8f1a61b2f095699e
destination=$1
install -d "$destination"
archive="$destination/crane.tar.gz"
curl --fail --location --proto '=https' --tlsv1.2 \
  "https://github.com/google/go-containerregistry/releases/download/v${version}/go-containerregistry_Linux_x86_64.tar.gz" \
  --output "$archive"
printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check -
tar -xOf "$archive" crane > "$destination/crane"
chmod 755 "$destination/crane"
