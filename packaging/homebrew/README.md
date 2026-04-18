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
