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

Proctor is intentionally long-form. The agent is supposed to learn the workflow
from this help text, then come back with real proof instead of a hand-wavy
"tested it" claim.

Proctor does three things:
  1. creates a verification contract for the feature being tested
  2. records real evidence against named scenarios
  3. refuses completion until the contract is satisfied

Proctor is agent-agnostic. The same CLI should work from Codex, Claude Code, or
any other coding agent that can run shell commands.

Typical prompt to give an agent:
  We just implemented the new authentication flow.
  Use proctor --help to manually test it.

Typical web workflow:
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
    --edge-case "any feature-specific risks=N/A: no extra feature-specific risks for this flow"

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

  # When backend or protocol risk matters, also wrap a real curl command:
  proctor record curl \
    --scenario failure-path \
    --assert 'status = 401' \
    --assert 'body contains invalid' \
    -- \
    curl -si -X POST http://127.0.0.1:3000/api/login \
      -H 'content-type: application/json' \
      -d '{"email":"demo@example.com","password":"wrong"}'

  proctor status
  proctor done
  proctor report

What counts as browser evidence:
  - a session id string for the browser run
  - a desktop screenshot
  - a report JSON artifact
  - a mobile screenshot when mobile or responsive proof matters
  - assertions tied to the scenario

The report JSON does not need to come from one specific helper. If your browser
tool gives you the final URL and issue counts separately, write a small
report.json file that matches the documented shape and attach that.

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

If your browser tool does not emit this exact file, you can still use Proctor:
capture the real browser data, then write a tiny JSON file with these fields.
  }

Use subcommand help for exact flags:
  proctor start --help
  proctor record browser --help
  proctor record curl --help
  proctor done --help

Outputs:
  - raw artifacts live under ~/.proctor by default
  - contract.md is the human-readable contract
  - report.html is the shareable report
  - report.html is always rendered in dark mode
`
}

func startHelpText() string {
	var b strings.Builder
	b.WriteString(`proctor start - create the verification contract

Usage:
  proctor start [flags]

You can run start interactively, but agents usually do better with explicit
flags so the contract is reproducible.

Required web flags:
  --feature TEXT             Human label for the feature or flow under test
  --url URL                  Browser URL for the flow
  --curl required|skip       Whether direct HTTP verification is required
  --happy-path TEXT          Primary success scenario
  --failure-path TEXT        Primary failure or back-out scenario
  --edge-case "CATEGORY=..." Edge-case coverage by category

Conditional flags:
  --curl-endpoint TEXT       Repeat once per endpoint when --curl required
  --curl-skip-reason TEXT    Required when --curl skip

Edge-case format:
  --edge-case "CATEGORY=scenario one; scenario two"
  --edge-case "CATEGORY=N/A: short reason"

Every category must be covered either by scenarios or by an explicit N/A reason.
Current categories:
`)
	for _, category := range proctor.EdgeCaseCategories {
		b.WriteString("  - " + category + "\n")
	}
	b.WriteString(`
Static web example:
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

HTTP-backed example:
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

After start:
  - run your browser checks
  - attach evidence with proctor record browser
  - wrap curl with proctor record curl when required
  - finish with proctor done
`)
	return b.String()
}

func recordHelpText() string {
	return `proctor record - attach evidence to an existing run

Usage:
  proctor record browser [flags]
  proctor record curl [flags] -- <command>

Use:
  proctor record browser --help
  proctor record curl --help

Important:
  - browser evidence attaches one browser run to one named scenario
  - curl evidence wraps one real command for one named scenario
  - only recorded evidence counts toward proctor done
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
  --report PATH              JSON report with desktop final URL and issue counts
  --screenshot LABEL=PATH    At least one screenshot; use desktop=... and mobile=...
  --assert TEXT              At least one passing assertion

Optional:
  --tool NAME                Defaults to agent-browser
  --fail-assert TEXT         Invert one assertion when you need to prove a failure condition

Supported browser assertions:
  final_url contains /dashboard
  final_url = http://127.0.0.1:3000/login
  console_errors = 0
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

The JSON can be synthesized from real browser-session output. It does not need
to be emitted by one specific browser helper.
  }

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
  - Proctor will also add implicit zero-issues assertions for console, page, network, and HTTP failures unless you explicitly override them
`
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
  -- <command>               Real curl command or equivalent HTTP client command

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
`
}

func statusHelpText() string {
	return `proctor status - show contract coverage for the active run

Usage:
  proctor status

This prints:
  - every scenario in the contract
  - whether browser evidence passes or fails
  - whether curl evidence passes or fails when required
  - any global gaps such as missing desktop or mobile screenshots

Use this after each record step so the agent can see what is still missing.
`
}

func doneHelpText() string {
	return `proctor done - enforce completion

Usage:
  proctor done

Passes only when:
  - every required scenario has valid evidence
  - browser scenarios have trusted browser evidence
  - browser evidence includes screenshots
  - required assertions pass
  - artifact hashes still match

If the contract is incomplete, proctor done exits non-zero and prints what is
still missing. This is the command the agent should treat as the real
definition of done.
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
The HTML report is always rendered in dark mode.
`
}
