# Verify Astral releases

Choose a reviewed full source commit and expected artifact digest. Do not use a floating release or
container tag as an integrity check. Maintainers should use the
[release procedure](https://github.com/astral-sh/actionlint/blob/main/RELEASING.md).

## Verify a release

Replace these placeholders with the values from your reviewed release:

```sh
tag=v1.X.Y
commit=FULL_REVIEWED_COMMIT_SHA
archive=actionlint_1.X.Y_linux_amd64.tar.gz
digest=EXPECTED_ARCHIVE_SHA256

gh release download "$tag" --repo astral-sh/actionlint \
  --pattern "$archive" --pattern actionlint_1.X.Y_attestation.json
printf '%s  %s\n' "$digest" "$archive" | sha256sum --check -
gh release verify "$tag" --repo astral-sh/actionlint
gh attestation verify "$archive" --repo astral-sh/actionlint \
  --bundle actionlint_1.X.Y_attestation.json \
  --signer-workflow astral-sh/actionlint/.github/workflows/release.yml \
  --source-ref refs/heads/main --source-digest "$commit" \
  --signer-digest "$commit" --deny-self-hosted-runners
```

Only unpack or execute the archive after verification succeeds. Release assets also include SHA-256
checksums and per-archive CycloneDX SBOMs. The attested source ref is `main`; the release tag is
created after the build and approval.

## Verify a container

Use the reviewed multi-platform image index digest. Verify the signed release manifest and the
registry attestation before running the image:

```sh
tag=v1.X.Y
version=${tag#v}
commit=FULL_REVIEWED_COMMIT_SHA
digest=sha256:FULL_REVIEWED_IMAGE_DIGEST
image=ghcr.io/astral-sh/actionlint

gh release download "$tag" --repo astral-sh/actionlint \
  --pattern "actionlint_${version}_container.json" \
  --pattern "actionlint_${version}_container-metadata-attestation.json"
gh release verify "$tag" --repo astral-sh/actionlint
gh attestation verify "actionlint_${version}_container.json" --repo astral-sh/actionlint \
  --bundle "actionlint_${version}_container-metadata-attestation.json" \
  --signer-workflow astral-sh/actionlint/.github/workflows/publish-release-container.yml \
  --source-ref refs/heads/main --source-digest "$commit" \
  --signer-digest "$commit" --deny-self-hosted-runners
jq -e --arg digest "$digest" --arg commit "$commit" \
  '.digest == $digest and .commit == $commit' "actionlint_${version}_container.json"
gh attestation verify "oci://$image@$digest" --repo astral-sh/actionlint --bundle-from-oci \
  --signer-workflow astral-sh/actionlint/.github/workflows/publish-release-container.yml \
  --source-ref refs/heads/main --source-digest "$commit" \
  --signer-digest "$commit" --deny-self-hosted-runners
docker run --rm "$image@$digest" -version
```

The index digest selects the verified AMD64 or ARM64 image. The release includes each platform's
digest, full-image SPDX SBOM, and provenance. Signatures establish identity and integrity; they do not
replace source review or semantic regression testing.
