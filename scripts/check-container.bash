#!/usr/bin/env bash
set -euo pipefail

image=$1
version=$2
fixtures="$(cd "$(dirname "$0")/testdata/container-smoke" && pwd)"
options=(--rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges)
docker run "${options[@]}" "$image" -version | head -n 1 | grep -Fx "$version"
docker run "${options[@]}" --entrypoint /usr/local/bin/shellcheck "$image" --version | grep -Fx 'version: 0.11.0'
docker run "${options[@]}" --entrypoint /usr/local/bin/ruff "$image" --version | grep -Fx 'ruff 0.16.2'
docker run "${options[@]}" --mount "type=bind,src=$fixtures,dst=/github/workspace,readonly" \
  --workdir /github/workspace "$image" clean.yaml
docker run "${options[@]}" --user 65532:65532 \
  --mount "type=bind,src=$fixtures,dst=/repo,readonly" --workdir /repo "$image" clean.yaml
set +e
output=$(docker run "${options[@]}" --mount "type=bind,src=$fixtures,dst=/repo,readonly" \
  --workdir /repo "$image" -oneline invalid.yaml 2>&1)
status=$?
set -e
if [[ "$status" != 1 ]]; then
  printf 'Expected lint exit 1, got %s:\n%s\n' "$status" "$output" >&2
  exit 1
fi
grep -F '[shellcheck]' <<< "$output"
grep -F '[ruff]' <<< "$output"
