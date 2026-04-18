# Packaging (WinGet + Homebrew Tap) and Doc Sweep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a repo-owned `packaging/` directory containing a WinGet manifest skeleton (id `MITPOAI.PookiePaws`) and a Homebrew formula intended for the `mitpoai/homebrew-pookiepaws` tap. Wire goreleaser hooks where reasonable, document the publish steps in `RELEASE_CHECKLIST.md`, and sweep the project's top-level docs to remove stale CLI inventory and cover the new install/update channels.

**Architecture:** All packaging artifacts live in `packaging/`. The actual external publish surfaces (winget-pkgs PR; homebrew tap repo) remain manual release-time actions documented in `RELEASE_CHECKLIST.md` — no CI automation in this plan. The Homebrew formula uses release asset URLs and SHA256s that goreleaser already computes; a small `packaging/scripts/render-formula.sh` helper renders the formula from a template after a release. Docs are updated in lockstep so README, ARCHITECTURE, CHANGELOG, RELEASE_CHECKLIST, and CONTRIBUTING are mutually consistent and reflect the actual `pookie` CLI surface (including the new `version --check` and `research` subcommands from Plans 1 and 2).

**Tech Stack:** YAML (WinGet manifest schema 1.6), Ruby (Homebrew formula DSL), Bash + a small Go helper (`cmd/render-formula`) to render the formula template from goreleaser checksums. No new runtime dependencies.

**Depends on:** Plans 1 (`pookie version --check`) and 2 (`pookie research`) for the CLI surface that the docs describe. This plan can be drafted in parallel but its docs become accurate only after 1 and 2 land.

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `packaging/winget/MITPOAI.PookiePaws.yaml` | NEW — version manifest (1.6 schema) |
| `packaging/winget/MITPOAI.PookiePaws.installer.yaml` | NEW — installer manifest (URL + SHA256 placeholders) |
| `packaging/winget/MITPOAI.PookiePaws.locale.en-US.yaml` | NEW — locale manifest |
| `packaging/winget/README.md` | NEW — how to update version + checksums per release |
| `packaging/homebrew/pookie.rb.tmpl` | NEW — formula template with `{{.Version}}`, `{{.URLDarwinArm64}}`, etc. |
| `packaging/homebrew/README.md` | NEW — how to publish to the tap repo |
| `packaging/scripts/render-formula.sh` | NEW — wraps `cmd/render-formula` to fill the template from a goreleaser dist |
| `cmd/render-formula/main.go` | NEW — small Go binary that reads `dist/checksums.txt` + a tag and renders `pookie.rb` |
| `cmd/render-formula/main_test.go` | NEW — template render tests |
| `.goreleaser.yml` | MODIFY — emit a `dist/checksums.txt` (likely already does) and add an `after_publish` hook documented in checklist |
| `RELEASE_CHECKLIST.md` | MODIFY — append WinGet + Homebrew publish steps |
| `README.md` | MODIFY — install/update sections list winget, brew tap, install.{ps1,sh} |
| `ARCHITECTURE.md` | MODIFY — replace stale CLI inventory; add packaging diagram bullet |
| `CHANGELOG.md` | MODIFY — note packaging additions |
| `CONTRIBUTING.md` | MODIFY — link `packaging/` and explain release flow |

---

## Phase A — WinGet manifest skeleton

The WinGet community repo (`microsoft/winget-pkgs`) accepts PRs adding/updating manifests under `manifests/m/MITPOAI/PookiePaws/<version>/`. We host editable templates inside our repo so contributors can update them in lockstep with our release.

### Task A1: Create the three WinGet manifests

**Files:**
- Create: `packaging/winget/MITPOAI.PookiePaws.yaml`
- Create: `packaging/winget/MITPOAI.PookiePaws.installer.yaml`
- Create: `packaging/winget/MITPOAI.PookiePaws.locale.en-US.yaml`
- Create: `packaging/winget/README.md`

- [ ] **Step 1: Write the version manifest**

Create `packaging/winget/MITPOAI.PookiePaws.yaml`:

```yaml
# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.1.6.0.schema.json

PackageIdentifier: MITPOAI.PookiePaws
PackageVersion: 0.5.2
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
```

- [ ] **Step 2: Write the installer manifest**

Create `packaging/winget/MITPOAI.PookiePaws.installer.yaml`:

```yaml
# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.1.6.0.schema.json

PackageIdentifier: MITPOAI.PookiePaws
PackageVersion: 0.5.2
Platform:
  - Windows.Desktop
MinimumOSVersion: 10.0.17763.0
InstallerType: zip
NestedInstallerType: portable
NestedInstallerFiles:
  - RelativeFilePath: pookie.exe
    PortableCommandAlias: pookie
Commands:
  - pookie
ReleaseDate: 2026-04-18
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/MITPOAI/PookiePaws/releases/download/v0.5.2/pookiepaws_0.5.2_windows_amd64.zip
    InstallerSha256: REPLACE_WITH_SHA256_FROM_RELEASE
  - Architecture: arm64
    InstallerUrl: https://github.com/MITPOAI/PookiePaws/releases/download/v0.5.2/pookiepaws_0.5.2_windows_arm64.zip
    InstallerSha256: REPLACE_WITH_SHA256_FROM_RELEASE
ManifestType: installer
ManifestVersion: 1.6.0
```

(Asset filenames must match what `.goreleaser.yml` actually produces; verify after the next release run. If the archive layout differs — for instance, the binary is at a nested path — adjust `RelativeFilePath`.)

- [ ] **Step 3: Write the locale manifest**

Create `packaging/winget/MITPOAI.PookiePaws.locale.en-US.yaml`:

```yaml
# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.1.6.0.schema.json

PackageIdentifier: MITPOAI.PookiePaws
PackageVersion: 0.5.2
PackageLocale: en-US
Publisher: MITPO AI
PublisherUrl: https://github.com/MITPOAI
PublisherSupportUrl: https://github.com/MITPOAI/PookiePaws/issues
PackageName: PookiePaws
PackageUrl: https://github.com/MITPOAI/PookiePaws
License: See LICENSE
LicenseUrl: https://github.com/MITPOAI/PookiePaws/blob/main/LICENSE
ShortDescription: Local-first AI agent for orchestrating skills, research, and outreach.
Description: |
  PookiePaws (binary: pookie) is an open-source AI agent that runs locally,
  exposes a web console, and orchestrates research, dossier generation, and
  outbound actions through a pluggable skill system.
Tags:
  - ai
  - agent
  - automation
  - cli
  - research
ManifestType: defaultLocale
ManifestVersion: 1.6.0
```

- [ ] **Step 4: Write `packaging/winget/README.md`**

Create `packaging/winget/README.md`:

````markdown
# WinGet manifests

These manifests are the source of truth for the `MITPOAI.PookiePaws`
package in the official `winget-pkgs` repo. They are updated in this repo
first, then submitted upstream as part of each release.

## Per-release update

1. Bump `PackageVersion` in all three files to the new tag (without the `v`).
2. Update `InstallerUrl` for each architecture to the new release asset URL.
3. Replace each `InstallerSha256` with the value from the release's
   `checksums.txt` (look for the matching `.zip`).
4. Update `ReleaseDate` to the publish date in `YYYY-MM-DD`.
5. Validate locally with the `winget` CLI:

       winget validate --manifest packaging/winget

6. Open a PR against `microsoft/winget-pkgs` adding the files under
   `manifests/m/MITPOAI/PookiePaws/<version>/`. The community bot will
   re-validate and run sandboxed install tests.

## First-time submission notes

- The `microsoft/winget-pkgs` PR template asks for installer URLs that are
  permanent (not pre-release). Use the GitHub release tag URL only after
  the release is published.
- Manifests must round-trip through `winget validate` cleanly.
````

- [ ] **Step 5: Commit**

```bash
git add packaging/winget/
git commit -m "feat(packaging): add WinGet manifest skeleton for MITPOAI.PookiePaws"
```

---

## Phase B — Homebrew tap formula + renderer

Homebrew taps are separate repos (`mitpoai/homebrew-pookiepaws`). We keep the formula template in this repo and render it after each goreleaser run. The rendered formula is then committed to the tap repo manually (or via a future CI hook).

### Task B1: Formula template

**Files:**
- Create: `packaging/homebrew/pookie.rb.tmpl`
- Create: `packaging/homebrew/README.md`

- [ ] **Step 1: Write the template**

Create `packaging/homebrew/pookie.rb.tmpl`:

```ruby
class Pookie < Formula
  desc "Local-first AI agent for skills, research, and outreach"
  homepage "https://github.com/MITPOAI/PookiePaws"
  version "{{ .Version }}"
  license "See LICENSE"

  on_macos do
    on_arm do
      url "https://github.com/MITPOAI/PookiePaws/releases/download/v{{ .Version }}/pookiepaws_{{ .Version }}_darwin_arm64.tar.gz"
      sha256 "{{ .DarwinArm64SHA256 }}"
    end
    on_intel do
      url "https://github.com/MITPOAI/PookiePaws/releases/download/v{{ .Version }}/pookiepaws_{{ .Version }}_darwin_amd64.tar.gz"
      sha256 "{{ .DarwinAmd64SHA256 }}"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/MITPOAI/PookiePaws/releases/download/v{{ .Version }}/pookiepaws_{{ .Version }}_linux_arm64.tar.gz"
      sha256 "{{ .LinuxArm64SHA256 }}"
    end
    on_intel do
      url "https://github.com/MITPOAI/PookiePaws/releases/download/v{{ .Version }}/pookiepaws_{{ .Version }}_linux_amd64.tar.gz"
      sha256 "{{ .LinuxAmd64SHA256 }}"
    end
  end

  def install
    bin.install "pookie"
  end

  test do
    assert_match "pookie v#{version}", shell_output("#{bin}/pookie version")
  end
end
```

- [ ] **Step 2: Write `packaging/homebrew/README.md`**

Create `packaging/homebrew/README.md`:

````markdown
# Homebrew formula

The `pookie` formula lives in the public tap repo
[`mitpoai/homebrew-pookiepaws`](https://github.com/MITPOAI/homebrew-pookiepaws).
This directory holds the template and the rendering helper.

## Per-release publish

1. Run a release with goreleaser (produces `dist/checksums.txt`).
2. Render the formula:

       packaging/scripts/render-formula.sh dist v0.5.2 > /tmp/pookie.rb

3. Verify it parses:

       brew audit --new-formula --strict /tmp/pookie.rb

4. Open a PR (or push directly) to `mitpoai/homebrew-pookiepaws` replacing
   `Formula/pookie.rb` with the new file.

## Install for end users

    brew install mitpoai/pookiepaws/pookie

(The tap is auto-added by `brew install` when the `<owner>/<repo>/<formula>`
syntax is used.)
````

- [ ] **Step 3: Commit**

```bash
git add packaging/homebrew/
git commit -m "feat(packaging): add Homebrew formula template for pookie tap"
```

---

### Task B2: Formula renderer (Go)

**Files:**
- Create: `cmd/render-formula/main.go`
- Create: `cmd/render-formula/main_test.go`
- Create: `packaging/scripts/render-formula.sh`

- [ ] **Step 1: Write failing tests**

Create `cmd/render-formula/main_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	in := `abc123  pookiepaws_0.5.2_darwin_arm64.tar.gz
def456  pookiepaws_0.5.2_darwin_amd64.tar.gz
789ghi  pookiepaws_0.5.2_linux_arm64.tar.gz
000jkl  pookiepaws_0.5.2_linux_amd64.tar.gz
xxxxxx  pookiepaws_0.5.2_windows_amd64.zip
`
	got, err := parseChecksums(strings.NewReader(in), "0.5.2")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := assets{
		DarwinArm64SHA256: "abc123",
		DarwinAmd64SHA256: "def456",
		LinuxArm64SHA256:  "789ghi",
		LinuxAmd64SHA256:  "000jkl",
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestRenderTemplate(t *testing.T) {
	tmpl := `version "{{ .Version }}" sha "{{ .DarwinArm64SHA256 }}"`
	out, err := renderFormulaText(tmpl, "0.5.2", assets{DarwinArm64SHA256: "abc"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != `version "0.5.2" sha "abc"` {
		t.Fatalf("got %q", out)
	}
}

func TestParseChecksumsMissingPlatform(t *testing.T) {
	in := `abc  pookiepaws_0.5.2_darwin_arm64.tar.gz
`
	_, err := parseChecksums(strings.NewReader(in), "0.5.2")
	if err == nil {
		t.Fatal("expected error when platforms missing")
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./cmd/render-formula/... -v`
Expected: FAIL — package undefined.

- [ ] **Step 3: Implement `main.go`**

Create `cmd/render-formula/main.go`:

```go
// Command render-formula fills the Homebrew formula template using SHA256s
// from a goreleaser checksums.txt file. Output is written to stdout.
//
// Usage:
//
//	render-formula --dist dist --version 0.5.2 --template packaging/homebrew/pookie.rb.tmpl
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type assets struct {
	Version            string
	DarwinArm64SHA256  string
	DarwinAmd64SHA256  string
	LinuxArm64SHA256   string
	LinuxAmd64SHA256   string
}

func main() {
	dist := flag.String("dist", "dist", "Path to goreleaser dist directory")
	version := flag.String("version", "", "Release version (e.g. 0.5.2; without v prefix)")
	tmplPath := flag.String("template", "packaging/homebrew/pookie.rb.tmpl", "Template file")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "--version required")
		os.Exit(2)
	}
	v := strings.TrimPrefix(*version, "v")

	checksumsPath := filepath.Join(*dist, "checksums.txt")
	f, err := os.Open(checksumsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", checksumsPath, err)
		os.Exit(1)
	}
	defer f.Close()

	a, err := parseChecksums(f, v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	tmplBytes, err := os.ReadFile(*tmplPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read template: %v\n", err)
		os.Exit(1)
	}
	out, err := renderFormulaText(string(tmplBytes), v, a)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func parseChecksums(r io.Reader, version string) (assets, error) {
	wantSuffixes := map[string]*string{
		"darwin_arm64.tar.gz": nil,
		"darwin_amd64.tar.gz": nil,
		"linux_arm64.tar.gz":  nil,
		"linux_amd64.tar.gz":  nil,
	}
	got := assets{Version: version}
	wantSuffixes["darwin_arm64.tar.gz"] = &got.DarwinArm64SHA256
	wantSuffixes["darwin_amd64.tar.gz"] = &got.DarwinAmd64SHA256
	wantSuffixes["linux_arm64.tar.gz"] = &got.LinuxArm64SHA256
	wantSuffixes["linux_amd64.tar.gz"] = &got.LinuxAmd64SHA256

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		sum, file := fields[0], fields[1]
		for suffix, target := range wantSuffixes {
			expected := fmt.Sprintf("pookiepaws_%s_%s", version, suffix)
			if file == expected {
				*target = sum
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return assets{}, err
	}
	for suffix, target := range wantSuffixes {
		if *target == "" {
			return assets{}, fmt.Errorf("checksums.txt missing entry for %s", suffix)
		}
	}
	return got, nil
}

func renderFormulaText(tmpl, version string, a assets) (string, error) {
	t, err := template.New("formula").Parse(tmpl)
	if err != nil {
		return "", err
	}
	a.Version = version
	var sb strings.Builder
	if err := t.Execute(&sb, a); err != nil {
		return "", err
	}
	return sb.String(), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/render-formula/... -v`
Expected: PASS.

- [ ] **Step 5: Add the wrapper script**

Create `packaging/scripts/render-formula.sh`:

```bash
#!/usr/bin/env bash
# Render the Homebrew formula from a goreleaser dist directory.
# Usage: packaging/scripts/render-formula.sh <dist-dir> <version>
set -euo pipefail

dist="${1:-dist}"
version="${2:?version required (e.g. 0.5.2 or v0.5.2)}"

exec go run ./cmd/render-formula \
  --dist "$dist" \
  --version "${version#v}" \
  --template packaging/homebrew/pookie.rb.tmpl
```

Make it executable:

```bash
chmod +x packaging/scripts/render-formula.sh
```

- [ ] **Step 6: Manual smoke (no real release needed)**

Create a synthetic checksums file:

```bash
mkdir -p /tmp/dist
cat > /tmp/dist/checksums.txt <<'EOF'
0000  pookiepaws_0.5.2_darwin_arm64.tar.gz
1111  pookiepaws_0.5.2_darwin_amd64.tar.gz
2222  pookiepaws_0.5.2_linux_arm64.tar.gz
3333  pookiepaws_0.5.2_linux_amd64.tar.gz
EOF
go run ./cmd/render-formula --dist /tmp/dist --version 0.5.2
```

Expected: a fully rendered Ruby formula on stdout with the four SHAs filled in.

- [ ] **Step 7: Commit**

```bash
git add cmd/render-formula/ packaging/scripts/render-formula.sh
git commit -m "feat(packaging): add render-formula tool to fill Homebrew template from checksums"
```

---

## Phase C — Goreleaser hook + release checklist

### Task C1: Confirm `.goreleaser.yml` emits checksums

**Files:**
- Modify (only if needed): `.goreleaser.yml`

- [ ] **Step 1: Inspect the existing config**

Run: `grep -n "checksum" .goreleaser.yml`

If a `checksum:` block exists with a `name_template`, ensure it produces `dist/checksums.txt`. If absent, add:

```yaml
checksum:
  name_template: "checksums.txt"
  algorithm: sha256
```

- [ ] **Step 2: Verify archive naming**

Run: `grep -n "name_template\|archives:" .goreleaser.yml`

The Homebrew/WinGet templates expect archives named `pookiepaws_<version>_<os>_<arch>.tar.gz` (and `.zip` for Windows). If the existing `archives.name_template` differs, either:

(a) update `.goreleaser.yml` to match, **or**
(b) update the asset names in `packaging/winget/MITPOAI.PookiePaws.installer.yaml` and the suffix patterns in `cmd/render-formula/main.go` (`parseChecksums` `expected := ...` line) to match what goreleaser actually produces.

Pick (a) if it doesn't break existing release downloads; otherwise (b).

- [ ] **Step 3: Commit (only if changes were made)**

```bash
git add .goreleaser.yml
git commit -m "build: ensure goreleaser emits dist/checksums.txt for packaging hooks"
```

---

### Task C2: Update `RELEASE_CHECKLIST.md`

**Files:**
- Modify: `RELEASE_CHECKLIST.md`

- [ ] **Step 1: Append packaging section**

Add a new section near the end of the checklist:

````markdown
## Packaging publish

After the GitHub release is published and assets are live:

### Homebrew tap (`mitpoai/homebrew-pookiepaws`)

- [ ] Render the formula:

      packaging/scripts/render-formula.sh dist <new-version> > /tmp/pookie.rb

- [ ] Audit it: `brew audit --new-formula --strict /tmp/pookie.rb`
- [ ] In a clone of `mitpoai/homebrew-pookiepaws`, replace `Formula/pookie.rb`
      with the new file, commit (`pookie <new-version>`), and push to `main`.
- [ ] Smoke-test from a clean machine:

      brew update && brew install mitpoai/pookiepaws/pookie && pookie version

### WinGet (`microsoft/winget-pkgs`)

- [ ] Update version, URLs, SHA256s, and `ReleaseDate` in
      `packaging/winget/*.yaml` (see `packaging/winget/README.md`).
- [ ] `winget validate --manifest packaging/winget`
- [ ] Open a PR against `microsoft/winget-pkgs` placing the three files under
      `manifests/m/MITPOAI/PookiePaws/<new-version>/`.
- [ ] Wait for the bot validation to pass; respond to feedback if any.
- [ ] Smoke-test on Windows: `winget upgrade MITPOAI.PookiePaws`.

### Direct install scripts

- [ ] Verify `install.sh` and `install.ps1` resolve to the new tag (they
      typically read from the GitHub releases API).
````

- [ ] **Step 2: Commit**

```bash
git add RELEASE_CHECKLIST.md
git commit -m "docs(release): WinGet and Homebrew publish steps"
```

---

## Phase D — Top-level docs sweep

### Task D1: README — install/update channels and CLI snapshot

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace the Installation section**

Find the existing "Installation" or "Install" section. Replace its contents with:

````markdown
## Installation

### macOS / Linux

    brew install mitpoai/pookiepaws/pookie

Or use the install script directly:

    curl -fsSL https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.sh | bash

### Windows

    winget install MITPOAI.PookiePaws

Or:

    irm https://raw.githubusercontent.com/MITPOAI/PookiePaws/main/install.ps1 | iex

### From source

    go install github.com/mitpoai/pookiepaws/cmd/pookie@latest

## Updating

| Channel        | Command                                          |
|----------------|--------------------------------------------------|
| Homebrew tap   | `brew upgrade mitpoai/pookiepaws/pookie`         |
| WinGet         | `winget upgrade MITPOAI.PookiePaws`              |
| Install script | re-run `install.sh` or `install.ps1`             |

`pookie version --check` performs a live lookup against GitHub Releases.
A short notice on stderr also appears during interactive commands when an
update is available; opt out with `POOKIEPAWS_NO_UPDATE_NOTIFIER=1`
(see Plan 1's update notifier section).
````

- [ ] **Step 2: Add or refresh the CLI overview**

Find the "Commands" / "CLI" section if any; otherwise create one. Replace with the actual current command list (verify against `cmd/pookie/main.go` switch):

```markdown
## CLI overview

| Command                          | Purpose |
|----------------------------------|---------|
| `pookie start`                   | Run the daemon + web console (with research scheduler) |
| `pookie status`                  | Query a running daemon |
| `pookie chat`                    | Interactive chat session |
| `pookie run --skill <id>`        | Headless skill execution |
| `pookie list`                    | List installed skills |
| `pookie install <repo>`          | Install a skill from GitHub |
| `pookie init`                    | Interactive setup wizard |
| `pookie research <subcommand>`   | Watchlists, schedule, refresh, status, recommendations |
| `pookie sessions / approvals / audit` | Inspect persisted state |
| `pookie context / memory`        | Inspect prompt and brain memory |
| `pookie doctor`                  | Diagnostics |
| `pookie smoke`                   | Operator smoke checks |
| `pookie version [--check]`       | Print version; `--check` queries GitHub Releases |
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs(readme): document install channels and refresh CLI inventory"
```

---

### Task D2: ARCHITECTURE.md — refresh stale CLI inventory

**Files:**
- Modify: `ARCHITECTURE.md`

- [ ] **Step 1: Read the current document**

Run: `grep -n "## " ARCHITECTURE.md` to map sections. Look specifically for any section that enumerates CLI commands or CLI-to-internal-package wiring.

- [ ] **Step 2: Replace stale CLI inventory**

In the section that lists CLI commands, replace it with the same table as in `README.md` Step D1.2. Add one paragraph below it:

```markdown
The CLI dispatcher is a custom switch in `cmd/pookie/main.go` (no external
flag library beyond `flag`). Subcommands are implemented as `cmdX(args
[]string)` handlers. New subcommands should add a `case` to the switch and
a corresponding `cmdX.go` file. The daemon (`pookie start`) is the only
command that launches background goroutines (HTTP server, research
scheduler).
```

- [ ] **Step 3: Add a packaging bullet to the system overview**

If `ARCHITECTURE.md` has a "Distribution" or "Packaging" section, add:

```markdown
- WinGet manifests live in `packaging/winget/`, formula template in
  `packaging/homebrew/`, with `cmd/render-formula` filling SHA256s from
  goreleaser output.
```

If no such section exists, add one near the end.

- [ ] **Step 4: Commit**

```bash
git add ARCHITECTURE.md
git commit -m "docs(architecture): refresh CLI inventory and document packaging layout"
```

---

### Task D3: CONTRIBUTING.md and CHANGELOG.md

**Files:**
- Modify: `CONTRIBUTING.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: CONTRIBUTING — packaging and release pointers**

Append a new section to `CONTRIBUTING.md`:

````markdown
## Packaging

Distribution channels (Homebrew tap, WinGet, install scripts) are described
in `packaging/`. The release flow is documented in `RELEASE_CHECKLIST.md`.
External registry submissions (winget-pkgs PRs, tap-repo updates) are done
manually at release time — they are not automated in CI today.

## Adding a CLI subcommand

1. Add a `case "<name>": cmd<Name>(os.Args[2:])` in `cmd/pookie/main.go`.
2. Implement `cmd<Name>` in a new file `cmd/pookie/<name>.go`.
3. Update the CLI table in both `README.md` and `ARCHITECTURE.md`.
4. If the command is interactive, attach `defer maybeShowUpdateNotice(...)`
   to its body; non-interactive commands must not call it.
````

- [ ] **Step 2: CHANGELOG — packaging entry**

Append to the `[Unreleased]` section:

```markdown
### Added
- WinGet manifest skeleton at `packaging/winget/`.
- Homebrew formula template + renderer at `packaging/homebrew/` and
  `cmd/render-formula/`.
- Release checklist now covers WinGet and Homebrew publish steps.
```

- [ ] **Step 3: Commit**

```bash
git add CONTRIBUTING.md CHANGELOG.md
git commit -m "docs: document packaging contributions and release process"
```

---

## Verification Summary

- `go test ./cmd/render-formula/...` green.
- `packaging/scripts/render-formula.sh /tmp/dist 0.5.2` produces a valid Ruby formula given a synthetic `checksums.txt`.
- WinGet manifests parse; if the `winget` CLI is available, `winget validate --manifest packaging/winget` passes.
- README, ARCHITECTURE, CONTRIBUTING, RELEASE_CHECKLIST, CHANGELOG mention all of: WinGet, Homebrew tap, install scripts, `pookie version --check`, the new `pookie research` surface.
- `grep -rn "TODO\|TBD" packaging/` yields zero hits.
- The CLI table in README and ARCHITECTURE both match the actual `switch` in `cmd/pookie/main.go`.

## Out of scope (deferred)

- Submitting the WinGet PR or pushing to the Homebrew tap repo — these are release-time manual actions per `RELEASE_CHECKLIST.md`.
- Automating tap-repo updates from CI (e.g. via `goreleaser`'s built-in `brews` block writing to a tap repo with a deploy key) — possible later improvement, but the manual flow is auditable.
- MSI / `.pkg` installers, Snap, Chocolatey — not in this pass.
- Telemetry on install channel uptake — out of scope.
- Removing `install.sh` / `install.ps1` — they remain as the canonical fallback per the spec.
