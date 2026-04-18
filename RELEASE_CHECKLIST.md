# Release Checklist

## Brain and provider validation

- Verify no stale preset model IDs remain in `internal/cli/providers.go`
- Run `pookie init` once from a clean home directory
- Run `pookie doctor --brain`
- Confirm these cases are covered:
  - dead endpoint
  - bad API key
  - valid endpoint with invalid model
  - valid endpoint with valid model

## Smoke coverage

- Run `pookie smoke --cli`
- Run `pookie smoke --api`
- Run `pookie smoke --provider` with one real provider before tagging a release
- Confirm `pookie smoke --json` output is usable in issue reports

## Operator workflows

- Confirm `pookie chat` fails fast on invalid provider/model config
- Confirm `pookie sessions --trace` shows technical failure details
- Confirm `pookie approvals`, `pookie audit`, and `pookie doctor` still work from a clean runtime

## Packaging publish

After the GitHub release is published and assets are live:

### Homebrew tap (`mitpoai/homebrew-pookiepaws`)

- Render the formula:

      packaging/scripts/render-formula.sh dist <new-version> > /tmp/pookie.rb

- Audit it: `brew audit --new-formula --strict /tmp/pookie.rb`
- In a clone of `mitpoai/homebrew-pookiepaws`, replace `Formula/pookie.rb`
  with the new file, commit (`pookie <new-version>`), and push to `main`.
- Smoke-test from a clean machine:

      brew update && brew install mitpoai/pookiepaws/pookie && pookie version

### WinGet (`microsoft/winget-pkgs`)

- Update `PackageVersion`, `InstallerUrl`, `InstallerSha256`, and `ReleaseDate`
  in `packaging/winget/*.yaml` (see `packaging/winget/README.md`).
- `winget validate --manifest packaging/winget`
- Open a PR against `microsoft/winget-pkgs` placing the three files under
  `manifests/m/MITPOAI/PookiePaws/<new-version>/`.
- Wait for the bot validation to pass; respond to feedback if any.
- Smoke-test on Windows: `winget upgrade MITPOAI.PookiePaws`.

### Direct install scripts

- Verify `install.sh` and `install.ps1` resolve to the new tag (they
  typically read from the GitHub releases API).
