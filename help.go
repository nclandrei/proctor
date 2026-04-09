package main

import (
	"fmt"
	"strings"

	"github.com/nclandrei/proctor/internal/proctor"
)

func commandHelp(args []string) (string, bool, error) {
	if len(args) == 0 {
		return rootHelpText(), true, nil
	}

	if args[0] == "--help" || args[0] == "-h" {
		return rootHelpText(), true, nil
	}

	if args[0] == "help" {
		text, err := topicHelp(args[1:])
		if err != nil {
			return "", false, err
		}
		return text, true, nil
	}

	switch args[0] {
	case "start":
		if wantsHelp(args[1:]) {
			return startHelpText(), true, nil
		}
	case "status":
		if wantsHelp(args[1:]) {
			return statusHelpText(), true, nil
		}
	case "note":
		if wantsHelp(args[1:]) {
			return noteHelpText(), true, nil
		}
	case "log":
		if wantsHelp(args[1:]) {
			return logHelpText(), true, nil
		}
	case "verify":
		if wantsHelp(args[1:]) {
			return verifyHelpText(), true, nil
		}
	case "done":
		if wantsHelp(args[1:]) {
			return doneHelpText(), true, nil
		}
	case "report":
		if wantsHelp(args[1:]) {
			return reportHelpText(), true, nil
		}
	case "record":
		if len(args) == 1 {
			return recordHelpText(), true, nil
		}
		switch args[1] {
		case "browser":
			if wantsHelp(args[2:]) {
				return recordBrowserHelpText(), true, nil
			}
		case "cli":
			if wantsHelp(args[2:]) {
				return recordCLIHelpText(), true, nil
			}
		case "ios":
			if wantsHelp(args[2:]) {
				return recordIOSHelpText(), true, nil
			}
		case "desktop":
			if wantsHelp(args[2:]) {
				return recordDesktopHelpText(), true, nil
			}
		case "curl":
			if wantsHelp(args[2:]) {
				return recordCurlHelpText(), true, nil
			}
		}
		if wantsHelp(args[1:]) {
			return recordHelpText(), true, nil
		}
	}

	return "", false, nil
}

func topicHelp(args []string) (string, error) {
	if len(args) == 0 {
		return rootHelpText(), nil
	}

	switch args[0] {
	case "start":
		return startHelpText(), nil
	case "status":
		return statusHelpText(), nil
	case "note":
		return noteHelpText(), nil
	case "log":
		return logHelpText(), nil
	case "verify":
		return verifyHelpText(), nil
	case "done":
		return doneHelpText(), nil
	case "report":
		return reportHelpText(), nil
	case "record":
		if len(args) == 1 {
			return recordHelpText(), nil
		}
		switch args[1] {
		case "browser":
			return recordBrowserHelpText(), nil
		case "cli":
			return recordCLIHelpText(), nil
		case "ios":
			return recordIOSHelpText(), nil
		case "desktop":
			return recordDesktopHelpText(), nil
		case "curl":
			return recordCurlHelpText(), nil
		default:
			return "", fmt.Errorf("unknown help topic: record %s", args[1])
		}
	default:
		return "", fmt.Errorf("unknown help topic: %s", strings.Join(args, " "))
	}
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func rootHelpText() string {
	return `proctor - make a coding agent prove it manually tested its own work

Quick reference (the verification loop, one pass per scenario):
  proctor start   -> define happy path, failure path, and edge-case scenarios
  proctor note    -> commit to intent BEFORE screenshotting ("what I'm about to test")
  proctor log     -> screenshot each step: what you did, what you see, how it compares
  proctor record  -> attach the screenshot + report + assertions
  proctor verify  -> write a verdict: does the screenshot satisfy the contract?
  proctor done    -> passes only if every scenario has note + record + verify

Commands:
  start       create a verification contract with happy/failure/edge scenarios
  status      show contract state: missing notes, pending verifications, gaps
  note        file pre-test intent for a (scenario, session) BEFORE recording
  log         record a step: action + screenshot + what you see + how it compares
  record      attach evidence (screenshots, reports, transcripts) to a scenario
  verify      write a verdict: does the screenshot satisfy the scenario contract?
  done        enforce contract; fails if any scenario is incomplete
  report      generate HTML/markdown summary for the run
  help TOPIC  detailed help (start | note | log | record | verify | done | status | report)

Gates that force the agent to slow down and look:
  record refuses without a pre-note for (scenario, session)
  done refuses while any scenario has pending-verification evidence
  done refuses if any scenario has no pre-note filed

Minimum lengths: pre-note 20 chars, verdicts/observations/comparisons 40 chars + 4 distinct words.
Vague filler like "looks good" or "as expected" is rejected.

---

Proctor is intentionally long-form below this line. The agent is supposed to
learn the workflow from this help text, then come back with real proof instead
of a hand-wavy "tested it" claim.

Reading this help text is not the task.
The task is to inspect the current diff, identify the user-visible change,
create the verification contract, run the manual checks, and record evidence.
Proctor is the verifier. It is not the feature under test.

Mandatory next step for the agent after reading this help:
  1. inspect the current repo diff and identify the feature to verify
  2. run proctor start with the right platform for that feature
  3. file a pre-test note for each scenario with proctor note --scenario X
     --session Y --notes "what I am about to test" BEFORE recording
  4. perform the actual manual checks, logging each step with proctor log:
     take a screenshot, LOOK AT IT, describe what you see, explain how it
     compares to the scenario. This is the Showboat pattern: you have eyes,
     use them, and write down what you actually see at every step.
  5. attach evidence with the relevant proctor record command(s)
  6. re-read each recorded screenshot and call proctor verify with a
     verdict stating whether the evidence satisfies the scenario contract
  7. finish with proctor done

Proctor does three things:
  1. creates a verification contract for the feature being tested
  2. records real evidence against named scenarios
  3. refuses completion until the contract is satisfied

Proctor is agent-agnostic. The same CLI should work from Codex, Claude Code, or
any other coding agent that can run shell commands.

When helpful, proctor start, proctor status, and proctor done recommend
known-good local capture workflows based on tools found on PATH. Those
recommendations are optional; the contract stays tool-agnostic.

Typical prompt to give an agent:
  We just implemented the new authentication flow.
  Use proctor --help to manually test it.

Typical web workflow:
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
    --edge-case "any feature-specific risks=N/A: no extra feature-specific risks for this flow"

  # Before recording, file a pre-test note committing to what you will check:
  proctor note \
    --scenario happy-path \
    --session auth-browser-1 \
    --notes "about to verify the dashboard loads after valid login and shows the user email"

  # Produce real browser artifacts with your browser tool of choice:
  #   desktop.png
  #   mobile.png
  #   report.json
  #
  # Then attach them to the contract:
  proctor record browser \
    --scenario happy-path \
    --session auth-browser-1 \
    --report /abs/path/report.json \
    --screenshot desktop=/abs/path/desktop.png \
    --screenshot mobile=/abs/path/mobile.png \
    --assert 'final_url contains /dashboard' \
    --assert 'desktop_screenshot = true' \
    --assert 'mobile_screenshot = true'

  # When a scenario carries backend or protocol risk, also wrap a real curl command:
  proctor record curl \
    --scenario failure-path \
    --assert 'status = 401' \
    --assert 'body contains invalid' \
    -- \
    curl -si -X POST http://127.0.0.1:3000/api/login \
      -H 'content-type: application/json' \
      -d '{"email":"demo@example.com","password":"wrong"}'

  # Re-read each recorded screenshot and write a verdict on whether it satisfies the contract:
  proctor verify \
    --scenario happy-path \
    --session auth-browser-1 \
    --verdict "This satisfies the happy-path contract because the dashboard shows 'Hello, demo@example.com' and the Sign out button is visible top-right, matching the expected redirect-to-dashboard behavior."

  proctor status
  proctor done
  proctor report

Typical iOS workflow:
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
    --edge-case "any feature-specific risks=N/A: no extra feature-specific risks for this flow"

  # use your own simulator tooling to build, launch, screenshot, and inspect logs.
  # Proctor only needs the recorded artifacts and a small ios-report.json file.
  proctor record ios \
    --scenario happy-path \
    --session pagena-library-1 \
    --report /abs/path/ios-report.json \
    --screenshot library=/abs/path/library.png \
    --assert 'screen contains Library' \
    --assert 'bundle_id = com.example.pagena' \
    --assert 'app_launch = true'

Typical CLI workflow:
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
    --edge-case "stderr, exit codes, and partial failure reporting=Unknown prompt returns a non-zero exit code and prints the error on stderr"

  # Proctor does not open the terminal for you.
  # Preferred, not required: use a real terminal app plus tmux or an equivalent
  # persistent multiplexer so the agent can keep one session alive, drive
  # keyboard input deterministically, capture pane output, and take screenshots.
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

Typical desktop workflow:
  proctor start \
    --platform desktop \
    --feature "bookmark manager" \
    --app-name "Firefox" \
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
    --edge-case "window management, resize, and multi-monitor=N/A: covered elsewhere" \
    --edge-case "drag-drop, clipboard, and system integration=N/A: no drag-drop in this flow" \
    --edge-case "keyboard shortcuts and accessibility=N/A: this pass is visual only" \
    --edge-case "any feature-specific risks=N/A: no additional risks"

  # Use peekaboo or screencapture to capture window screenshots without stealing focus:
  #   peekaboo see --app Firefox --mode window --path /tmp/firefox.png --json
  #   Write desktop-report.json from the captured data.
  proctor record desktop \
    --scenario happy-path \
    --session firefox-desktop-1 \
    --report /abs/path/desktop-report.json \
    --screenshot window=/abs/path/window.png \
    --assert 'app_name contains Firefox' \
    --assert 'crashes = 0'

What counts as desktop evidence:
  - a desktop session id string
  - at least one window screenshot
  - a desktop-report.json artifact with app metadata and issue counts
  - assertions tied to the scenario

The desktop report only needs a small shape:
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

Use your own window capture tooling. Peekaboo is recommended because it uses
ScreenCaptureKit for background capture without stealing focus. Fallback:
screencapture -x -l <windowID> plus osascript for window metadata.

What counts as browser evidence:
  - a session id string for the browser run
  - a desktop screenshot
  - a mobile screenshot
  - a report JSON artifact
  - assertions tied to the scenario

The report JSON does not need to come from one specific helper. If your browser
tool gives you the final URL and issue counts separately, write a small
report.json file that matches the documented shape and attach that.

For web runs, mobile proof is mandatory. Proctor requires at least one desktop
and at least one mobile screenshot somewhere in the recorded browser evidence.
Console warnings are still recorded in the report, but the default blocking
gate only fails on console errors, page errors, failed requests, and HTTP
errors. Add an explicit assertion such as "console_warnings = 0" when warnings
must block a scenario.

The browser report only needs a small shape:
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

If your browser tool does not emit this exact file, you can still use Proctor:
capture the real browser data, then write a tiny JSON file with these fields.

What counts as iOS evidence:
  - a simulator or run session id string
  - at least one simulator screenshot
  - an ios-report.json artifact with simulator, app, and issue metadata
  - assertions tied to the scenario

The iOS report only needs a small shape:
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

use your own simulator tooling. Proctor does not boot the simulator, call Xcode,
or take the screenshots for you. It only records the evidence and blocks
completion when the contract is incomplete.

What counts as CLI evidence:
  - a real terminal session id string
  - at least one terminal screenshot
  - a transcript artifact from that terminal session
  - the actual command that was exercised
  - assertions tied to the scenario

Use subcommand help for exact flags:
  proctor start --help
  proctor note --help
  proctor log --help
  proctor record browser --help
  proctor record cli --help
  proctor record ios --help
  proctor record desktop --help
  proctor record curl --help
  proctor verify --help
  proctor done --help

Outputs:
  - raw artifacts live under ~/.proctor by default
  - contract.md is the human-readable contract
  - report.html is the shareable report
  - report.html is a plain document with a light theme
` + allPlatformRecommendationSection()
}

func startHelpText() string {
	var b strings.Builder
	b.WriteString(`proctor start - create the verification contract

Usage:
  proctor start [flags]

You can run start interactively, but agents usually do better with explicit
flags so the contract is reproducible.

Before choosing --feature, inspect the current repo diff and identify the
user-visible change that actually needs verification. Do not substitute a
generic smoke test that is unrelated to the current diff.

Required:
  --platform web|ios|cli|desktop
                             Defaults to web
  --feature TEXT             Human label for the feature or flow under test
  --happy-path TEXT          Primary success scenario
  --failure-path TEXT        Primary failure or back-out scenario
  --edge-case "CATEGORY=..." Edge-case coverage by category

Platform-specific flags:
  web:
    --url URL                Browser URL for the flow
    --curl required|scenario|skip
                             HTTP verification mode for the contract
  ios:
    --ios-scheme TEXT        Xcode scheme or app target name
    --ios-bundle-id TEXT     App bundle id to launch on the simulator
    --ios-simulator TEXT     Optional simulator label to pin the intended device
    --curl required|scenario|skip
                             HTTP verification mode for the contract
  desktop:
    --app-name TEXT          Name of the desktop application under test
    --app-bundle-id TEXT     Optional macOS bundle identifier
    --curl required|scenario|skip
                             HTTP verification mode for the contract
  cli:
    --cli-command TEXT       Command line the agent is manually exercising

Conditional flags:
  --curl-endpoint TEXT       Repeat once per endpoint when --curl required
                             or once per risky scenario when --curl scenario
  --curl-skip-reason TEXT    Required when --curl skip on web or ios

curl modes:
  required  shorthand that requires curl for happy-path and failure-path
  scenario  require curl only for named risky scenarios
  skip      require no curl evidence for this web or ios run; must include a reason

scenario curl format:
  --curl-endpoint "happy-path=POST /api/login"
  --curl-endpoint "failure-path=POST /api/login"
  --curl-endpoint "auth and session state:Already signed-in users are redirected away from /login=GET /api/session"

Edge-case format:
  --edge-case "CATEGORY=scenario one; scenario two"
  --edge-case "CATEGORY=N/A: short reason"

Every category must be covered either by scenarios or by an explicit N/A reason.
If any platform category is omitted, or uses bare N/A with no reason, proctor start fails.
Web categories:
`)
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformWeb) {
		b.WriteString("  - " + category + "\n")
	}
	b.WriteString("\niOS categories:\n")
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformIOS) {
		b.WriteString("  - " + category + "\n")
	}
	b.WriteString("\nCLI categories:\n")
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformCLI) {
		b.WriteString("  - " + category + "\n")
	}
	b.WriteString("\nDesktop categories:\n")
	for _, category := range proctor.EdgeCaseCategoriesForPlatform(proctor.PlatformDesktop) {
		b.WriteString("  - " + category + "\n")
	}
	b.WriteString(`
Static web example:
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

HTTP-backed example:
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

iOS example:
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

CLI example:
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

Desktop example:
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

After start:
  - run your platform checks
  - attach web evidence with proctor record browser, iOS evidence with proctor record ios, desktop evidence with proctor record desktop, or terminal evidence with proctor record cli
  - wrap curl with proctor record curl for the scenarios that require it on web, ios, or desktop runs
  - re-read each recorded screenshot and close the loop with proctor verify --scenario X --session Y --verdict "..."
  - finish with proctor done
`)
	b.WriteString(allPlatformRecommendationSection())
	return b.String()
}

func recordHelpText() string {
	return `proctor record - attach evidence to an existing run

Usage:
  proctor record browser [flags]
  proctor record cli [flags]
  proctor record ios [flags]
  proctor record desktop [flags]
  proctor record curl [flags] -- <command>

Use:
  proctor record browser --help
  proctor record cli --help
  proctor record ios --help
  proctor record desktop --help
  proctor record curl --help

Important:
  - browser evidence attaches one browser run to one named scenario
  - cli evidence attaches one terminal session run to one named scenario
  - ios evidence attaches one simulator run to one named scenario
  - desktop evidence attaches one desktop app session to one named scenario
  - curl evidence wraps one real HTTP command for one named scenario
  - curl evidence must produce a real HTTP response and match one declared curl endpoint for that scenario
  - curl requirements are decided per scenario, not by endpoint alone
  - only recorded evidence counts toward proctor done
  - every record call BLOCKS until a pre-test note has been filed for the
    (scenario, session) pair with proctor note --scenario X --session Y
    --notes "..." (minimum 20 chars)
  - every recorded screenshot evidence enters pending-verification and must
    be closed with proctor verify --scenario X --session Y --verdict "..." (minimum 40 chars)
    before proctor done can pass
`
}

func recordBrowserHelpText() string {
	return `proctor record browser - attach browser evidence to one scenario

Usage:
  proctor record browser \
    --scenario ID \
    --session SESSION \
    --report /abs/path/report.json \
    --screenshot desktop=/abs/path/desktop.png \
    [--screenshot mobile=/abs/path/mobile.png] \
    --assert 'EXPRESSION' \
    [--assert 'EXPRESSION' ...]

Required:
  --scenario ID              Scenario id from contract.md or proctor status
  --session SESSION          Stable browser run label or session id string
  --report PATH              JSON report with desktop and mobile final URL and issue counts
  --screenshot LABEL=PATH    At least one screenshot; every web run still needs desktop and mobile coverage overall
  --assert TEXT              At least one passing assertion

Optional:
  --tool NAME                Defaults to agent-browser
  --fail-assert TEXT         Invert one assertion when you need to prove a failure condition

Supported browser assertions:
  final_url contains /dashboard
  final_url = http://127.0.0.1:3000/login
  console_errors = 0
  console_warnings = 0
  failed_requests = 0
  http_errors = 1
  desktop_screenshot = true
  mobile_screenshot = true
  mobile.final_url contains /login

Expected report JSON shape:
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

The JSON can be synthesized from real browser-session output. It does not need
to be emitted by one specific browser helper.

Happy-path example:
  proctor record browser \
    --scenario happy-path \
    --session auth-browser-1 \
    --report /abs/path/report.json \
    --screenshot desktop=/abs/path/desktop.png \
    --screenshot mobile=/abs/path/mobile.png \
    --assert 'final_url contains /dashboard' \
    --assert 'desktop_screenshot = true' \
    --assert 'mobile_screenshot = true'

Notes:
  - one report can be reused for multiple scenarios if it genuinely proves each one
  - every web run must record at least one desktop screenshot and at least one mobile screenshot before proctor done can pass
  - implicit zero-issues assertions only cover console errors, page errors, failed requests, and HTTP errors
  - console warnings are recorded in the report but stay non-blocking unless you assert them explicitly
  - screenshots must be at least 10KB; smaller files are rejected as likely placeholders
  - screenshots must be fresh (modified within the last 30 minutes)
` + platformRecommendationSection(proctor.PlatformWeb, true)
}

func recordCLIHelpText() string {
	return `proctor record cli - attach terminal evidence to one scenario

Usage:
  proctor record cli \
    --scenario ID \
    --session SESSION \
    --command "cli subcommand --flag" \
    --transcript /abs/path/pane.txt \
    --screenshot LABEL=/abs/path/terminal.png \
    [--exit-code N] \
    --assert 'EXPRESSION' \
    [--assert 'EXPRESSION' ...]

Required:
  --scenario ID              Scenario id from contract.md or proctor status
  --session SESSION          Stable terminal session label or id string
  --command TEXT             Actual command line exercised in the terminal
  --transcript PATH          Captured terminal transcript from that session
  --screenshot LABEL=PATH    At least one screenshot from the verified terminal run
  --assert TEXT              At least one passing assertion

Optional:
  --tool NAME                Defaults to terminal-session
  --exit-code N              Captured process exit code when relevant
  --fail-assert TEXT         Invert one assertion when you need to prove a failure condition

Supported cli assertions:
  output contains onboarding
  output contains prompt not found
  command contains magellan
  session contains cli-session
  tool = terminal-session
  exit_code = 0
  screenshot = true

Example:
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

Notes:
  - Preferred, not required: use a real terminal app plus tmux or an equivalent persistent multiplexer
  - one transcript and screenshot set can be reused for multiple scenarios if it genuinely proves each one
  - every cli scenario needs a transcript, at least one screenshot, and at least one passing assertion
  - transcripts must be at least 10 bytes; shorter files are rejected as empty
  - screenshots must be at least 10KB; smaller files are rejected as likely placeholders
  - screenshots must be fresh (modified within the last 30 minutes)
` + platformRecommendationSection(proctor.PlatformCLI, false)
}

func recordIOSHelpText() string {
	return `proctor record ios - attach iOS simulator evidence to one scenario

Usage:
  proctor record ios \
    --scenario ID \
    --session SESSION \
    --report /abs/path/ios-report.json \
    --screenshot LABEL=/abs/path/screen.png \
    --assert 'EXPRESSION' \
    [--assert 'EXPRESSION' ...]

Required:
  --scenario ID              Scenario id from contract.md or proctor status
  --session SESSION          Stable simulator session label or run id string
  --report PATH              JSON report with simulator, app, and issue metadata
  --screenshot LABEL=PATH    At least one screenshot from the verified simulator run
  --assert TEXT              At least one passing assertion

Optional:
  --tool NAME                Defaults to ios-simulator
  --fail-assert TEXT         Invert one assertion when you need to prove a failure condition

Supported ios assertions:
  screen contains Library
  bundle_id = com.example.pagena
  simulator contains iPhone 16 Pro
  runtime contains iOS
  state = foreground
  app_launch = true
  launch_errors = 0
  crashes = 0
  fatal_logs = 0
  screenshot = true

Expected report JSON shape:
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

The JSON can be synthesized from real simulator-session output. It does not need
to be emitted by one specific helper.

Example:
  proctor record ios \
    --scenario happy-path \
    --session pagena-library-1 \
    --report /abs/path/ios-report.json \
    --screenshot library=/abs/path/library.png \
    --assert 'screen contains Library' \
    --assert 'bundle_id = com.example.pagena' \
    --assert 'app_launch = true'

Notes:
  - one simulator report can be reused for multiple scenarios if it genuinely proves each one
  - every ios run must record at least one screenshot before proctor done can pass
  - implicit zero-issue assertions cover launch errors, crashes, and fatal logs
  - screenshots must be at least 10KB; smaller files are rejected as likely placeholders
  - screenshots must be fresh (modified within the last 30 minutes)
` + platformRecommendationSection(proctor.PlatformIOS, true)
}

func recordDesktopHelpText() string {
	return `proctor record desktop - attach desktop app evidence to one scenario

Usage:
  proctor record desktop \
    --scenario ID \
    --session SESSION \
    --report /abs/path/desktop-report.json \
    --screenshot LABEL=/abs/path/window.png \
    --assert 'EXPRESSION' \
    [--assert 'EXPRESSION' ...]

Required:
  --scenario ID              Scenario id from contract.md or proctor status
  --session SESSION          Stable desktop session label or id string
  --report PATH              JSON report with app metadata and issue counts
  --screenshot LABEL=PATH    At least one screenshot from the verified desktop run
  --assert TEXT              At least one passing assertion

Optional:
  --tool NAME                Defaults to peekaboo
  --fail-assert TEXT         Invert one assertion when you need to prove a failure condition

Supported desktop assertions:
  app_name contains Firefox
  bundle_id = org.mozilla.firefox
  state = running
  window_title contains Bookmarks
  crashes = 0
  fatal_logs = 0
  screenshot = true

Expected report JSON shape:
  {
    "app": {
      "name": "Firefox",
      "bundleId": "org.mozilla.firefox",
      "pid": 12345,
      "state": "running",
      "windowTitle": "Bookmark Manager"
    },
    "issues": {
      "crashes": 0,
      "fatalLogs": 0
    }
  }

The JSON can be synthesized from real window-capture output. It does not need
to be emitted by one specific helper.

Example:
  proctor record desktop \
    --scenario happy-path \
    --session firefox-desktop-1 \
    --report /abs/path/desktop-report.json \
    --screenshot window=/abs/path/window.png \
    --assert 'app_name contains Firefox' \
    --assert 'crashes = 0' \
    --assert 'screenshot = true'

Notes:
  - one desktop report can be reused for multiple scenarios if it genuinely proves each one
  - every desktop run must record at least one screenshot before proctor done can pass
  - implicit zero-issue assertions cover crashes and fatal logs
  - screenshots must be at least 10KB; smaller files are rejected as likely placeholders
  - screenshots must be fresh (modified within the last 30 minutes)
` + platformRecommendationSection(proctor.PlatformDesktop, true)
}

func recordCurlHelpText() string {
	return `proctor record curl - wrap one real HTTP command for one scenario

Usage:
  proctor record curl \
    --scenario ID \
    --assert 'EXPRESSION' \
    [--assert 'EXPRESSION' ...] \
    -- <command>

Required:
  --scenario ID              Scenario id from contract.md or proctor status
  --assert TEXT              At least one passing assertion
  -- <command>               Real curl command or equivalent HTTP client command that returns an HTTP response

Optional:
  --fail-assert TEXT         Invert one assertion when you need to prove a failure condition

Supported curl assertions:
  status = 200
  status = 401
  exit_code = 0
  body contains invalid
  header.content-type contains application/json

Example:
  proctor record curl \
    --scenario failure-path \
    --assert 'status = 401' \
    --assert 'body contains invalid' \
    --assert 'header.content-type contains application/json' \
    -- \
    curl -si -X POST http://127.0.0.1:3000/api/login \
      -H 'content-type: application/json' \
      -d '{"email":"demo@example.com","password":"wrong"}'

Use the scenario ids from the contract. If proctor start used --curl scenario,
only the named risky scenarios need curl evidence. The wrapped request must
match one of that scenario's declared curl endpoints.
` + platformRecommendationSection(proctor.PlatformWeb, true)
}

func statusHelpText() string {
	return `proctor status - show contract coverage for the active run

Usage:
  proctor status

This prints:
  - every scenario in the contract
  - whether a pre-test note has been filed for each scenario
  - whether browser evidence passes or fails
  - whether cli evidence passes or fails
  - whether ios evidence passes or fails
  - whether desktop evidence passes or fails
  - whether curl evidence passes or fails for scenarios that require it
  - whether recorded evidence is still pending-verification (awaiting proctor verify)
  - any global gaps such as missing required screenshots

Use this after each record step so the agent can see what is still missing.
Scenarios with pending-verification evidence appear here until the agent
writes observation notes with proctor verify. Scenarios that are missing a
pre-test note also appear here with a reminder to run proctor note first.
When the run is incomplete, proctor status also prints platform-specific local
capture recommendations based on tools found on PATH.
`
}

func noteHelpText() string {
	return `proctor note - file a pre-test note BEFORE recording evidence

Usage:
  proctor note \
    --scenario ID \
    --session SESSION \
    --notes "what I am about to test"

Required:
  --scenario ID      Scenario id from contract.md or proctor status
  --session SESSION  Stable session id you will reuse for proctor record
  --notes TEXT       Free-text statement of intent (minimum 20 characters)

Pre-notes are the forcing function. Before any proctor record browser/ios/
desktop/cli/curl call, proctor expects the agent to commit to specifics
about the flow they are about to verify, in the same session string they
will use when they record the screenshot. The contract describes the
scenario in the abstract; the pre-note is the agent's statement of
"right now, in this session, I am about to test X in concrete terms."

proctor record refuses to accept evidence when no pre-note exists for the
(scenario, session) pair. proctor done refuses to pass when any scenario
has evidence but no pre-note attached. Both gates apply to every surface
including curl.

Multiple pre-notes per (scenario, session) are allowed. The first note
satisfies the record gate; additional notes create an audit trail for
cases where the agent extends what they intend to check mid-session.

Pre-notes should be specific and falsifiable, not generic:

  bad:  "about to test login"
  bad:  "will verify the screen"
  good: "about to submit the login form with a valid email and the
         correct password and confirm we land on /dashboard"

Example:
  proctor note \
    --scenario happy-path \
    --session auth-browser-1 \
    --notes "about to log in with demo@example.com and expect redirect to /dashboard with a Sign out link"
`
}

func logHelpText() string {
	return `proctor log - record a verification step with screenshot + observation

Usage:
  proctor log \
    --scenario ID \
    --session SESSION \
    --surface SURFACE \
    --screenshot /abs/path/step.png \
    --action "what I just did" \
    --observation "what I see in the screenshot" \
    --comparison "how what I see relates to the scenario"

Required:
  --scenario ID          Scenario id from contract.md or proctor status
  --session SESSION      Stable session id (reuse for proctor record later)
  --surface SURFACE      One of: browser, ios, cli, desktop
  --screenshot PATH      Absolute path to the screenshot from this step
  --action TEXT          What the agent did at this step (minimum 20 characters)
  --observation TEXT     What the agent sees in the screenshot (minimum 40 chars, 4+ distinct words)
  --comparison TEXT      How what the agent sees compares to the scenario (minimum 40 chars, 4+ distinct words)

proctor log is the Showboat pattern: the agent takes a screenshot, LOOKS
AT IT with its own vision, writes down what it actually sees, and explains
how that compares to what the scenario requires. Proctor enforces the
structure; the agent provides the eyes.

Each call captures one step. The agent is expected to:

  1. Do something (navigate, click, type, run a command)
  2. Take a screenshot of the result
  3. Open that screenshot and LOOK AT IT
  4. Describe what is actually visible: UI elements, text, layout, state
  5. Explain how what it sees compares to the scenario's requirements

Steps are stored in screenshot-log.jsonl and included in the verification
report. They build a chronological visual audit trail of the entire
verification process.

The log is optional for proctor done to pass, but agents that log their
steps produce richer evidence and more trustworthy verification.

Actions must be at least 20 characters. Observations and comparisons must
be at least 40 characters with 4+ distinct words. Vague filler phrases
like "looks good", "as expected", or "no issues" are rejected outright.

Good observations name specific visible elements:
  bad:  "the page looks correct as expected"
  good: "login form with email input, password input, blue Sign In button,
         and a Forgot password? link below the form"

Good comparisons reference the scenario requirements:
  bad:  "this matches what was expected of the scenario"
  good: "the dashboard greeting shows the user email matching the happy-path
         requirement that valid credentials redirect to the dashboard"

Example workflow (web login verification):

  proctor log \
    --scenario happy-path \
    --session auth-browser-1 \
    --surface browser \
    --screenshot /tmp/step1-login-page.png \
    --action "Navigated to http://127.0.0.1:3000/login" \
    --observation "Login form with email input, password input, and blue Sign In button. Page title is Login. No errors visible." \
    --comparison "Login page is present as expected. The scenario needs valid credentials to redirect to dashboard - ready to enter them."

  proctor log \
    --scenario happy-path \
    --session auth-browser-1 \
    --surface browser \
    --screenshot /tmp/step2-filled-form.png \
    --action "Entered demo@example.com in email field and typed password, then clicked Sign In" \
    --observation "Page is now showing the dashboard. URL bar shows /dashboard. Greeting says Hello, demo@example.com. Sign out button visible top-right." \
    --comparison "This matches the happy-path scenario: valid credentials redirected to the dashboard with the user greeting visible."

Steps are numbered automatically per (scenario, session) pair.
`
}

func verifyHelpText() string {
	return `proctor verify - write a verdict on whether the evidence satisfies the contract

Usage:
  proctor verify \
    --scenario ID \
    --session SESSION \
    --verdict "state whether the screenshot satisfies the contract and why"

Required:
  --scenario ID      Scenario id matching a recorded evidence entry
  --session SESSION  Session id used when the evidence was recorded
  --verdict TEXT     Verdict (minimum 40 chars, 4+ distinct words, must include a judgment word)

Every proctor record command marks its evidence pending-verification. The
agent is expected to:

  1. Re-read the screenshot it just recorded
     (any multimodal agent can do this with its own file-read capability)
  2. Compare what is visible against the scenario's contract claim
  3. State whether the evidence satisfies the contract and why, using at
     least one judgment word: satisfies, confirms, proves,
     demonstrates, fails, does not, missing, incorrect, because

proctor done refuses to pass while any scenario's most-recent evidence is
still pending-verification. Verdicts are stored alongside the evidence for
the contract report. Each evidence record can only be verified once.

Good verdicts connect what is visible to the contract:

  good: "This satisfies the happy-path contract because the dashboard
         shows 'Hello, demo@example.com' and the Sign out button is
         visible top-right, matching the expected redirect-to-dashboard
         behavior."
  good: "This fails the failure-path contract because the error message
         is missing; the page shows a blank form instead of the expected
         'invalid credentials' text."

Bad verdicts describe pixels without judging the contract:

  bad:  "ok"
  bad:  "looks good"
  bad:  "dashboard is visible" (too vague, no contract connection)

Example:
  proctor verify \
    --scenario happy-path \
    --session auth-browser-1 \
    --verdict "This satisfies the contract because the dashboard greeting and Sign out link match the expected behavior."
`
}

func doneHelpText() string {
	return `proctor done - enforce completion

Usage:
  proctor done

Passes only when:
  - every scenario with recorded evidence has at least one pre-test note filed
    before that evidence via proctor note
  - every required scenario has valid evidence
  - browser scenarios have trusted browser evidence
  - cli scenarios have trusted terminal evidence
  - ios scenarios have trusted ios evidence
  - desktop scenarios have trusted desktop app evidence
  - every recorded piece of evidence has been verified with proctor verify
  - the run includes the required screenshot coverage for its platform
  - required assertions pass
  - artifact hashes still match
  - no duplicate screenshots are reused across different scenarios
  - the run is not expired (max 2 hours from start)

Recording a screenshot is not enough on its own. Before each proctor record
call, run proctor note --scenario ID --session SESSION --notes "..." to
commit to what you are about to test. After each proctor record call, run
proctor verify --scenario ID --session SESSION --verdict "..." with a
verdict stating whether the evidence satisfies the contract. proctor done
will refuse to pass while any scenario has evidence without a pre-note, or
while any scenario still has pending-verification evidence.

If the contract is incomplete, proctor done exits non-zero and prints what is
still missing. This is the command the agent should treat as the real
definition of done. When it fails, it also prints platform-specific local
capture recommendations based on tools found on PATH.
`
}

func reportHelpText() string {
	return `proctor report - print the generated contract and report paths

Usage:
  proctor report

Outputs:
  - contract.md
  - report.html

Both files live under ~/.proctor by default unless PROCTOR_HOME is set.
The HTML report uses a light theme.
`
}
