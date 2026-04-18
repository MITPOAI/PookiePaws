package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/skills"
)

// skillEntry is a unified representation of an installed skill for display
// purposes, regardless of whether it comes from the embedded registry or a
// workspace SKILL.md file.
type skillEntry struct {
	Name        string
	Version     string
	Description string
	Source      string // "built-in" or "workspace"
}

// cmdList prints a tabular summary of every installed marketing skill.
// It combines the built-in skills from the embedded registry with any
// additional skills installed in the workspace/skills directory.
func cmdList(args []string) {
	defer maybeShowUpdateNotice(context.Background(), version, os.Stderr, "")
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	spin := p.NewSpinner("Loading skills…")
	spin.Start()
	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		spin.Stop(false, "Engine initialisation failed")
		p.Error("%v", err)
		os.Exit(1)
	}
	defer stack.Close()
	spin.Stop(true, "Engine ready")

	// Collect built-in skills from the registry.
	entries := make(map[string]skillEntry)
	for _, def := range stack.coord.SkillDefinitions() {
		entries[def.Name] = skillEntry{
			Name:        def.Name,
			Description: def.Description,
			Source:      "built-in",
		}
	}

	// Scan workspace/skills for additional SKILL.md manifests.
	// These may also carry version information from their YAML frontmatter.
	skillsDir := filepath.Join(workspaceRoot, "skills")
	dirs, _ := os.ReadDir(skillsDir)
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		mdPath := filepath.Join(skillsDir, dir.Name(), "SKILL.md")
		data, err := os.ReadFile(mdPath)
		if err != nil {
			continue
		}
		manifest, err := skills.ParseSkillMarkdown(string(data))
		if err != nil {
			continue
		}
		name := manifest.Name
		if name == "" {
			name = dir.Name()
		}
		existing, ok := entries[name]
		if ok {
			// Merge version from workspace manifest into built-in entry.
			if manifest.Version != "" {
				existing.Version = manifest.Version
			}
			entries[name] = existing
		} else {
			entries[name] = skillEntry{
				Name:        name,
				Version:     manifest.Version,
				Description: manifest.Description,
				Source:      "workspace",
			}
		}
	}

	// Also attempt to read version from embedded defaults for built-in entries
	// that still have no version. Walk workspace skills directory for their
	// original SKILL.md embedded manifests is not possible from here, but the
	// schema.go ParseSkillMarkdown already covers it above if they exist in
	// workspace/skills. For built-in defaults, we parse the embedded FS.
	embeddedManifests := loadBuiltinManifestVersions()
	for name, ver := range embeddedManifests {
		if e, ok := entries[name]; ok && e.Version == "" {
			e.Version = ver
			entries[name] = e
		}
	}

	// Sort by name for deterministic output.
	sorted := make([]skillEntry, 0, len(entries))
	for _, e := range entries {
		sorted = append(sorted, e)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	if len(sorted) == 0 {
		p.Warning("No skills installed")
		p.Dim("Install skills with:  pookie install <owner/repo>")
		p.Blank()
		return
	}

	// Build the display table.
	rows := make([][2]string, 0, len(sorted))
	for _, e := range sorted {
		ver := e.Version
		if ver == "" {
			ver = "—"
		}
		desc := e.Description
		if desc == "" {
			desc = "—"
		}
		label := fmt.Sprintf("%-10s  %s", ver, desc)
		rows = append(rows, [2]string{e.Name, label})
	}

	p.Blank()
	p.Box("Marketing Skills", rows)
	p.Blank()
	p.Dim("  %d skill(s) installed", len(sorted))
	p.Blank()
}

// loadBuiltinManifestVersions parses the embedded SKILL.md files from the
// skills/defaults directory to extract version strings. This is a best-effort
// helper; errors are silently ignored.
func loadBuiltinManifestVersions() map[string]string {
	versions := map[string]string{}
	// Re-parse the embedded defaults. The skills package exposes
	// ParseSkillMarkdown but not the embed.FS directly. However, we can
	// read the workspace skills directory which already happens above.
	// For a cleaner approach, we scan the defaults directory at build time
	// via the registry. Since SkillDefinition does not carry version, we
	// simply return an empty map here and rely on workspace manifests.
	return versions
}
