#!/usr/bin/env bash
set -euo pipefail

digest=$1
if [[ ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  echo 'Expected a SHA-256 image digest.' >&2
  exit 1
fi

# Do not use the runner's Docker login or GitHub token for this check. A newly
# created GHCR package is private until its visibility is configured separately.
token=$(curl -q --fail --silent --show-error --proto '=https' --tlsv1.2 --get \
  --data-urlencode service=ghcr.io \
  --data-urlencode scope=repository:astral-sh/actionlint:pull \
  https://ghcr.io/token | jq -er '.token')
headers=$(curl -q --fail --silent --show-error --proto '=https' --tlsv1.2 --head \
  --header "Authorization: Bearer $token" \
  --header 'Accept: application/vnd.oci.image.index.v1+json' \
  "https://ghcr.io/v2/astral-sh/actionlint/manifests/$digest")
tr -d '\r' <<< "$headers" | grep -Fxi "docker-content-digest: $digest"
