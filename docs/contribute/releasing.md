# Releasing BuildMax

> **Audience:** maintainers · **Status:** current

BuildMax releases are created from `main` by maintainers. Pushing a version tag
starts `.github/workflows/release.yml`, which builds archives and the container
image, generates checksums and SBOMs, scans the image, and publishes provenance
attestations.

## Versioning

BuildMax follows Semantic Versioning. During alpha, use tags such as
`v0.2.0-alpha.1`; GoReleaser marks prerelease tags as GitHub pre-releases.
Stable releases omit the prerelease suffix.

Never move or reuse a published tag. Fix a bad release with a new patch or
prerelease version.

## Prepare

1. Confirm `main` is up to date and all required CI checks pass.
2. Move relevant entries from `Unreleased` in [CHANGELOG.md](../../CHANGELOG.md)
   into a dated version section and restore an empty `Unreleased` section.
3. Review [SECURITY.md](../../SECURITY.md), installation instructions, known
   limitations, and configuration examples for release-specific changes.
4. Run the local verification commands:

   ```bash
   ./make test
   ./make build
   ./make npm-licenses
   npm exec --yes --package=markdownlint-cli2@0.23.2 -- markdownlint-cli2
   goreleaser check
   goreleaser release --snapshot --clean --skip=publish,docker
   ./make verify-archive --all
   ./make verify-archive
   ```

The snapshot requires GoReleaser `v2.17.1`, Syft `v1.51.0`, and
`go-licenses v1.6.0`. CI installs those exact versions.

## Signing And Provenance

Alpha releases use GitHub Artifact Attestations for archives and SBOMs, plus
the SBOM and provenance attestations produced by Docker Buildx for the GHCR
image. They do not carry a separate Cosign signature. This keeps one documented
keyless verification path for GitHub-hosted source and artifacts instead of
asking users to understand two equivalent identity systems.

Revisit Cosign if BuildMax publishes images outside GHCR, distributes artifacts
outside GitHub Releases, or needs signatures that must be verified without
GitHub's attestation service. Consumers should identify container images by
digest rather than relying on a mutable tag.

## Publish

Create an annotated tag on the reviewed `main` commit and push only that tag:

```bash
git tag -a v0.2.0-alpha.1 -m "v0.2.0-alpha.1"
git push origin v0.2.0-alpha.1
```

Do not create release tags from a local commit that is not already on `main`.
The release workflow owns GitHub Release creation and GHCR publication.

## Verify

After the workflow completes:

1. Confirm every expected platform archive, `checksums.txt`, and one SPDX SBOM
   per archive are attached to the GitHub Release.
2. Download one archive, verify its checksum, and run `buildmax version`.
3. Confirm the archive contains `LICENSE`, `NOTICE-THIRD-PARTY`, `README.md`,
   `SECURITY.md`, `CHANGELOG.md`, and `config-examples/`.
4. Verify the GitHub attestation as described in
   [the installation guide](../start/install.md).
5. Pull `ghcr.io/gougoujiang/buildmax:<version>` by digest and confirm the
   container starts. Alpha versions must not move the `latest` tag.
6. Confirm `ghcr.io/gougoujiang/buildmax-portal:<version>` exists and carries
   the **same** version. It is published by a separate workflow
   (`.github/workflows/portal-image.yml`) triggered by the same tag, so a
   failure there leaves the binaries released and the Portal image missing —
   which is the intended trade, but it has to be checked rather than assumed.
   Run it with `BUILDMAX_API_BASE` set and confirm `/config.js` carries the
   value.

The Wails desktop application is not part of the release workflow because its
native bundles are not yet signed or notarized.

## Respond to a Bad Release

Do not silently replace release artifacts or reuse the tag. Add a prominent
warning to the affected GitHub Release, open a tracking issue when disclosure
is safe, and publish a corrected version. For a security issue, follow the
private process in [SECURITY.md](../../SECURITY.md) before public discussion.
