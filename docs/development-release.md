# Release process (maintainers)

This repository uses **Release Drafter** for changelog drafting and a manual
**release** workflow for publishing.

## What is automated

1. PRs are auto-labeled by `.github/labeler.yml`.
2. Release Drafter groups merged PRs by label into a draft release.
3. The manual `release` workflow:
   - reads the current draft release
   - uses its resolved version tag (for example `v1.4.2`)
   - creates/pushes that tag if missing
   - builds Linux binaries
   - uploads binaries + `checksums.txt` to the release assets
   - publishes the draft

## Publishing a release

1. Ensure the release draft content looks correct under GitHub **Releases**.
2. Open **Actions** → **release** workflow → **Run workflow**.
3. Enter exactly `yes` for:
   - `Are you sure you want to publish the current release draft?`
4. Run the workflow from the branch you want to release (normally `main`).

If the input is anything other than `yes`, the workflow exits without
publishing.

## Linux binaries produced

The release workflow builds and uploads:

- `stevedore-agent-linux-386`
- `stevedore-agent-linux-amd64`
- `stevedore-agent-linux-arm64`
- `stevedore-agent-linux-loong64`
- `stevedore-agent-linux-mips`
- `stevedore-agent-linux-mipsle`
- `stevedore-agent-linux-mips64`
- `stevedore-agent-linux-mips64le`
- `stevedore-agent-linux-ppc64`
- `stevedore-agent-linux-ppc64le`
- `stevedore-agent-linux-riscv64`
- `stevedore-agent-linux-s390x`
- `stevedore-agent-linux-armv6`
- `stevedore-agent-linux-armv7`
- `checksums.txt`

These are attached as assets on the published GitHub Release.
