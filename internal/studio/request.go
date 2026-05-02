package studio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadRequest(path string) (CampaignRequest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return CampaignRequest{}, err
	}
	var req CampaignRequest
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(b, &req)
	default:
		err = yaml.Unmarshal(b, &req)
	}
	if err != nil {
		return CampaignRequest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := req.Normalize(); err != nil {
		return CampaignRequest{}, err
	}
	return req, nil
}

func (r *CampaignRequest) Normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.BrandName = strings.TrimSpace(r.BrandName)
	r.Product = strings.TrimSpace(r.Product)
	r.Niche = strings.TrimSpace(r.Niche)
	r.Goal = strings.TrimSpace(r.Goal)
	r.Offer = strings.TrimSpace(r.Offer)
	r.TargetAudience = strings.TrimSpace(r.TargetAudience)
	r.Style = strings.TrimSpace(r.Style)
	r.Provider = strings.TrimSpace(r.Provider)
	if r.Name == "" {
		r.Name = "studio-campaign"
	}
	if r.Product == "" {
		return errors.New("studio campaign requires product")
	}
	if r.Goal == "" {
		r.Goal = "Generate campaign-ready social content for manual review."
	}
	if r.Style == "" {
		r.Style = "cute high-energy motion graphics with bold captions"
	}
	if len(r.Platforms) == 0 {
		r.Platforms = []string{"tiktok", "instagram"}
	}
	if r.ContentVariants <= 0 {
		r.ContentVariants = 4
	}
	if r.ContentVariants > 20 {
		return errors.New("content_variants must be 20 or fewer")
	}
	if r.DurationSec <= 0 {
		r.DurationSec = 15
	}
	return nil
}
