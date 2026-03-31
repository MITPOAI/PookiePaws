package skills

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseSkillMarkdownExtendedManifest(t *testing.T) {
	content := `---
name: competitor-pricing-extractor
description: Capture competitor offers and summarize pricing pressure.
category: research
version: 1.0.0
tags: [pricing, competitor, ecommerce]
tools:
  - crawl_pages
  - normalize_currency
events:
  - workflow.submitted
transport: jsonrpc
rpc_method: marketing.pricing.extract
rpc_notifications:
  - marketing.skill.progress
approval_policy: review_before_publish
timeout: 2m
---
Collect live pricing evidence and normalize it into a marketer-friendly summary.

## Inputs
- competitor (string, required): Competitor brand or account name.
- domains (array, required): Domains to crawl for public pricing.

## Outputs
- summary (string, required): Short pricing narrative for the operator.
- observations (array, required): Normalized pricing snapshots.

## RPC Notifications
- marketing.skill.progress
`

	manifest, err := ParseSkillMarkdown(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if manifest.Name != "competitor-pricing-extractor" {
		t.Fatalf("unexpected name %q", manifest.Name)
	}
	if manifest.Category != "research" {
		t.Fatalf("unexpected category %q", manifest.Category)
	}
	if manifest.Version != "1.0.0" {
		t.Fatalf("unexpected version %q", manifest.Version)
	}
	if manifest.RPC.Method != MethodCompetitorPricingExtract {
		t.Fatalf("unexpected rpc method %q", manifest.RPC.Method)
	}
	if manifest.Timeout != 2*time.Minute {
		t.Fatalf("unexpected timeout %s", manifest.Timeout)
	}
	if len(manifest.InputFields) != 2 {
		t.Fatalf("expected 2 input fields, got %d", len(manifest.InputFields))
	}
	if !manifest.InputFields[0].Required || manifest.InputFields[0].Type != "string" {
		t.Fatalf("unexpected first input field %+v", manifest.InputFields[0])
	}
	if len(manifest.OutputFields) != 2 {
		t.Fatalf("expected 2 output fields, got %d", len(manifest.OutputFields))
	}
	if len(manifest.RPC.Notifications) != 2 {
		t.Fatalf("expected merged notifications, got %d", len(manifest.RPC.Notifications))
	}
	if manifest.Prompt != "Collect live pricing evidence and normalize it into a marketer-friendly summary." {
		t.Fatalf("unexpected prompt %q", manifest.Prompt)
	}
}

func TestMarketingJSONRPCPayloads(t *testing.T) {
	cases := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name: "market trends",
			value: NewMarketTrendsRequest("req-1", MarketTrendsParams{
				Brand:            "PookiePaws",
				Regions:          []string{"AU", "US"},
				Channels:         []string{"seo", "email"},
				Topics:           []string{"pet wellness", "gift bundles"},
				Competitors:      []string{"OpenClaw"},
				LookbackDays:     14,
				MaxSources:       12,
				IncludeSentiment: true,
			}),
			expected: `{"jsonrpc":"2.0","id":"req-1","method":"marketing.trends.refresh","params":{"brand":"PookiePaws","regions":["AU","US"],"channels":["seo","email"],"topics":["pet wellness","gift bundles"],"competitors":["OpenClaw"],"lookback_days":14,"max_sources":12,"include_sentiment":true}}`,
		},
		{
			name: "seo audit",
			value: NewSEOAuditRequest("req-2", SEOAuditParams{
				URL:              "https://pookiepaws.example",
				SitemapURL:       "https://pookiepaws.example/sitemap.xml",
				FocusKeywords:    []string{"pet gifting", "pet wellness"},
				Locale:           "en-AU",
				Device:           "mobile",
				CrawlLimit:       50,
				CheckCompetitors: []string{"https://openclaw.example"},
			}),
			expected: `{"jsonrpc":"2.0","id":"req-2","method":"marketing.seo.audit","params":{"url":"https://pookiepaws.example","sitemap_url":"https://pookiepaws.example/sitemap.xml","focus_keywords":["pet gifting","pet wellness"],"locale":"en-AU","device":"mobile","crawl_limit":50,"check_competitors":["https://openclaw.example"]}}`,
		},
		{
			name: "competitor pricing",
			value: NewCompetitorPricingRequest("req-3", CompetitorPricingParams{
				Competitor:        "OpenClaw",
				Domains:           []string{"https://openclaw.example"},
				Products:          []string{"starter box", "vip bundle"},
				Regions:           []string{"AU"},
				Currency:          "AUD",
				MaxPages:          25,
				CapturePromotions: true,
			}),
			expected: `{"jsonrpc":"2.0","id":"req-3","method":"marketing.pricing.extract","params":{"competitor":"OpenClaw","domains":["https://openclaw.example"],"products":["starter box","vip bundle"],"regions":["AU"],"currency":"AUD","max_pages":25,"capture_promotions":true}}`,
		},
		{
			name: "cross-channel campaign draft",
			value: NewCrossChannelCampaignDraftRequest("req-4", CrossChannelCampaignDraftParams{
				Brief:     "Counter a competitor price drop with a premium-value narrative.",
				Objective: "Retain VIP subscribers",
				AudienceSegments: []AudienceSegment{
					{Name: "VIP subscribers", NeedState: "value reassurance", Offer: "bundle bonus"},
				},
				Channels:   []string{"email", "sms", "landing_page"},
				BrandVoice: "warm, premium, reassuring",
				Tone:       "empathetic",
				Offer:      "limited bundle bonus",
				Locales:    []string{"en-AU"},
				Constraints: CampaignConstraints{
					CharacterLimit: 140,
					BlockedClaims:  []string{"guaranteed savings"},
					RequiredCTA:    "Shop the VIP bundle",
				},
				IncludeApprovalStep: true,
			}),
			expected: `{"jsonrpc":"2.0","id":"req-4","method":"marketing.campaign.draft","params":{"brief":"Counter a competitor price drop with a premium-value narrative.","objective":"Retain VIP subscribers","audience_segments":[{"name":"VIP subscribers","need_state":"value reassurance","offer":"bundle bonus"}],"channels":["email","sms","landing_page"],"brand_voice":"warm, premium, reassuring","tone":"empathetic","offer":"limited bundle bonus","locales":["en-AU"],"constraints":{"character_limit":140,"blocked_claims":["guaranteed savings"],"required_cta":"Shop the VIP bundle"},"include_approval_step":true}}`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := json.Marshal(testCase.value)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			if string(data) != testCase.expected {
				t.Fatalf("unexpected payload\nwant: %s\ngot:  %s", testCase.expected, string(data))
			}
		})
	}
}
