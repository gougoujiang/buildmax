# Releasing BuildMax

> **Audience:** maintainers · **Status:** current

BuildMax releases are created from `main` by maintainers. Pushing a version tag
starts `.github/workflows/release.yml`, which builds archives and the container
image, generates checksums and SBOMs, and publishes provenance attestations.

The container image is scanned before it is published, and a HIGH or CRITICAL
vulnerability with a fix available fails the release with nothing pushed. The
same holds for the Portal image in `.github/workflows/portal-image.yml`, which
scans on pull requests too. Both used to scan after pushing, where a finding
could only turn the job red: `v0.2.0-alpha.3` published two images carrying a
fixed openssl CVE and then failed on it.

## Versioning

BuildMax follows Semantic Versioning. During alpha, use tags such as
`v0.2.0-alpha.1`; GoReleaser marks prerelease tags as GitHub pre-releases.
Stable releases omit the prerelease suffix.

Never move or reuse a published tag. Fix a bad release with a new patch or
prerelease version.

## Prepare

1. Confirm `main` is up to date and all required CI checks pass.
2. Fold the unreleased entries into [CHANGELOG.md](../../CHANGELOG.md):

   ```bash
   ./make changelog                 # preview the section
   ./make changelog release 0.1.0   # write it and clear docs/changelog/
   ```

   Read the result before committing; the fold is mechanical and orders entries
   by filename, not by what matters most to a reader. It is also what the
   release page will say, so preview the body the workflow will publish:

   ```bash
   ./make release notes v0.2.0-alpha.1
   ```

   The body is `.github/release-notes.tmpl` with this version's section filled
   in: highlights and upgrade notes above the install steps, the categorized
   lists below them. A tag with no section in `CHANGELOG.md` fails the release.
3. Review [SECURITY.md](../../SECURITY.md), installation instructions, known
   limitations, and configuration examples for release-specific changes.
4. Run the local verification commands:

   ```bash
   ./make test
   ./make build
   ./make release licenses
   npm exec --yes --package=markdownlint-cli2@0.23.2 -- markdownlint-cli2
   goreleaser check
   goreleaser release --snapshot --clean --skip=publish,docker
   ./make release verify --all
   ./make release verify
   ```

The snapshot requires GoReleaser `v2.17.1`, Syft `v1.51.0`, and
`go-licenses v1.6.0`. CI installs those exact versions, and cutting a release
needs GoReleaser on your PATH for the snapshot above. Contributors who only edit
`.goreleaser.yaml` do not: that pull request and `./make check ci` validate it
with the same pinned version through the action and `go run`, respectively.

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
   per archive are attached to the GitHub Release, and that its body carries
   the version's changelog section.
2. Download one archive, verify its checksum, and run `buildmax version`.
3. Confirm the archive contains `LICENSE`, `NOTICE-THIRD-PARTY`, `README.md`,
   `SECURITY.md`, `CHANGELOG.md`, and `config-examples/`.
4. Verify the GitHub attestation as described in
   [the installation guide](../start/install.md).
5. Pull `ghcr.io/gougoujiang/buildmax:<version>` by digest and confirm the
   container starts. Alpha versions must not move the `latest` tag. The image
   scan already passed before publication, so a red release workflow here means
   something after the push failed, not a vulnerable image.
6. Confirm `ghcr.io/gougoujiang/buildmax-portal:<version>` exists and carries
   the **same** version. It is published by a separate workflow
   (`.github/workflows/portal-image.yml`) triggered by the same tag, so a
   failure there leaves the binaries released and the Portal image missing —
   which is the intended trade, but it has to be checked rather than assumed.
   Run it with `BUILDMAX_API_BASE` set and confirm `/config.js` carries the
   value, and that `/third-party-notices.txt` serves the npm license
   attributions.

The Wails desktop application is not part of the release workflow because its
native bundles are not yet signed or notarized.

## Respond to a Bad Release

Do not silently replace release artifacts or reuse the tag. Add a prominent
warning to the affected GitHub Release, open a tracking issue when disclosure
is safe, and publish a corrected version. For a security issue, follow the
private process in [SECURITY.md](../../SECURITY.md) before public discussion.
