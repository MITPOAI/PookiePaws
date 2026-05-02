# Local Marketing Studio

`pookiepaws studio campaign` creates a local campaign workspace for research, ad planning, content generation, and manual launch preparation.

It does not buy media, post ads, bypass platform controls, or hide browser automation. Paid ads must be launched manually in accounts you own or have permission to use.

## Run A Studio Campaign

```powershell
go run ./cmd/pookiepaws studio campaign --file examples/studio_campaign.yaml --provider mock
```

Output is written under `outputs/studio/<campaign-id>/`:

- `research/research_brief.md`
- `strategy/campaign_strategy.md`
- `ads/ad_platform_export.json`
- `ads/launch_checklist.md`
- `browser/manual_upload_workflow.yaml`
- `content/<batch-id>/review_queue.md`
- content item folders with generated assets, edit plans, and per-item review reports

## Inline Campaign

```powershell
go run ./cmd/pookiepaws studio campaign `
  --brand-name "PookiePaws" `
  --product "PookiePaws Paw Balm" `
  --niche "pet care" `
  --goal "Launch social campaign" `
  --offer "Shop the launch bundle" `
  --target-audience "Pet owners who want simple care routines" `
  --platforms "tiktok,instagram" `
  --variants 4 `
  --provider mock
```

## Rendering

Run `pookiepaws doctor` first. If FFmpeg is available on `PATH` or through the `FFMPEG` env var, pass `--render` to produce MP4 drafts:

```powershell
go run ./cmd/pookiepaws studio campaign --file examples/studio_campaign.yaml --provider mock --render
```

## Ad Launch Boundary

`ads/ad_platform_export.json` contains draft ad names, copy, CTA, placeholder destination URLs, and generated asset paths. Treat it as upload input only after human review. The generated browser workflow includes explicit confirmation steps and remains a safe scaffold.
