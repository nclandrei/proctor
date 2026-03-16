# Proctor

Proctor makes a coding agent prove it manually tested its own work before it can say "done".

The point is not browser automation by itself. Agents can already click buttons, run `curl`, and take screenshots. The missing piece is the contract:

- what had to be tested
- what counted as proof
- what blocked completion

Proctor creates that contract, records evidence against it, and refuses completion until the required proof exists.

The CLI is intentionally long-form. A fresh agent should be able to start with `proctor --help`, learn the workflow, and complete a run without reading Proctor's source.

## Install

```bash
brew tap nclandrei/tap
brew install nclandrei/tap/proctor
```

## Who This Is For

Proctor is agent-agnostic. It is meant to work from:

- Codex
- Claude Code
- any other coding agent with shell access

It does not assume one agent runtime, one browser driver, or one editor.

## What Proctor Does

Proctor is not:

- a browser automation framework
- an iOS automation framework
- a hosted QA platform

Proctor is:

- a manual-test contract generator
- an evidence recorder
- a completion gate
- a shareable reporting layer

## The Human Prompt

This is the kind of prompt Proctor is designed for:

```text
We just implemented the new authentication flow.
Use proctor --help to manually test it.
```

That prompt should be enough. The agent should not need extra explanation from the human.

## Quick Start

### 1. Create the Contract

For user-visible web work, start here:

```bash
proctor start \
  --feature "new authentication flow" \
  --url http://127.0.0.1:3000/login \
  --curl required \
  --curl-endpoint "POST /api/login" \
  --happy-path "Valid credentials redirect to the dashboard." \
  --failure-path "Invalid credentials show an error and keep the user on /login." \
  --edge-case "validation and malformed input=Bad email shows inline validation" \
  --edge-case "empty or missing input=Empty email and password show required-field errors" \
  --edge-case "retry or double-submit=Second submit does not create duplicate requests" \
  --edge-case "loading, latency, and race conditions=Button stays disabled while the request is pending" \
  --edge-case "network or server failure=500 response shows a retryable error state" \
  --edge-case "auth and session state=Already signed-in users are redirected away from /login" \
  --edge-case "refresh, back-navigation, and state persistence=Refresh preserves the authenticated state" \
  --edge-case "mobile or responsive behavior=Login form stays usable at mobile width" \
  --edge-case "accessibility and keyboard behavior=Enter submits from the password field; tab order stays correct" \
  --edge-case "any feature-specific risks=N/A: no extra feature-specific risks"
```

If the flow is mostly client-side and there is no meaningful backend or protocol risk, skip `curl` with an explicit reason:

```bash
proctor start \
  --feature "What Did I Just Watch finder" \
  --url http://127.0.0.1:4174/kimarite \
  --curl skip \
  --curl-skip-reason "Static client-side filter UI with no separate backend contract." \
  --happy-path "Selecting plain-language finish and approach clues narrows the library and updates the URL." \
  --failure-path "A user can back out with Not sure yet and return to the broad library without broken state." \
  --edge-case "validation and malformed input=N/A: no freeform input, only preset links" \
  --edge-case "empty or missing input=Starting with no clue selected still shows the broad library and finder." \
  --edge-case "retry or double-submit=N/A: idempotent client-side link navigation only" \
  --edge-case "loading, latency, and race conditions=N/A: static client-side filter state with no async mutation" \
  --edge-case "network or server failure=N/A: no feature-specific backend dependency" \
  --edge-case "auth and session state=N/A: public catalog page" \
  --edge-case "refresh, back-navigation, and state persistence=Direct filtered URL preserves the selected clue state on load." \
  --edge-case "mobile or responsive behavior=Filtered finder state remains readable and usable on mobile." \
  --edge-case "accessibility and keyboard behavior=N/A: this pass is visual only" \
  --edge-case "any feature-specific risks=N/A: reset behavior is covered by the main failure path"
```

### 2. Capture Real Browser Evidence

Proctor does not drive the browser for you. Use your own browser tooling to produce:

- a desktop screenshot
- a mobile screenshot
- a `report.json` file with desktop and mobile final URL and issue counts

Proctor only needs a small report shape:

```json
{
  "desktop": {
    "finalUrl": "http://127.0.0.1:3000/dashboard",
    "issues": {
      "consoleErrors": 0,
      "consoleWarnings": 0,
      "pageErrors": 0,
      "failedRequests": 0,
      "httpErrors": 0
    }
  },
  "mobile": {
    "finalUrl": "http://127.0.0.1:3000/dashboard",
    "issues": {
      "consoleErrors": 0,
      "consoleWarnings": 0,
      "pageErrors": 0,
      "failedRequests": 0,
      "httpErrors": 0
    }
  }
}
```

`consoleWarnings` is part of the browser report schema so the run keeps the full
browser-health picture. By default, though, Proctor only blocks completion on
console errors, page errors, failed requests, and HTTP errors. Add an explicit
assertion such as `console_warnings = 0` when warnings should fail the run too.

If your browser tool does not emit this exact file, that is still fine. Capture
the real browser session data, then write a tiny `report.json` file with this
shape and attach that to Proctor.

### 3. Attach Browser Evidence

Each `record browser` command attaches one browser run to one scenario:

```bash
proctor record browser \
  --scenario happy-path \
  --session auth-browser-1 \
  --report /abs/path/report.json \
  --screenshot desktop=/abs/path/desktop.png \
  --screenshot mobile=/abs/path/mobile.png \
  --assert 'final_url contains /dashboard' \
  --assert 'desktop_screenshot = true' \
  --assert 'mobile_screenshot = true'
```

You can reuse one browser report for multiple scenarios if it genuinely proves each one.

### 4. Attach HTTP Evidence When Required

When `curl` matters, wrap the real command:

```bash
proctor record curl \
  --scenario failure-path \
  --assert 'status = 401' \
  --assert 'body contains invalid' \
  --assert 'header.content-type contains application/json' \
  -- \
  curl -si -X POST http://127.0.0.1:3000/api/login \
    -H 'content-type: application/json' \
    -d '{"email":"demo@example.com","password":"wrong"}'
```

### 5. Check Coverage And Finish

```bash
proctor status
proctor done
proctor report
```

`proctor done` is the real completion gate. If it fails, the run is not complete.

## What Counts As Proof

Freehand notes do not count.

For browser evidence, Proctor expects:

- a session id string
- desktop and mobile screenshots across the run
- a report JSON artifact
- at least one passing assertion

The report JSON can be synthesized from real browser-session output. It does
not have to come from one specific browser helper.

For web runs, mobile proof is mandatory. Even when the primary scenario is
desktop-first, `proctor done` still requires at least one desktop screenshot and
at least one mobile screenshot somewhere in the recorded browser evidence.

For curl evidence, Proctor expects:

- a real wrapped command
- the captured transcript
- at least one passing assertion

Provenance alone is not enough. Evidence must also include scenario-specific assertions.

## Browser Assertions

Examples:

- `final_url contains /dashboard`
- `final_url = http://127.0.0.1:3000/login`
- `console_errors = 0`
- `console_warnings = 0`
- `failed_requests = 0`
- `http_errors = 1`
- `desktop_screenshot = true`
- `mobile_screenshot = true`
- `mobile.final_url contains /login`

If you do not explicitly assert browser health counts, Proctor adds implicit zero-issue assertions for the blocking browser-health metrics:

- console errors
- page errors
- failed requests
- HTTP errors

Console warnings are deliberately excluded from that default gate. Proctor still
records `consoleWarnings` in the report so you can inspect them later or make
them blocking with an explicit assertion such as `console_warnings = 0`.

## Edge Cases Are First-Class

Proctor does not accept "give me two edge cases".

Each category must be covered either by:

- one or more concrete scenarios
- or `N/A` with a reason

Current categories:

- validation and malformed input
- empty or missing input
- retry or double-submit
- loading, latency, and race conditions
- network or server failure
- auth and session state
- refresh, back-navigation, and state persistence
- mobile or responsive behavior
- accessibility and keyboard behavior
- any feature-specific risks

## Commands

- `proctor --help`
  The long-form agent onboarding surface.
- `proctor start`
  Creates the verification contract.
- `proctor status`
  Shows what still passes or fails.
- `proctor record browser`
  Attaches browser evidence to one scenario.
- `proctor record curl`
  Wraps and records one real HTTP command for one scenario.
- `proctor done`
  Fails until the contract is satisfied.
- `proctor report`
  Prints the generated output paths.

Use subcommand help for exact flags:

```bash
proctor start --help
proctor record browser --help
proctor record curl --help
proctor done --help
```

## Outputs

Artifacts live outside the repo by default:

```text
~/.proctor/runs/<repo-slug>/<run-id>/
```

Important files:

- `run.json`
- `evidence.jsonl`
- `contract.md`
- `report.html`
- `artifacts/`

`contract.md` and `report.html` are derived from the recorded evidence. They are human-facing outputs, not the source of truth.

`report.html` is always rendered in dark mode.

## Current Scope

The current implementation is strongest for web:

- browser evidence is fully enforced
- `curl` is supported when backend or protocol risk matters

iOS and CLI are part of the design, but not yet implemented in the same depth.

## Development

```bash
go test ./...
go run . --help
```

If you are changing the browser reporting or CLI help, rerun a fresh-agent test. The target bar is simple:

- a new agent should start with `proctor --help`
- it should not need to read Proctor's source
- it should be able to create a run, record evidence, and finish with `proctor done`
