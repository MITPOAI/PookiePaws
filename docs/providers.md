# Generation Providers

Provider implementations share the `internal/providers.Provider` interface:

- `GenerateImage(prompt, options)`
- `GenerateVideo(prompt, imageInput, options)`
- `UpscaleImage(asset)`
- `UpscaleVideo(asset)`
- `RemoveBackground(asset)`
- `CaptionAsset(asset)`
- `GetTaskStatus(taskID)`
- `DownloadResult(taskID)`

## Mock Provider

`--provider mock` is the default. It writes deterministic PNG placeholder assets and JSON metadata without API calls or cost.

```powershell
go run ./cmd/pookiepaws providers test --provider mock
go run ./cmd/pookiepaws generate-image --provider mock --prompt "cute product hero shot"
```

## fal.ai

The fal.ai adapter reads `FAL_KEY`. Network generation is intentionally scaffolded in the MVP so the local flow can stabilize first.

```powershell
$env:FAL_KEY = "..."
go run ./cmd/pookiepaws generate-image --provider fal --prompt "product shot"
```

## Runware

The Runware adapter reads `RUNWARE_API_KEY`. It is scaffolded behind the same interface and should map concrete model endpoints, request IDs, polling, and download handling in the next milestone.

```powershell
$env:RUNWARE_API_KEY = "..."
go run ./cmd/pookiepaws generate-image --provider runware --prompt "product shot"
```

## Environment

Copy `.env.example` into your shell or local environment manager:

- `RUNWARE_API_KEY`
- `FAL_KEY`
- `POOKIEPAWS_HOME`
- `PYTHON`

Run `pookiepaws doctor` to verify the local source build tools, Python, FFmpeg, memory path, and provider env vars before trying a real render.
