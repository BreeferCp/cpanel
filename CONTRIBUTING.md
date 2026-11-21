# Contributing

Thanks for helping. A few ground rules keep a root-level hosting panel safe to run.

## Setup

```bash
go test ./...                                          # backend
cd web && npm ci && npm run typecheck && npm run dev   # frontend (proxies /api to :2087)
```

Run the daemon with `BREEFER_DRY_RUN=true` (see the README) so nothing touches your machine.

## Standards

- **Validate, never escape.** Anything that reaches a template, config file, SQL identifier or command line goes through an allow-list validator in `internal/system/validate.go` first. Add a test that feeds it a hostile value.
- **Tenancy goes through `loadAccount`.** New account-scoped queries must filter on `account_id`; add an integration test in `internal/api/api_test.go` proving another customer gets a 404.
- **No shell.** Use `system.Runner`; pass secrets on stdin.
- **Errors:** return typed API errors for user mistakes; let unexpected errors reach the wrapper (it logs and returns an opaque 500). Never put internal detail in a response.
- **Provisioners are dry-run testable.** Render config in a pure function and test the output; keep host side effects in thin wrappers.
- **UI:** reuse the primitives in `web/src/components/ui`; use design tokens, not raw colours; every icon button needs an `aria-label`; keep both themes working.
- `gofmt`, `go vet` and `npm run typecheck` must pass. Add or update tests with every behaviour change.

## Pull requests

Describe the change and how you verified it. Anything that changes the security model should say so explicitly and update `docs/security.md`.

## Credits

Created and maintained by Shine Akhanda ([shineakhanda.dev](https://shineakhanda.dev)). By contributing you agree that your work is released under the project's AGPL-3.0-or-later license.
