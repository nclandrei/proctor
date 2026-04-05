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

Tagged GitHub releases publish prebuilt Homebrew archives from
`nclandrei/proctor` and refresh `nclandrei/homebrew-tap/Formula/proctor.rb`
automatically.
The release workflow expects a `HOMEBREW_TAP_TOKEN` GitHub secret with push
access to `nclandrei/homebrew-tap`.

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

Reading `proctor --help` is not the task. It is the entry point. The agent is
still expected to inspect the current diff, identify the user-visible change,
create the right contract, run the manual checks, and record real evidence.

## Quick Start

### 1. Create the Contract

Before writing the contract, inspect the current repo diff and choose the
actual user-visible change under test. Do not replace the changed feature with
a generic smoke test just because it is faster to validate.

For user-visible web work, start here:

```bash
proctor start \
  --platform web \
  --feature "new authentication flow" \
  --url http://127.0.0.1:3000/login \
  --curl scenario \
  --curl-endpoint "happy-path=POST /api/login" \
  --curl-endpoint "failure-path=POST /api/login" \
  --curl-endpoint "Already signed-in users are redirected away from /login=GET /api/session" \
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

`curl` is decided per scenario. `--curl scenario` is the explicit risk-based mode, and each `--curl-endpoint` entry binds one or more endpoints to a named scenario. `--curl required` remains as a shorthand for requiring curl on both the happy path and failure path. `proctor done` enforces that recorded curl evidence produces a real HTTP response and that the wrapped command matches one of the scenario's declared endpoints.

If the flow is mostly client-side and there is no meaningful backend or protocol risk, skip `curl` with an explicit reason:

```bash
proctor start \
  --platform web \
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

For iOS work, create an iOS contract instead:

```bash
proctor start \
  --platform ios \
  --feature "reader library relaunch" \
  --ios-scheme Pagena \
  --ios-bundle-id com.example.pagena \
  --ios-simulator "iPhone 16 Pro" \
  --curl skip \
  --curl-skip-reason "UI-only iOS verification for this pass." \
  --happy-path "Launching the app lands on the library screen." \
  --failure-path "Missing content shows a visible recovery state instead of a blank screen." \
  --edge-case "validation and malformed input=N/A: no freeform input in this flow" \
  --edge-case "empty or missing input=N/A: no required input in this flow" \
  --edge-case "retry or double-submit=N/A: no repeated mutation in this flow" \
  --edge-case "loading, latency, and race conditions=Loading placeholder settles once without duplicate content." \
  --edge-case "network or server failure=Offline launch shows a recoverable empty state." \
  --edge-case "auth and session state=N/A: anonymous browsing only" \
  --edge-case "app lifecycle, relaunch, and state persistence=Foregrounding the app keeps the same selected title." \
  --edge-case "device traits, orientation, and layout=Library remains readable on the target simulator." \
  --edge-case "accessibility, dynamic type, and keyboard behavior=N/A: this pass is visual only" \
  --edge-case "any feature-specific risks=N/A: no extra feature-specific risks"
```

For CLI and TUI work, create a CLI contract instead:

```bash
proctor start \
  --platform cli \
  --feature "magellan prompt inspection flow" \
  --cli-command "magellan prompts inspect onboarding" \
  --happy-path "Inspecting a known prompt shows the body and metadata in a readable terminal layout." \
  --failure-path "Inspecting an unknown prompt exits non-zero and prints a clear error." \
  --edge-case "invalid or malformed input=Broken prompt syntax shows a validation error without a panic" \
  --edge-case "missing required args, files, config, or env=Missing prompt slug explains what argument is required" \
  --edge-case "retry, rerun, and idempotency=Running the same inspect command twice gives the same result" \
  --edge-case "long-running output, streaming, or progress state=N/A: single-shot command with immediate output" \
  --edge-case "interrupts, cancellation, and signals=N/A: command exits immediately" \
  --edge-case "tty, pipe, and non-interactive behavior=Piped output still renders the inspected prompt body without ANSI garbage" \
  --edge-case "terminal layout, wrapping, and resize behavior=The inspected prompt still wraps cleanly in a narrow terminal" \
  --edge-case "keyboard navigation and shortcut behavior=N/A: single-shot command with no in-app key handling" \
  --edge-case "state, config, and persistence across reruns=N/A: read-only inspection command" \
  --edge-case "stderr, exit codes, and partial failure reporting=Unknown prompt returns a non-zero exit code and prints the error on stderr" \
  --edge-case "any feature-specific risks=N/A: no extra feature-specific risks"
```

For desktop app work, create a desktop contract instead:

```bash
proctor start \
  --platform desktop \
  --feature "Firefox bookmark manager" \
  --app-name "Firefox" \
  --app-bundle-id "org.mozilla.firefox" \
  --curl skip \
  --curl-skip-reason "UI-only desktop verification" \
  --happy-path "Bookmark manager opens and lists saved bookmarks." \
  --failure-path "Empty bookmark list shows a helpful prompt." \
  --edge-case "validation and malformed input=N/A: no freeform input in this flow" \
  --edge-case "empty or missing input=N/A: no required input in this flow" \
  --edge-case "retry or double-submit=N/A: no repeated mutation in this flow" \
  --edge-case "loading, latency, and race conditions=N/A: instant local operation" \
  --edge-case "network or server failure=N/A: no backend dependency in this flow" \
  --edge-case "auth and session state=N/A: no authentication required" \
  --edge-case "window management, resize, and multi-monitor=Bookmark manager resizes cleanly" \
  --edge-case "drag-drop, clipboard, and system integration=N/A: no drag-drop in this flow" \
  --edge-case "keyboard shortcuts and accessibility=N/A: this pass is visual only" \
  --edge-case "any feature-specific risks=N/A: no additional risks"
```

### 2. Capture Real Evidence

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

For CLI and TUI work, Proctor expects a real terminal session.
Preferred, not required: use a real terminal app plus `tmux` or an equivalent
persistent multiplexer so the agent can keep one session alive, drive keyboard
input deterministically, capture pane output, and take screenshots.

- run the CLI in a real terminal session
- capture at least one screenshot
- capture the terminal transcript from that session
- record the actual command you exercised

## Known-Good Capture Workflows

Proctor does not own capture. The agent should discover or choose a workflow
that can emit the required artifacts, then attach them with `proctor record`.

Known-good defaults:

- Web: if `agent-browser` is available in the agent environment, it is a good default for driving the browser, capturing desktop and mobile screenshots, and producing the small `report.json` Proctor expects.
- iOS: if `xcrun` is available, `xcrun simctl` is a good default for booting a simulator, taking screenshots, and inspecting logs before you write `ios-report.json`.
- Desktop: if `peekaboo` is available, it uses ScreenCaptureKit for background window capture without stealing focus. Fallback: `screencapture -x -l <windowID>` plus `osascript` for window metadata before you write `desktop-report.json`.
- CLI/TUI: a real terminal plus `tmux` is a good default for keeping the session alive, capturing the transcript, and taking a screenshot before `proctor record cli`.
- HTTP: `curl -si` is a good default when a scenario carries direct HTTP risk.

These are recommendations only. Proctor stays tool-agnostic and accepts any
workflow that produces the required artifacts and assertions.

At runtime, `proctor start`, `proctor status`, and `proctor done` also print
local recommendations based on tools found on `PATH`.

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

### 4. Capture And Attach Real iOS Evidence

Proctor does not boot the simulator for you. Use your own simulator tooling to
build, launch, screenshot, and inspect logs. Proctor only needs a screenshot
plus a small `ios-report.json` file:

```json
{
  "simulator": {
    "name": "iPhone 16 Pro",
    "runtime": "iOS 18.2"
  },
  "app": {
    "bundleId": "com.example.pagena",
    "screen": "Library",
    "state": "foreground"
  },
  "issues": {
    "launchErrors": 0,
    "crashes": 0,
    "fatalLogs": 0
  }
}
```

Then record that evidence against the scenario:

```bash
proctor record ios \
  --scenario happy-path \
  --session pagena-library-1 \
  --report /abs/path/ios-report.json \
  --screenshot library=/abs/path/library.png \
  --assert 'screen contains Library' \
  --assert 'bundle_id = com.example.pagena' \
  --assert 'app_launch = true'
```

One simulator report can be reused for multiple scenarios if it genuinely
proves each one.

### 5. Attach Real Desktop Evidence

Proctor does not launch the app for you. Use your own window capture tooling to
screenshot and inspect the running app. Proctor only needs a screenshot plus a
small `desktop-report.json` file:

```json
{
  "app": {
    "name": "Firefox",
    "bundleId": "org.mozilla.firefox",
    "state": "running",
    "windowTitle": "Bookmark Manager"
  },
  "issues": {
    "crashes": 0,
    "fatalLogs": 0
  }
}
```

Then record that evidence against the scenario:

```bash
proctor record desktop \
  --scenario happy-path \
  --session firefox-desktop-1 \
  --report /abs/path/desktop-report.json \
  --screenshot window=/abs/path/window.png \
  --assert 'app_name contains Firefox' \
  --assert 'crashes = 0' \
  --assert 'screenshot = true'
```

One desktop report can be reused for multiple scenarios if it genuinely
proves each one.

### 6. Attach Real CLI Evidence

Then record the terminal evidence against the scenario:

```bash
proctor record cli \
  --scenario happy-path \
  --session magellan-cli-1 \
  --command "magellan prompts inspect onboarding" \
  --transcript /abs/path/pane.txt \
  --screenshot terminal=/abs/path/terminal.png \
  --exit-code 0 \
  --assert 'output contains onboarding' \
  --assert 'exit_code = 0' \
  --assert 'screenshot = true'
```

### 7. Attach HTTP Evidence When Required

When a scenario requires `curl`, wrap the real HTTP command that hits one of the scenario's declared `--curl-endpoint` contracts:

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

### 8. Check Coverage And Finish

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

For iOS evidence, Proctor expects:

- a simulator session id string
- at least one simulator screenshot across the run
- an `ios-report.json` artifact
- at least one passing assertion

The iOS report can be synthesized from real simulator-session output. It does
not have to come from one specific helper.

For desktop evidence, Proctor expects:

- a desktop session id string
- at least one window screenshot across the run
- a `desktop-report.json` artifact with app metadata and issue counts
- at least one passing assertion

The desktop report can be synthesized from real window-capture output. It does
not have to come from one specific helper.

For CLI evidence, Proctor expects:

- a terminal session id string
- at least one terminal screenshot across the run
- a transcript artifact from that session
- the actual exercised command
- at least one passing assertion

For curl evidence, Proctor expects:

- a real wrapped HTTP command
- a parsed HTTP response
- a wrapped request whose method and path match one of the scenario's declared endpoints
- the captured transcript
- at least one passing assertion

Provenance alone is not enough. Evidence must also include scenario-specific assertions.

`curl` is gated per scenario, not per endpoint. Endpoints are recorded on each scenario so the contract can say which HTTP surfaces carry risk, and `proctor done` enforces those scenario-level HTTP contracts scenario-by-scenario.

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

## iOS Assertions

Examples:

- `screen contains Library`
- `bundle_id = com.example.pagena`
- `simulator contains iPhone 16 Pro`
- `runtime contains iOS`
- `state = foreground`
- `app_launch = true`
- `launch_errors = 0`
- `crashes = 0`
- `fatal_logs = 0`
- `screenshot = true`

If you do not explicitly assert iOS health counts, Proctor adds implicit
zero-issue assertions for:

- launch errors
- crashes
- fatal logs

## Desktop Assertions

Examples:

- `app_name contains Firefox`
- `bundle_id = org.mozilla.firefox`
- `state = running`
- `window_title contains Bookmarks`
- `crashes = 0`
- `fatal_logs = 0`
- `screenshot = true`

If you do not explicitly assert desktop health counts, Proctor adds implicit
zero-issue assertions for:

- crashes
- fatal logs

## CLI Assertions

Examples:

- `output contains onboarding`
- `output contains prompt not found`
- `command contains magellan`
- `session contains cli-session`
- `tool = terminal-session`
- `exit_code = 0`
- `screenshot = true`

## Edge Cases Are First-Class

Proctor does not accept "give me two edge cases".

Each category must be covered either by:

- one or more concrete scenarios
- or `N/A` with a reason

If any category is omitted, or uses bare `N/A` with no reason, `proctor start`
fails instead of silently treating it as uncovered.

Current categories:

Web:
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

iOS:
- validation and malformed input
- empty or missing input
- retry or double-submit
- loading, latency, and race conditions
- network or server failure
- auth and session state
- app lifecycle, relaunch, and state persistence
- device traits, orientation, and layout
- accessibility, dynamic type, and keyboard behavior
- any feature-specific risks

Desktop:
- validation and malformed input
- empty or missing input
- retry or double-submit
- loading, latency, and race conditions
- network or server failure
- auth and session state
- window management, resize, and multi-monitor
- drag-drop, clipboard, and system integration
- keyboard shortcuts and accessibility
- any feature-specific risks

CLI:
- invalid or malformed input
- missing required args, files, config, or env
- retry, rerun, and idempotency
- long-running output, streaming, or progress state
- interrupts, cancellation, and signals
- tty, pipe, and non-interactive behavior
- terminal layout, wrapping, and resize behavior
- keyboard navigation and shortcut behavior
- state, config, and persistence across reruns
- stderr, exit codes, and partial failure reporting
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
- `proctor record cli`
  Attaches terminal evidence to one scenario.
- `proctor record ios`
  Attaches iOS simulator evidence to one scenario.
- `proctor record desktop`
  Attaches desktop app evidence to one scenario.
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
proctor record cli --help
proctor record ios --help
proctor record desktop --help
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
- `captures.jsonl`
- `contract.md`
- `report.html`
- `artifacts/`

`contract.md` and `report.html` are derived from the recorded evidence. They are human-facing outputs, not the source of truth.

Artifact files are append-only. Re-recording the same surface, scenario, and label creates a new uniquely named file under `artifacts/` instead of overwriting an earlier recording.

`report.html` is a plain document with a light theme, keeps its styles, screenshot previews, and embedded log transcripts self-contained, lets readers enlarge screenshots inline, and keeps logs collapsed until the reader expands them.

## Current Scope

Current supported surfaces:

- web browser evidence with desktop and mobile proof
- iOS simulator evidence with screenshots plus simulator/app report metadata
- desktop app evidence with screenshots plus app metadata and health checks
- CLI and TUI evidence with screenshots plus transcripts
- risk-based `curl` evidence when backend or protocol verification matters
- `curl` risk is modeled per scenario, with scenario-level endpoint lists and scenario-level completion gates

## Development

```bash
go test ./...
go run . --help
```

If you are changing the browser reporting or CLI help, rerun a fresh-agent test. The target bar is simple:

- a new agent should start with `proctor --help`
- it should not need to read Proctor's source
- it should be able to create a run, record evidence, and finish with `proctor done`
