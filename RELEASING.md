# Releasing actionlint

This guide is for Astral maintainers. Consumers should follow the [artifact verification
instructions](docs/releasing.md).

## Versioning

There is no release version constant to bump in the source tree. `command.go` leaves `version` empty
for source builds; GoReleaser injects the selected version with linker flags. Choose an unused
`v1.x.y` tag, optionally with a SemVer prerelease suffix. Update release notes or documentation in a
reviewed PR when needed, but do not run the inherited `scripts/bump-version.bash`: it pushes to `main`
and uses upstream publishing identities.

## Publish

Publication must already be enabled through the repository's reviewed release protections. The
workflow requires `RELEASE_ENABLED=true`, protected `release-gate` and `release` environments, and
immutable GitHub releases/tags. Do not weaken those controls to get a release through.

1. Merge the reviewed changes and confirm the intended `main` commit is green.
2. Dispatch `Release` with `tag=dry-run`. This builds and verifies all release archives and both
   container platforms without signing or publishing.
3. Dispatch `Release` on `main` with the unused version tag. Check the run's exact source SHA and
   artifacts before approving `release-gate`.
4. Confirm the immutable GitHub release and its assets verify, then check anonymous access to the
   signed GHCR image by digest using the consumer instructions.

The workflow checks out the exact source SHA and requires a clean tree. Pinned GoReleaser, Syft,
and Go produce the archives and SBOMs once. A small payload check enforces the expected files and
checksums. Separate protected jobs attest and publish those same bytes with the repository's
`GITHUB_TOKEN`; they do not rebuild them. No prepare-release commit, long-lived signing key, broker
token, or upstream package-manager publication is involved.

The container verifier checks OCI blob hashes and descriptor sizes, image identity, embedded
actionlint bytes, and attestation subjects and predicate types. It exports SBOM and provenance
payloads; it does not claim complete SPDX inventory validation or every SLSA field.

Images are published only to `ghcr.io/astral-sh/actionlint`. Their version tags omit the leading `v`
and cannot replace different contents. Only a successful stable GitHub release advances `latest`.
A newly created GHCR package is private by default; the workflow stops before version tagging and
GitHub release publication unless the image is anonymously accessible. First-package visibility must
be configured through a separately approved activation step.

## Recover a partial release

Inspect the existing source SHA, tag, draft release, uploaded asset digests, and image digest before
retrying. Do not delete or move a release tag, retarget an image version, overwrite published assets,
or disable protections. A run that has already created its Git tag or draft requires a separately
reviewed recovery operation. If `main` changed during approval and GitHub rejects the repository
token's release creation, inspect any partial publication and dispatch a newly reviewed revision;
do not substitute a broader token.
