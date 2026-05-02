package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadRequest(path string) (BatchRequest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return BatchRequest{}, err
	}
	var req BatchRequest
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(b, &req)
	default:
		err = yaml.Unmarshal(b, &req)
	}
	if err != nil {
		return BatchRequest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := req.Normalize(); err != nil {
		return BatchRequest{}, err
	}
	return req, nil
}

func (r *BatchRequest) Normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Product = strings.TrimSpace(r.Product)
	r.Goal = strings.TrimSpace(r.Goal)
	r.Style = strings.TrimSpace(r.Style)
	r.Provider = strings.TrimSpace(r.Provider)
	if r.Name == "" {
		r.Name = "content-batch"
	}
	if r.DurationSec <= 0 {
		r.DurationSec = 15
	}
	if r.Variants <= 0 {
		r.Variants = 3
	}
	if r.Variants > 20 {
		return errors.New("variants must be 20 or fewer")
	}
	if len(r.Platforms) == 0 {
		r.Platforms = []string{"tiktok"}
	}
	if r.Product == "" && len(r.Items) == 0 {
		return errors.New("content batch requires product or explicit items")
	}
	if r.Style == "" {
		r.Style = "cute high-energy motion graphics"
	}
	if len(r.Items) == 0 {
		r.Items = defaultItems(*r)
	}
	for i := range r.Items {
		item := &r.Items[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Platform = firstNonEmpty(item.Platform, r.Platforms[0])
		item.Product = firstNonEmpty(item.Product, r.Product)
		item.Style = firstNonEmpty(item.Style, r.Style)
		if item.DurationSec <= 0 {
			item.DurationSec = r.DurationSec
		}
		if item.ID == "" {
			item.ID = fmt.Sprintf("item-%02d", i+1)
		}
		if item.Product == "" {
			return fmt.Errorf("item %s requires product", item.ID)
		}
	}
	return nil
}

func defaultItems(req BatchRequest) []ContentItem {
	angles := []string{
		"thumb-stopping hook and product introduction",
		"problem to solution transformation",
		"benefit proof and CTA",
		"objection handling with friendly reassurance",
		"limited offer reminder without fake urgency",
	}
	var items []ContentItem
	for i := 0; i < req.Variants; i++ {
		platform := req.Platforms[i%len(req.Platforms)]
		angle := angles[i%len(angles)]
		items = append(items, ContentItem{
			ID:          fmt.Sprintf("variant-%02d", i+1),
			Platform:    platform,
			Product:     req.Product,
			Angle:       angle,
			Style:       req.Style,
			DurationSec: req.DurationSec,
		})
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
