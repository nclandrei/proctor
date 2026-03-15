# Proctor

Proctor is an enforcement layer for agentic manual testing.

The problem is not that agents cannot click buttons, run `curl`, or take screenshots. They can. The problem is that nothing forces them to think through the risky cases, gather real proof, and block themselves from declaring "done" before that proof exists.

Showboat is the closest reference in spirit: it captures proof of work and discourages hand-wavy reporting. Proctor takes the next step. It creates a manual-testing contract, requires real evidence against that contract, and refuses completion until the contract is satisfied.

## Thesis

Agentic software development needs an equivalent of red-green-refactor for product verification.

Today the workflow looks like this:

- the agent implements a feature
- the agent adds or updates tests
- the agent may or may not manually verify the product
- the final answer often depends on trust

Proctor changes that contract:

- the agent must declare the feature or flow it is verifying
- Proctor interrogates the agent for the relevant surface and targets
- Proctor requires the agent to think through happy path, failure path, and all materially relevant edge cases
- the agent performs the checks with its own tools
- only recorded evidence counts
- `proctor done` fails until every obligation is satisfied

The wedge is not "another browser automation tool". The wedge is the missing enforcement layer between coding agents and verification tools.

## Product Position

Proctor is not:

- a browser automation framework
- an iOS automation framework
- a CLI/TUI runtime
- a hosted QA platform

Proctor is:

- a contract engine
- an evidence validator
- a completion gate
- a shareable reporting layer

It should work across web, iOS, CLI, and API surfaces, but it should not try to replace the best execution tool for each one.

## Core Decisions

These are the current product decisions for v0.

### 1. Proctor does not act as the browser agent

Proctor does not try to navigate pages, recover from browser failures, or improvise shell commands better than the coding agent can.

Instead:

- the agent tells Proctor what it is testing
- Proctor tells the agent what proof is required
- the agent uses its own tools to execute the checks
- the agent records evidence with Proctor

This keeps Proctor focused on enforcement instead of becoming a mediocre browser or mobile tool.

### 2. Browser verification is the default for user-visible features

For user-visible features, browser verification is required at minimum.

For HTTP-backed features, `curl` or equivalent API verification should be required when there is meaningful backend or protocol risk. It should not be mandatory for every purely visual or mostly client-side change.

In practice:

- web UI features should require browser evidence
- HTTP-backed flows with meaningful contract risk should require browser plus API evidence
- CLI features should require real terminal verification
- iOS features should require simulator-backed visual verification

Proctor should challenge the agent before allowing `curl` to be skipped. A skip should require a reason such as:

- the change is primarily visual or client-side
- there is no meaningful backend contract risk
- the important edge cases can be exercised reliably in the browser
- precise status or body assertions are not important for this flow

### 3. No fixed minimum edge-case count

Proctor should not let the agent cut corners with "list at least 2 edge cases".

Instead, the agent must enumerate all materially relevant edge cases it can think of. Proctor should require coverage by category and force the agent to either:

- provide one or more scenarios for that category, or
- explicitly mark it `N/A` with a reason

Suggested categories:

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

### 4. Only tool-recorded evidence counts

Freehand notes are useful, but they are not proof.

Proof should be derived from wrapper-recorded evidence such as:

- screenshots
- browser console and network reports
- curl request and response transcripts
- CLI pane captures and screenshots
- iOS screenshots and logs
- assertions tied to a declared obligation

### 5. Evidence trust model

Perfect local tamper-proofing is not realistic when the agent has shell access. Proctor should optimize for making fake verification expensive and obvious, while making real verification the easiest path.

Proposed trust tiers:

- `Tier 0`: freehand notes. These are commentary only and never satisfy obligations.
- `Tier 1`: imported artifacts. Useful for adoption and fallback, but explicitly weak.
- `Tier 2`: Proctor-observed command evidence. Proctor wraps a command and records argv, cwd, timestamps, exit status, stdout or stderr, hashes, and assertions.
- `Tier 3`: Proctor-owned session evidence. Evidence is tied to a session Proctor created or registered, such as a browser run, iOS simulator session, or similar higher-trust runtime context.

Required minimums:

- browser obligations must require `Tier 3` evidence
- iOS obligations must require `Tier 3` evidence
- CLI obligations may start at `Tier 2` for the MVP, with room to tighten later
- API and `curl` obligations should require at least `Tier 2`

Mandatory visual proof:

- browser obligations must require visual artifacts such as screenshots
- iOS obligations must require visual artifacts such as simulator screenshots

Provenance alone is not enough. An obligation must require both:

- valid provenance for the evidence tier
- scenario-specific assertions tied to the contract

Examples:

- a screenshot alone does not prove "login success"
- a curl transcript alone does not prove "invalid login handled correctly"

The evidence must also include the expected claim, such as:

- dashboard visible after successful login
- `401` returned for invalid credentials
- no duplicate requests on double-submit
- mobile layout remains usable
- no failed network requests for the exercised path

### 6. Proctor blocks completion

The key command is `proctor done`.

`proctor done` should exit non-zero until:

- the contract exists
- every applicable obligation has evidence
- required artifacts are present
- the evidence passes validation

If nobody checks `proctor done`, Proctor is just a good habit. If the agent session, wrapper, or CI checks `proctor done`, Proctor becomes real enforcement.

### 7. Artifacts stay out of the repo by default

Proctor should not clutter repositories or force engineers to update `.gitignore`.

Default artifact path:

```text
~/.proctor/runs/<repo-slug>/<run-id>/
```

This directory should contain the contract, artifacts, and generated reports for a single run.

### 8. Proctor generates a shareable final document

Proctor should produce a final document inspired by Showboat, but generated from recorded evidence instead of freehand markdown.

Proposed outputs:

- `contract.md`: the declared flow, required obligations, and pass or fail status
- `report.html`: a shareable review artifact with screenshots, logs, curl transcripts, and a pass matrix
- `artifacts/`: raw screenshots, transcripts, reports, and metadata

The report should be useful for:

- the engineer who asked for verification
- another agent reviewing the work
- a human reviewer who wants proof without replaying the session

## How Proctor Works

Proctor only knows what the agent tells it plus what it can inspect locally:

- CLI arguments
- interactive answers
- current working directory
- repository files
- git state
- environment variables
- previously recorded run data

It cannot read the hidden conversation context. That means the agent must provide the feature and targets explicitly.

The flow should be:

1. the human tells the agent to use `proctor --help`
2. the agent reads the help text and starts a run
3. Proctor interrogates the agent about the flow
4. Proctor creates the verification contract
5. the agent uses browser, shell, iOS, or CLI tooling to execute the checks
6. the agent records evidence against the contract
7. `proctor done` decides pass or fail
8. Proctor generates a shareable report

## Example Agent Flow

This is the current target interaction model.

```text
human: hey man I want to test this new feature, use proctor --help to manually test it

ai agent: ok, I am calling proctor --help now

tool: proctor --help

proctor:
  Proctor enforces manual verification contracts for agent-built software.

  For user-visible web features:
    - browser verification is required
    - curl/API verification is required when there is meaningful backend or protocol risk

  You must provide:
    - the feature or flow name
    - the browser URL
    - the HTTP endpoint(s), if applicable
    - the happy path
    - the main failure path
    - all materially relevant edge cases you can think of

  Proctor does not drive the browser for you. It defines the contract and
  validates the proof you bring back.

  Typical workflow:
    proctor start
    proctor status
    proctor record browser ...
    proctor record curl ...
    proctor done
    proctor report

  All artifacts are stored outside the repo under ~/.proctor by default.

ai agent: ok, I am starting proctor

tool: proctor start

proctor: what flow are you testing?
ai agent: new authentication flow

proctor: browser URL?
ai agent: http://127.0.0.1:3000/login

proctor: curl endpoint(s)?
ai agent:
  - POST /api/login
  - POST /api/logout

proctor: describe the happy path
ai agent: valid email and password redirect to dashboard and create a session

proctor: describe the main failure path
ai agent: invalid password shows an error and returns 401 without creating a session

proctor: list all materially relevant edge cases you can think of, grouped by category.
for any category that does not apply, mark it N/A with a reason.

ai agent:
  validation and malformed input:
    - invalid email format shows inline validation
    - password below minimum length is rejected
    - email with leading and trailing spaces is normalized correctly

  empty or missing input:
    - empty email and password show required-field errors

  retry or double-submit:
    - double-clicking submit does not create duplicate requests or duplicate sessions

  loading, latency, and race conditions:
    - slow response keeps button disabled and shows loading state
    - repeated Enter key during loading does not submit twice

  network or server failure:
    - 500 response shows generic failure UI
    - offline or network drop shows retryable error state

  auth and session state:
    - already-authenticated user visiting /login is redirected away
    - logout clears session and protected pages require login again

  refresh, back-navigation, and state persistence:
    - refresh after login preserves authenticated state
    - back navigation does not show stale logged-out login page after successful auth

  mobile or responsive behavior:
    - login page layout remains usable on mobile width
    - error messages remain visible without overlap on small screens

  accessibility and keyboard behavior:
    - tab order is correct
    - Enter submits from password field
    - error message is exposed correctly

  any feature-specific risks:
    - remember-me toggle persists session length correctly

proctor:
  contract created

  required proof:
    - browser happy path
    - browser main failure path
    - browser proof for each declared edge case above
    - desktop screenshot of success state
    - mobile screenshot of success state
    - browser console clean for exercised flows
    - browser network report for exercised flows
    - curl success transcript for login because auth has backend contract risk
    - curl failure transcript for invalid login
    - curl or API transcripts for relevant backend edge cases
    - final generated contract and report

  artifacts path:
    ~/.proctor/runs/<repo-slug>/<run-id>/

  live documents:
    contract.md
    report.html

  use your browser and shell tools now, then record evidence

ai agent:
  uses browser tooling to exercise the declared flows
  uses curl to exercise the declared HTTP cases
  records screenshots, console and network reports, and curl transcripts with proctor

tool: proctor status

proctor:
  complete:
    - browser happy path
    - browser failure path
    - validation cases
    - desktop screenshot
    - curl success transcript

  missing:
    - double-submit browser proof
    - mobile screenshot
    - offline or network failure proof
    - curl invalid-login transcript
    - keyboard accessibility proof

ai agent:
  performs remaining checks and records them

tool: proctor done

proctor:
  PASS
  all required obligations satisfied

  generated:
    ~/.proctor/runs/<repo-slug>/<run-id>/contract.md
    ~/.proctor/runs/<repo-slug>/<run-id>/report.html
    ~/.proctor/runs/<repo-slug>/<run-id>/artifacts/

tool: proctor report

proctor:
  shareable report ready
```

## Initial Command Model

The likely MVP command set is:

```text
proctor --help
proctor start
proctor status
proctor record browser ...
proctor record curl ...
proctor record cli ...
proctor record ios ...
proctor done
proctor report
```

Notes:

- `--help` should be long-form and agent-readable, similar in spirit to Showboat
- the interactive questioning belongs in `proctor start`, not inside `--help`
- `record` commands should validate evidence and attach it to specific obligations
- `done` is the gate
- `report` renders the shareable output from the evidence store

## MVP Scope

The first version should stay narrow and prove the product shape.

### Phase 1

- single-user local CLI
- interactive `proctor start`
- browser and `curl` evidence recording
- obligation store on disk
- `status`, `done`, and generated `contract.md`
- artifacts written to `~/.proctor`

### Phase 2

- HTML report generation
- CLI verification support
- iOS verification support
- richer obligation categories and validation rules

### Phase 3

- optional guard or wrapper mode around agent sessions
- CI enforcement via `proctor done`
- project-level defaults in `proctor.yaml`
- stronger report review UX

## Non-Goals For v0

- full autonomous test execution
- hosted dashboards
- replacing browser automation frameworks
- replacing mobile automation frameworks
- inferring the whole task from hidden LLM conversation state

## Current Gaps

These are the main product gaps or risks that still need design work.

### 1. Anti-gaming and evidence provenance

If the agent can attach arbitrary files or summarize its own results freely, it can game the system. Proctor needs a clear trust model for what counts as first-party evidence and how that evidence is tied to a run.

### 2. Obligation quality

If the questioning is weak, the contract will be weak. Proctor needs a strong interrogation flow that pushes the agent toward real risk analysis instead of generic happy-path thinking.

### 3. Evidence ingestion UX

We still need to decide whether `proctor record` should wrap external tools directly, ingest outputs after the fact, or support both. This affects trust, ergonomics, and how easy it is for agents to adopt.

### 4. Replayability versus proof

Some evidence is easiest to store as screenshots and logs, but the best evidence is often replayable. We still need to decide how much Proctor should care about re-runnable steps versus static artifacts.

### 5. Surface-specific richness

The web story is strongest so far. CLI and iOS need equally good contract semantics, artifact expectations, and examples so Proctor does not become "mostly a web tool".

### 6. Guard mode

`proctor done` is only a real enforcement boundary if something checks it. A future wrapper or CI mode will likely matter a lot for adoption and integrity.

### 7. Project configuration

The current design can start fully interactive, but real projects will eventually want defaults for routes, endpoints, schemes, commands, report preferences, and policy tuning.

## Open Questions

- how much evidence validation should happen at record-time versus done-time
- what the first on-disk schema should look like
- whether `record` should wrap external tools or just ingest their outputs
- when `proctor.yaml` should arrive versus keeping v0 fully interactive

## Immediate Next Step

Build the smallest possible version that can:

- ask the agent the right questions
- store a contract on disk
- record browser and curl evidence
- compute missing obligations
- fail `proctor done` until the contract is fully satisfied
