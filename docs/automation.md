# Content Automation

`pookiepaws automate` turns one campaign request into a local review queue of social content drafts. It generates plans, mock or provider-backed assets, edit plans, per-item review reports, and a batch review queue. It never posts or uploads automatically.

## Quick Run

```powershell
go run ./cmd/pookiepaws automate --file examples/content_batch.yaml
```

This writes a batch under `outputs/content/<batch-id>/`:

- `batch_request.json`: normalized request used for the run
- `review_queue.json`: machine-readable review queue
- `review_queue.md`: human-readable queue with project IDs and report paths
- one folder per generated content item with assets, `edit_plan.json`, `reports/review.md`, and optional `outputs/final.mp4`

## Inline Batch

```powershell
go run ./cmd/pookiepaws automate `
  --product "PookiePaws Paw Balm" `
  --goal "Launch week social ad drafts" `
  --platforms "tiktok,instagram" `
  --variants 4 `
  --provider mock
```

## Rendering

By default, automation creates assets and edit plans but does not render MP4s. Install FFmpeg and pass `--render` when you want video files:

```powershell
go run ./cmd/pookiepaws doctor
go run ./cmd/pookiepaws automate --file examples/content_batch.yaml --render
```

Use `--dry-run` to skip provider asset generation and rendering while still writing plans and reports.

## Approval Boundary

Generated drafts are always marked `not_posted` and `requires_manual_approval`. Browser automation remains dry-run/scaffolded in the MVP, so uploading or posting must be done manually or through a future explicit confirmation step.
