# Troubleshooting

## Chat fails immediately

Start with:

```powershell
pookie doctor --brain
```

This checks four things in order:

1. Config present
2. Endpoint reachable
3. Credentials accepted
4. Model accepted

If `model accepted` is `false`, fix `llm_model` first. The endpoint and API key may still be valid.

## Common fixes

- Invalid model ID:
  - Run `pookie init`
  - Re-select the provider
  - Confirm the exact saved model ID
- Bad API key:
  - Run `pookie init`
  - Re-enter the provider key
- Dead endpoint or wrong URL:
  - Check `llm_base_url`
  - It must point to an OpenAI-compatible `chat/completions` endpoint

## Operator diagnostics

- `pookie doctor --brain`
  - Focused provider, endpoint, credential, and model validation
- `pookie sessions --trace --id <session_id>`
  - Shows saved prompt traces and technical failure details
- `pookie smoke --provider`
  - Uses the active saved config and runs a minimal completion request
- `pookie smoke --cli`
  - Runs deterministic CLI smoke checks with a mocked provider
- `pookie smoke --api`
  - Runs deterministic API smoke checks with a mocked provider

## Current known gap

- MCP-backed providers can still run, but `doctor --brain` and `smoke --provider` currently validate OpenAI-compatible providers only.
