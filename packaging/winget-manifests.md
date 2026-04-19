# WinGet manifests

`packaging/winget/` is the source of truth for the `MITPOAI.PookiePaws`
package submitted to `microsoft/winget-pkgs`.

Keep that directory limited to the three manifest YAML files only.
`winget validate --manifest <dir>` treats the directory as a manifest root and
fails if extra files such as `README.md` are present.

The checked-in manifests currently target GitHub release `v1.0.0`, published on
2026-04-07.

## Per-release update

1. Bump `PackageVersion` in all three files to the new tag (without the `v`).
2. Update `InstallerUrl` to the new release asset URL.
3. Replace `InstallerSha256` with the value from the release's
   `checksums.txt` for the matching `.zip`.
4. Update `ReleaseDate` to the publish date in `YYYY-MM-DD`.
5. Validate locally:

       winget validate --manifest packaging/winget

6. Open a PR against `microsoft/winget-pkgs` adding the files under
   `manifests/m/MITPOAI/PookiePaws/<version>/`.

## Architectures

Only `x64` ships for Windows. `goreleaser` does not currently build a Windows
arm64 binary (`ignore: { goos: windows, goarch: arm64 }` in
`.goreleaser.yml`). If that changes, add a second `Installers:` entry with the
matching asset URL and SHA256.
