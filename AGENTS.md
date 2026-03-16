# AGENTS.md

## Repo Focus

This repository builds `proctor`, a Go CLI that enforces manual verification contracts for agent-built web and iOS features.

The current implementation target is:

- shared core
- full web flow
- full iOS simulator flow
- browser evidence
- iOS simulator evidence
- risk-based `curl` evidence

In the current contract model, `curl` risk is decided per scenario. Endpoints are attached to the scenarios that need direct HTTP verification; they are not a separate completion unit by themselves.
## Working Rules

- Keep runtime artifacts out of the repo. Proctor writes runs under `~/.proctor` by default.
- Treat browser evidence as higher-trust than imported notes. Browser records need a session id, screenshots, a report artifact, and assertions.
- Treat iOS evidence the same way. iOS records need a simulator session id, screenshots, a report artifact, and assertions.
- Reports are derived output, not source-of-truth evidence.
- Edge cases are first-class scenarios. Do not collapse them into generic notes.
- Prefer the Go standard library unless an external dependency clearly improves the core product.

## Code Layout Expectations

- Keep the CLI thin.
- Keep run storage, evidence validation, and report generation in reusable package code.
- Preserve clean extension points for future CLI and additional surfaces.

## Verification

Before landing changes:

- run `gofmt -w` on modified Go files
- run `go test ./...`

If behavior changes materially, update `README.md` so the product plan and the scaffold do not drift.
