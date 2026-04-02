package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

//go:embed defaults/*/SKILL.md
var defaultSkillFS embed.FS

type Registry struct {
	skills map[string]engine.Skill
}

var _ engine.SkillRegistry = (*Registry)(nil)

func NewDefaultRegistry() (*Registry, error) {
	registry := &Registry{skills: map[string]engine.Skill{}}
	manifests, err := loadEmbeddedManifests(defaultSkillFS)
	if err != nil {
		return nil, err
	}

	for _, manifest := range manifests {
		switch manifest.Name {
		case "utm-validator":
			registry.Register(NewUTMValidatorSkill(manifest))
		case "salesmanago-lead-router":
			registry.Register(NewSalesmanagoLeadRouterSkill(manifest))
		case "mitto-sms-drafter":
			registry.Register(NewMittoSMSDrafterSkill(manifest))
		case "whatsapp-message-drafter":
			registry.Register(NewWhatsAppMessageDrafterSkill(manifest))
		case "mitpo-ba-researcher":
			registry.Register(NewBAResearcherSkill(manifest))
		case "mitpo-creative-director":
			registry.Register(NewCreativeDirectorSkill(manifest))
		case "mitpo-seo-auditor":
			registry.Register(NewSEOAuditorSkill(manifest))
		}
	}
	return registry, nil
}

func (r *Registry) Register(skill engine.Skill) {
	r.skills[skill.Definition().Name] = skill
}

func (r *Registry) Get(name string) (engine.Skill, bool) {
	skill, ok := r.skills[name]
	return skill, ok
}

func (r *Registry) List() []engine.SkillDefinition {
	definitions := make([]engine.SkillDefinition, 0, len(r.skills))
	for _, skill := range r.skills {
		definitions = append(definitions, skill.Definition())
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})
	return definitions
}

func loadEmbeddedManifests(filesystem fs.FS) ([]Manifest, error) {
	var manifests []Manifest

	err := fs.WalkDir(filesystem, ".", func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Base(current) != "SKILL.md" {
			return nil
		}

		data, err := fs.ReadFile(filesystem, current)
		if err != nil {
			return err
		}
		manifest, err := ParseSkillMarkdown(string(data))
		if err != nil {
			return fmt.Errorf("parse %s: %w", current, err)
		}
		if strings.TrimSpace(manifest.Name) == "" {
			return fmt.Errorf("skill file %s is missing a name", current)
		}
		manifests = append(manifests, manifest)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return manifests, nil
}
