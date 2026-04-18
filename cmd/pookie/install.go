package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/skills"
)

// githubAPIBase is the base URL for the GitHub REST API. Tests can swap
// this to point at an httptest.Server.
var githubAPIBase = "https://api.github.com"

// cmdInstall downloads a SKILL.md from a public GitHub repository, validates
// it against the security sandbox rules, and saves it into the workspace
// skills directory.
//
// Supported argument formats:
//
//	owner/repo              resolves @latest tag, falls back to main → master → HEAD
//	owner/repo@ref          uses the specified branch or tag
//	owner/repo@latest       resolves to the highest semver tag
//	owner/repo/subpath      fetches from a subdirectory
//	owner/repo/subpath@ref  subpath at a specific ref
func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	yes := fs.Bool("yes", false, "Skip the confirmation prompt")
	fs.BoolVar(yes, "y", false, "Skip the confirmation prompt (shorthand)")
	home := fs.String("home", "", "override runtime home directory")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: pookie install [--yes] [--home <path>] <owner/repo[@ref]>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	repoArg := fs.Arg(0)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}
	_ = runtimeRoot // workspace is what we need for saving

	owner, repo, ref, subpath := parseRepoArg(repoArg)
	if owner == "" || repo == "" {
		p.Error("invalid repo path %q — expected owner/repo or owner/repo@ref", repoArg)
		os.Exit(1)
	}

	// Resolve @latest or empty ref to the highest semver tag.
	if ref == "" || ref == "latest" {
		if tag := resolveLatestTag(owner, repo); tag != "" {
			ref = tag
			p.Info("resolved latest tag → %s", ref)
		} else if ref == "latest" {
			p.Warning("no semver-tagged release found, falling back to main")
			ref = ""
		}
		// ref == "" still falls through main → master → HEAD via fetchSkillMD's existing chain
	}

	skillFile := "SKILL.md"
	if subpath != "" {
		skillFile = subpath + "/SKILL.md"
	}

	display := owner + "/" + repo
	if ref != "" {
		display += "@" + ref
	}

	spin := p.NewSpinner(fmt.Sprintf("Fetching SKILL.md from %s…", display))
	spin.Start()

	content, usedRef, err := fetchSkillMD(owner, repo, ref, skillFile)
	if err != nil {
		spin.Stop(false, "Download failed")
		p.Error("%v", err)
		os.Exit(1)
	}
	spin.Stop(true, fmt.Sprintf("Downloaded from %s/%s@%s (%d bytes)", owner, repo, usedRef, len(content)))

	sourceURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, usedRef, skillFile)
	p.Info("source: %s", sourceURL)

	// Parse and validate.
	manifest, err := skills.ParseSkillMarkdown(string(content))
	if err != nil {
		p.Error("Invalid SKILL.md: %v", err)
		os.Exit(1)
	}
	if err := validateInstallManifest(manifest); err != nil {
		p.Error("Security validation failed: %v", err)
		os.Exit(1)
	}
	p.Success("Schema validated: %s", manifest.Name)

	// Save to workspace/skills/<name>/SKILL.md
	destDir := filepath.Join(workspaceRoot, "skills", manifest.Name)
	destFile := filepath.Join(destDir, "SKILL.md")

	// Confirmation prompt (unless --yes).
	if !*yes {
		p.Blank()
		p.Plain("About to install skill %q from %s into %s", manifest.Name, sourceURL, destFile)
		if !confirmYesNo("Proceed?", os.Stdin) {
			p.Info("aborted")
			return
		}
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		p.Error("Create skill directory: %v", err)
		os.Exit(1)
	}

	if err := os.WriteFile(destFile, content, 0o644); err != nil {
		p.Error("Save SKILL.md: %v", err)
		os.Exit(1)
	}
	p.Success("Saved to %s", destFile)

	ver := manifest.Version
	if ver == "" {
		ver = "—"
	}
	desc := manifest.Description
	if desc == "" {
		desc = "—"
	}

	p.Blank()
	p.Box("Installed Skill", [][2]string{
		{"name", manifest.Name},
		{"version", ver},
		{"description", desc},
		{"location", destFile},
	})
	p.Blank()
}

// confirmYesNo writes prompt to stderr and reads a single line from in.
// Returns true only for "y" or "yes" (case-insensitive). Any other input,
// EOF, or error returns false (safe default — do not perform the action).
func confirmYesNo(prompt string, in io.Reader) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

// resolveLatestTag queries GitHub's tags API and returns the highest
// semver-valid tag (e.g. "v1.2.3"). Returns "" on any failure — callers
// should fall back to the existing main → master → HEAD chain.
func resolveLatestTag(owner, repo string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=100", githubAPIBase, owner, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pookie-install")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return ""
	}
	var best string
	for _, t := range tags {
		v := t.Name
		if !strings.HasPrefix(v, "v") {
			v = "v" + v
		}
		if !semver.IsValid(v) {
			continue
		}
		if best == "" || semver.Compare(v, best) > 0 {
			best = v
		}
	}
	return best
}

// parseRepoArg parses owner/repo[@ref][/subpath] from a GitHub-style path.
// Accepts optional github.com/ or https://github.com/ prefixes.
func parseRepoArg(arg string) (owner, repo, ref, subpath string) {
	arg = strings.TrimPrefix(arg, "https://github.com/")
	arg = strings.TrimPrefix(arg, "http://github.com/")
	arg = strings.TrimPrefix(arg, "github.com/")

	// Split off ref (owner/repo@ref or owner/repo/subpath@ref).
	if idx := strings.LastIndex(arg, "@"); idx >= 0 {
		ref = arg[idx+1:]
		arg = arg[:idx]
	}

	parts := strings.SplitN(arg, "/", 3)
	if len(parts) < 2 {
		return "", "", "", ""
	}
	owner = parts[0]
	repo = parts[1]
	if len(parts) == 3 {
		subpath = parts[2]
	}
	return
}

// fetchSkillMD tries to download SKILL.md via the GitHub raw-content CDN.
// It tries the supplied ref first; if empty it falls through main → master → HEAD.
func fetchSkillMD(owner, repo, ref, skillPath string) ([]byte, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	candidates := []string{ref}
	if ref == "" {
		candidates = []string{"main", "master", "HEAD"}
	}

	for _, r := range candidates {
		rawURL := "https://raw.githubusercontent.com/" +
			owner + "/" + repo + "/" + r + "/" + skillPath

		resp, err := client.Get(rawURL)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, "", fmt.Errorf("read response body: %w", readErr)
		}
		if resp.StatusCode == http.StatusOK {
			return body, r, nil
		}
	}

	return nil, "", fmt.Errorf("SKILL.md not found in %s/%s (tried: %s)", owner, repo, strings.Join(candidates, ", "))
}

// validateInstallManifest checks a parsed skill manifest against the
// PookiePaws security sandbox rules before writing it to disk.
func validateInstallManifest(m skills.Manifest) error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("manifest is missing a name field")
	}

	// Prevent path-traversal attacks in the skill name used as a directory.
	clean := filepath.Clean(m.Name)
	if clean != m.Name || strings.ContainsAny(m.Name, `/\`) || strings.Contains(m.Name, "..") {
		return fmt.Errorf("skill name %q is unsafe for filesystem storage", m.Name)
	}

	// Block skills that declare shell/exec tools — these bypass ExecGuard.
	blocked := []string{"shell", "exec", "bash", "powershell", "cmd.exe", "system", "subprocess"}
	for _, tool := range m.Tools {
		lower := strings.ToLower(tool)
		for _, b := range blocked {
			if strings.Contains(lower, b) {
				return fmt.Errorf("skill declares a blocked tool %q", tool)
			}
		}
	}

	// Sanity-check timeout: reject anything over 4 hours.
	if m.Timeout > 4*time.Hour {
		return fmt.Errorf("skill timeout %s exceeds the maximum allowed value (4h)", m.Timeout)
	}

	return nil
}
