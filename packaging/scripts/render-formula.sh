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
