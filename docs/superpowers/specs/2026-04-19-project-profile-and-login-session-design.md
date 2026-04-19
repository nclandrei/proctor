# Project Profile & Reusable Login Session — Design

- Status: draft (design approved, plan pending)
- Date: 2026-04-19
- Owner: Andrei Nicolae

## Problem

Today, every time an agent runs `proctor --help` in a project, it rediscovers:

- what kind of project this is (web / iOS / desktop / CLI)
- where the dev server runs
- what the auth endpoint looks like
- which test user to use
- how to log in

Each run re-pays that discovery cost, and re-logs-in through the UI even when a valid session already exists on disk. For web projects with meaningful auth flows, this is the dominant source of friction.

## Goal

Give each project a persistent, user-local profile that proctor reads transparently, and (for web) a managed login-state file that lets the agent skip the login UI across runs. Proctor continues not to drive any browser; the feature is about *remembering* and *surfacing*, not *automating*.

## Non-Goals

- Proctor does not perform the login. The agent's browser tool still types credentials and produces the session blob.
- Proctor does not define a browser-session schema. The login file is opaque to proctor; it is whatever the agent's tool emits (Playwright `storageState`, Puppeteer cookies, etc.).
- No repo-tracked component. Nothing about this feature adds files to the user's git repo.
- No multi-profile support in v1. Layout is chosen so multi-profile is an additive change later.
- Session reuse applies only to web. iOS / desktop / CLI get the profile half (no rediscovery of scheme / bundle ID / command), not a session blob.

## Storage

Everything lives under `~/.proctor/` (honors the existing `PROCTOR_HOME` env var; no new root). No files are written to the repo.

```
~/.proctor/
├── runs/...                             # existing
├── repos/...                            # existing (active-run pointer)
└── profiles/
    └── <repo-slug>/                     # slug resolved from git remote, same as runs
        ├── profile.yaml                 # mode 0600
        └── session.json                 # mode 0600, web-only, opaque blob
```

`<repo-slug>` reuses the existing `RepoSlug()` logic so profiles, runs, and repo-state all key off the same identifier.

File permissions are 0600 on both files. Parent directories are 0700.

## Profile Schema

YAML, single file per repo. Platform-specific fields live in a tagged sub-block. Empty sub-blocks for non-active platforms are allowed (and omitted on write).

Illustrative example (shows every sub-block populated so the schema is visible in one place; in practice only the active platform's sub-block is written):

```yaml
version: 1
repo_slug: nclandrei-proctor
platform: web
incomplete: false
missing_fields: []
web:
  dev_url: http://127.0.0.1:3000
  auth_url: POST /api/session
  test_email: demo@example.com
  test_password: hunter2
  login:
    file: session.json              # relative to profile dir
    ttl: 12h                        # Go duration string
    saved_at: 2026-04-19T12:00:00Z  # RFC3339 UTC
    sha256: <hex>
ios:
  scheme: Pagena
  bundle_id: com.example.pagena
  simulator: iPhone 16 Pro
desktop:
  app_name: Firefox
  bundle_id: org.mozilla.firefox
cli:
  command: magellan prompts inspect onboarding
```

Rules:

- `version: 1` is mandatory. Forward-compatibility hook; proctor refuses profiles with unknown versions.
- `platform` is mandatory and drives which sub-block is required.
- `incomplete` is a computed mirror of `len(missing_fields) > 0`. Written for human readability; on load, proctor recomputes it from the fields and the effective rules for `platform`.
- `missing_fields` is the canonical list of required fields for the active platform that are empty. Consumers read this to decide whether to prompt.
- Secret fields (`test_password`, `web.login.*`) live in the same file; security comes from file perms.
- `web.login.file` is always `session.json` for now (no override in v1). The field exists so later we can relocate it.

Required fields per platform (v1):

- `web`: `dev_url`, `test_email`, `test_password`. `auth_url` is optional (recommendation, not gate).
- `ios`: `scheme`, `bundle_id`, `simulator`.
- `desktop`: `app_name`, `bundle_id`.
- `cli`: `command`.

## CLI Surface

All commands are new. None modify existing command behavior except `proctor start` and `proctor --help`.

### `proctor init [flags]`

Create or update the profile for the current repo. Idempotent: safe to re-run.

- If no profile file exists, runs `profile_detect.Detect(repoRoot)` to populate best-guess defaults (platform from `package.json` / `go.mod` / `Podfile` / `*.xcodeproj`, dev URL from common dev-server patterns, etc.).
- If a profile file already exists, detection is skipped — the existing profile is the base. `--force-detect` re-runs detection and merges it in.
- Precedence order, highest-wins: explicit flags > existing file > detection output.
- Writes atomically (write-temp, fsync, rename); holds an exclusive file lock on `profile.yaml` during the read-merge-write cycle so two concurrent `init` / `project set` calls can't clobber each other.
- Recomputes `missing_fields` and `incomplete` after merge.
- Prints the resulting profile (same output as `proctor project show`), including what's still missing.

Flags (web; analogous for other platforms):

```
--platform {web|ios|desktop|cli}
--url <dev-url>
--auth-url "<METHOD> <path>"
--test-email <email>
--test-password <password>
--ios-scheme <scheme>
--ios-bundle-id <id>
--ios-simulator <name>
--app-name <name>
--app-bundle-id <id>
--cli-command <command>
--login-ttl <duration>                # default 12h; namespaced under --login- on init
--force-detect                        # re-run detector even if profile exists
```

Exit codes:

- 0: profile written (may still be `incomplete`).
- Non-zero: unwritable path, permission failure, conflicting explicit flags (e.g., `--platform web` with `--ios-scheme`), unknown flags.

A completed `init` with `incomplete: true` is **not** an error — it's the expected state when the agent doesn't yet have credentials. The agent reads `missing_fields` to decide what to prompt the human for.

### `proctor project show`

Prints the profile in a human-readable form. Redacts `test_password` (shown as `***`). `web.login.sha256` and `web.login.file` are printed as-is — they are not secrets, and the session file's contents are never dumped by `project show`. Surfaces login freshness explicitly:

```
login:
  state: fresh (saved 3h ago, ttl 12h)
```

States: `missing`, `fresh`, `stale`, `corrupt` (file present but hash mismatch). Corruption is detected lazily — only when a read operation (`project show`, `start`) computes the current hash of `session.json` and compares it to the recorded `sha256`. `login save` always writes a matching hash, so corruption signals tampering or a crashed save, not normal use.

### `proctor project get <field>`

Prints one field's raw value to stdout. The canonical way for the agent to fetch a secret to pass to its browser tool, because it emits exactly the requested value (no redaction, no extra formatting) and never dumps the whole profile. Uses dotted-path notation: `web.test_password`, `web.login.file`.

Exit codes: 0 on success; non-zero if field is unknown or empty.

### `proctor project set <key>=<value> [<key>=<value>...]`

Update one or more fields in-place. Uses the same dotted-path notation. After write, recomputes `missing_fields` / `incomplete`. This is the "stamp credentials" command:

```
proctor project set web.test_password=hunter2
```

Accepts multiple pairs on one invocation so the agent can stamp several values atomically.

### `proctor login save --file <path>`

Web only. Copies the file at `<path>` into `~/.proctor/profiles/<slug>/session.json`, records `saved_at`, `sha256`, and leaves `ttl` as-is (or applies `--ttl <dur>` on this command to override). Fails if `platform != web`.

The file's contents are not validated — proctor treats it as an opaque blob. The agent is responsible for capturing it in whatever format its browser tool produces (e.g., Playwright's `storageState.json`).

### `proctor login invalidate`

Web only. Deletes `session.json`, clears `saved_at` and `sha256` from the profile.

## `proctor start` Integration

On invocation, `proctor start`:

1. Loads the profile for the current repo (if any).
2. For any unprovided flag, uses the profile value.
3. Passes explicit flags through unchanged when present (flags win).
4. If the resulting config still has missing required fields, fails with:
   ```
   profile incomplete: missing web.test_password
   fill it with:  proctor project set web.test_password=<value>
   or re-run:     proctor init --test-password=<value>
   ```
5. Writes a `profile_provenance` block into `run.json` recording which fields came from flags vs profile.

No change to record / verify / note / done.

## `proctor --help` Integration

`--help` stays a static, deterministic document (no I/O). But it is always canonical: any new flow lands in `--help` the same commit as the code.

Changes:

- New top-level section "Project profile" describing:
  - the per-repo profile concept
  - the first-time path (`proctor init`, optional human prompt, `proctor project set` to stamp)
  - the steady-state path (`proctor project show`, then `proctor start` auto-fills)
  - the login reuse loop (`proctor login save` after successful login, reuse until `stale`, then re-login)
- Updated quickref listing the new commands.
- Each new command has its own subcommand help (`proctor init --help`, etc.).

## Error Handling

- **Missing profile when `start` needs it:** print clear "no profile — run `proctor init` or pass flags explicitly" message. Exit non-zero.
- **Corrupt / unparseable YAML:** print the path and parse error. Exit non-zero. Never silently rewrite.
- **Wrong file mode:** on load, if perms are more permissive than 0600 on `profile.yaml` or `session.json`, print a warning and fix the mode (best-effort). Don't refuse — proctor is a dev tool.
- **Version mismatch:** `version != 1` → refuse with "profile version X unsupported by this proctor".
- **Platform / field conflict:** explicit flag for wrong platform (`--ios-scheme` with `platform: web`) → refuse at flag-parse.
- **Login corruption:** if `session.json` is present but its hash doesn't match the profile's recorded `sha256`, report `state: corrupt` in `project show` and treat as `missing` for freshness.

## Testing

Implementation uses red-green TDD (per user request). For each unit below:

1. Profile schema round-trip (YAML marshal/unmarshal, version check, required-field computation).
2. Profile store (atomic write, 0600 perms, locking, idempotent update).
3. Detector (per-platform signal matrix; golden fixtures for `package.json`, `go.mod`, `Podfile`, `*.xcodeproj`).
4. Login store (save-copy-hash, invalidate, freshness states).
5. `proctor init` command (new-file, update, flag precedence, incomplete computation).
6. `proctor project show / get / set` (redaction, exit codes, dotted-path resolution).
7. `proctor login save / invalidate` (copy, hash, TTL states, web-only gate).
8. `proctor start` integration (profile merge, provenance in run.json, incomplete errors).
9. `proctor --help` (snapshot test that required commands and sections appear).

Integration tests extend existing patterns in `cli_integration_test.go` / `verify_integration_test.go`.

## Out of Scope (v1)

- Multi-profile per repo (dev/staging, admin/user). Layout is forward-compatible.
- Env-var interpolation in profile values (`${PROCTOR_TEST_PASSWORD}`).
- OS keychain integration.
- Repo-tracked profile defaults (`.proctor/project.yaml`).
- Login-state schema validation (cookies / localStorage / URL shape).
- Session "liveness" probe that actually checks the saved login still works.
- CLI TUI / interactive `init`.
- Automatic platform re-detection on `start` if profile says one platform but repo signals another.

## Migration / Compat

- No migration needed. Existing users keep working: `proctor start` with flags continues to work exactly as today, because profile is strictly additive.
- `PROCTOR_HOME` continues to control the root.
- Existing `--session <id>` flag on `record`/`note`/`verify` is untouched. The new feature uses `proctor login` as its noun specifically to avoid overlap.
