# WinGet manifests

These manifests are the source of truth for the `MITPOAI.PookiePaws`
package in the official `winget-pkgs` repo. They are updated in this repo
first, then submitted upstream as part of each release.

## Per-release update

1. Bump `PackageVersion` in all three files to the new tag (without the `v`).
2. Update `InstallerUrl` to the new release asset URL.
3. Replace `InstallerSha256` with the value from the release's
   `checksums.txt` (look for the matching `.zip`).
4. Update `ReleaseDate` to the publish date in `YYYY-MM-DD`.
5. Validate locally with the `winget` CLI:

       winget validate --manifest packaging/winget

6. Open a PR against `microsoft/winget-pkgs` adding the files under
   `manifests/m/MITPOAI/PookiePaws/<version>/`. The community bot will
   re-validate and run sandboxed install tests.

## Architectures

Only `x64` ships for Windows. `goreleaser` does not currently build a
Windows arm64 binary (`ignore: { goos: windows, goarch: arm64 }` in
`.goreleaser.yml`). If that ever changes, add a second `Installers:` entry
with `Architecture: arm64` and the matching asset URL + SHA256.

## First-time submission notes

- The `microsoft/winget-pkgs` PR template asks for installer URLs that are
  permanent (not pre-release). Use the GitHub release tag URL only after
  the release is published.
- Manifests must round-trip through `winget validate` cleanly.
