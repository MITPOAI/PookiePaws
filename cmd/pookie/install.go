package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/skills"
)

// cmdInstall downloads a SKILL.md from a public GitHub repository, validates
// it against the security sandbox rules, and saves it into the workspace
// skills directory.
//
// Supported argument formats:
//
//	owner/repo              tries main → master → HEAD
//	owner/repo@ref          uses the specified branch or tag
//	owner/repo/subpath      fetches from a subdirectory
//	owner/repo/subpath@ref  subpath at a specific ref
func cmdInstall(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pookie install <owner/repo[@ref]>")
		os.Exit(1)
	}

	repoArg := args[0]

	fs := flag.NewFlagSet("install", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	_ = fs.Parse(args[1:])

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
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		p.Error("Create skill directory: %v", err)
		os.Exit(1)
	}

	destFile := filepath.Join(destDir, "SKILL.md")
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
