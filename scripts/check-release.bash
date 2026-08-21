#!/usr/bin/env bash
# Check the exact GoReleaser payload before signing or publishing it.
set -euo pipefail

if [[ $# != 2 || ! $1 =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "Usage: $0 VERSION ARTIFACT_DIRECTORY" >&2
  exit 1
fi
version=$1
dist=$(cd "$2" && pwd -P)
targets=(darwin_amd64 darwin_arm64 linux_386 linux_amd64 linux_armv6 linux_arm64 windows_386 windows_amd64 windows_arm64 freebsd_386 freebsd_amd64)
assets=()
for target in "${targets[@]}"; do
  extension=tar.gz
  if [[ $target == windows_* ]]; then extension=zip; fi
  archive="actionlint_${version}_${target}.${extension}"
  assets+=("$archive" "$archive.cdx.json")
done
checksum="actionlint_${version}_checksums.txt"

# Reject missing, extra, non-regular, or repeated assets before trusting paths
# from the checksum file. GoReleaser owns archive contents and naming.
shopt -s nullglob dotglob
files=("$dist/"*)
for file in "${files[@]}"; do
  if [[ ! -f $file || -L $file ]]; then
    echo "Unexpected release entry: $file" >&2
    exit 1
  fi
done
diff -u <(printf '%s\n' "${assets[@]}" "$checksum" | LC_ALL=C sort) \
  <(printf '%s\n' "${files[@]##*/}" | LC_ALL=C sort)
diff -u <(printf '%s\n' "${assets[@]}" | LC_ALL=C sort) \
  <(sed -E 's/^[0-9a-f]{64}  //' "$dist/$checksum" | LC_ALL=C sort)
(cd "$dist" && sha256sum --strict --check "$checksum")
printf 'Verified %s release assets\n' "${#assets[@]}"
