#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd "$(dirname "$0")" && pwd -P)
scratch=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-.}}/actionlint-release-test.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
mkdir "$scratch/fixture"
for target in darwin_amd64 darwin_arm64 linux_386 linux_amd64 linux_armv6 linux_arm64 windows_386 windows_amd64 windows_arm64 freebsd_386 freebsd_amd64; do
  extension=tar.gz
  if [[ $target == windows_* ]]; then extension=zip; fi
  archive="$scratch/fixture/actionlint_1.2.3_$target.$extension"
  printf 'archive fixture\n' > "$archive"
done
(cd "$scratch/fixture" && sha256sum ./*.tar.gz ./*.zip | sed 's@  \./@  @') > "$scratch/fixture/actionlint_1.2.3_checksums.txt"
check() { bash "$script_dir/check-release.bash" 1.2.3 "$1"; }
check "$scratch/fixture" > /dev/null
fresh_case() {
  case_dir=$(mktemp -d "$scratch/case.XXXXXX")
  cp "$scratch/fixture/"* "$case_dir/"
}
reject() {
  if check "$case_dir" > "$scratch/error" 2>&1; then
    echo "Accepted invalid release: $1" >&2
    exit 1
  fi
}
fresh_case
printf extra > "$case_dir/extra"
reject 'extra file'
fresh_case
rm "$case_dir/actionlint_1.2.3_linux_amd64.tar.gz"
reject 'missing archive'
fresh_case
cat "$scratch/fixture/actionlint_1.2.3_checksums.txt" >> "$case_dir/actionlint_1.2.3_checksums.txt"
reject 'duplicate checksum'
fresh_case
printf changed >> "$case_dir/actionlint_1.2.3_linux_amd64.tar.gz"
reject 'changed archive'
fresh_case
printf '%064d  ../outside\n' 0 > "$case_dir/actionlint_1.2.3_checksums.txt"
reject 'checksum path outside payload'
fresh_case
rm "$case_dir/actionlint_1.2.3_linux_amd64.tar.gz"
ln -s "$scratch/fixture/actionlint_1.2.3_linux_amd64.tar.gz" "$case_dir/actionlint_1.2.3_linux_amd64.tar.gz"
reject 'symlink asset'
printf 'Release payload tests passed\n'
