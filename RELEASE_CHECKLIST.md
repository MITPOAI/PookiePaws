# Release Checklist

## Brain and provider validation

- Verify no stale preset model IDs remain in `internal/cli/providers.go`
- Run `pookie init` once from a clean home directory
- Run `pookie doctor --brain`
- Confirm these cases are covered:
  - dead endpoint
  - bad API key
  - valid endpoint with invalid model
  - valid endpoint with valid model

## Smoke coverage

- Run `pookie smoke --cli`
- Run `pookie smoke --api`
- Run `pookie smoke --provider` with one real provider before tagging a release
- Confirm `pookie smoke --json` output is usable in issue reports

## Operator workflows

- Confirm `pookie chat` fails fast on invalid provider/model config
- Confirm `pookie sessions --trace` shows technical failure details
- Confirm `pookie approvals`, `pookie audit`, and `pookie doctor` still work from a clean runtime
