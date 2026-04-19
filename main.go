package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nclandrei/proctor/internal/proctor"
)

var version = "dev"

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		fmt.Printf("proctor %s\n", version)
		return nil
	}

	if text, ok, err := commandHelp(args); err != nil {
		return err
	} else if ok {
		fmt.Print(text)
		return nil
	}

	store, err := proctor.NewStore()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	switch args[0] {
	case "init":
		return runInit(store, cwd, args[1:])
	case "start":
		return runStart(store, cwd, args[1:])
	case "status":
		return runStatus(store, cwd)
	case "note":
		return runNote(store, cwd, args[1:])
	case "record":
		return runRecord(store, cwd, args[1:])
	case "log":
		return runLog(store, cwd, args[1:])
	case "verify":
		return runVerify(store, cwd, args[1:])
	case "done":
		return runDone(store, cwd)
	case "report":
		return runReport(store, cwd)
	case "project":
		return runProject(store, cwd, args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func runProject(store *proctor.Store, cwd string, args []string) error {
	if len(args) == 0 {
		return errors.New("project requires a subcommand: show, get, set")
	}
	repoRoot := proctor.RepoRoot(cwd)
	slug, err := proctor.RepoSlug(repoRoot)
	if err != nil {
		return err
	}
	switch args[0] {
	case "show":
		p, err := proctor.LoadProfile(store, slug)
		if err != nil {
			return err
		}
		return printProfile(os.Stdout, store, p)
	case "get":
		if len(args) < 2 {
			return errors.New("project get requires <field>")
		}
		p, err := proctor.LoadProfile(store, slug)
		if err != nil {
			return err
		}
		val, err := p.FieldValue(args[1])
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	default:
		return fmt.Errorf("unknown project subcommand: %s", args[0])
	}
}

func runStart(store *proctor.Store, cwd string, args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var endpoints stringList
	var edgeCases stringList
	var force bool
	opts := proctor.StartOptions{}
	fs.StringVar(&opts.Platform, "platform", proctor.PlatformWeb, "")
	fs.StringVar(&opts.Feature, "feature", "", "")
	fs.StringVar(&opts.BrowserURL, "url", "", "")
	fs.StringVar(&opts.CLICommand, "cli-command", "", "")
	fs.StringVar(&opts.IOSScheme, "ios-scheme", "", "")
	fs.StringVar(&opts.IOSBundleID, "ios-bundle-id", "", "")
	fs.StringVar(&opts.IOSSimulator, "ios-simulator", "", "")
	fs.StringVar(&opts.DesktopAppName, "app-name", "", "")
	fs.StringVar(&opts.DesktopBundleID, "app-bundle-id", "", "")
	fs.StringVar(&opts.CurlMode, "curl", "", "")
	fs.Var(&endpoints, "curl-endpoint", "")
	fs.StringVar(&opts.CurlSkipReason, "curl-skip-reason", "", "")
	fs.StringVar(&opts.HappyPath, "happy-path", "", "")
	fs.StringVar(&opts.FailurePath, "failure-path", "", "")
	fs.Var(&edgeCases, "edge-case", "")
	fs.BoolVar(&force, "force", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts.CurlEndpoints = endpoints
	opts.EdgeCaseInputs = edgeCases
	if err := validateStartFlags(&opts); err != nil {
		return err
	}
	if !force {
		if _, err := store.LoadRun(proctor.RepoRoot(cwd)); err == nil {
			return errors.New("active run already exists for this repo; use --force to replace it")
		}
	}
	run, err := proctor.CreateRun(store, cwd, opts)
	if err != nil {
		return err
	}
	fmt.Printf("Created run %s\n", run.ID)
	fmt.Printf("Run directory: %s\n", store.RunDir(run))
	printRunRecommendations(os.Stdout, "Recommended next step:", run, nil)
	return nil
}

func runStatus(store *proctor.Store, cwd string) error {
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	eval, err := proctor.Evaluate(store, run)
	if err != nil {
		return err
	}
	preNotes, err := store.LoadPreNotes(run)
	if err != nil {
		return err
	}
	preNoteScenarios := map[string]bool{}
	for _, note := range preNotes {
		preNoteScenarios[note.Scenario] = true
	}
	fmt.Printf("Run: %s\n", run.ID)
	fmt.Printf("Feature: %s\n", run.Feature)
	for _, item := range eval.ScenarioEvaluations {
		fmt.Printf("- %s (%s)\n", item.Scenario.Label, item.Scenario.ID)
		if preNoteScenarios[item.Scenario.ID] {
			fmt.Printf("  pre-note: filed\n")
		} else {
			fmt.Printf("  pre-note: missing (run proctor note --scenario %s --session <session> --notes '...')\n", item.Scenario.ID)
		}
		if item.Scenario.CurlRequired {
			if len(item.Scenario.CurlEndpoints) > 0 {
				fmt.Printf("  curl contract: %s\n", strings.Join(item.Scenario.CurlEndpoints, "; "))
			}
		}
		if item.LogOK {
			fmt.Printf("  log: pass\n")
		} else {
			fmt.Printf("  log: missing (run proctor log --scenario %s --session <session> --surface <surface> --screenshot <path> --action '...' --observation '...' --comparison '...')\n", item.Scenario.ID)
		}
		for _, surface := range item.Scenario.RequiredSurfaces() {
			ok, _ := item.SurfaceStatus(surface)
			if ok {
				fmt.Printf("  %s: pass\n", surface)
				continue
			}
			fmt.Printf("  %s: fail (%s)\n", surface, strings.Join(item.SurfaceIssues(surface), ", "))
		}
	}
	if len(eval.GlobalMissing) > 0 {
		fmt.Println("Global gaps:")
		for _, item := range eval.GlobalMissing {
			fmt.Printf("- %s\n", item)
		}
	}
	if eval.Complete {
		fmt.Println("Status: complete")
	} else {
		fmt.Println("Status: incomplete")
		printRunRecommendations(os.Stdout, "Suggested capture workflows:", run, &eval)
	}
	return nil
}

func runNote(store *proctor.Store, cwd string, args []string) error {
	fs := flag.NewFlagSet("note", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var scenario, session, notes string
	fs.StringVar(&scenario, "scenario", "", "")
	fs.StringVar(&session, "session", "", "")
	fs.StringVar(&notes, "notes", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var missing []string
	if strings.TrimSpace(scenario) == "" {
		missing = append(missing, "--scenario")
	}
	if strings.TrimSpace(session) == "" {
		missing = append(missing, "--session")
	}
	if strings.TrimSpace(notes) == "" {
		missing = append(missing, "--notes")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	note, err := proctor.FilePreNote(store, run, scenario, session, notes)
	if err != nil {
		return err
	}
	fmt.Printf("Filed pre-test note %s for scenario %s session %s\n", note.ID, note.Scenario, note.Session)
	return nil
}

func runRecord(store *proctor.Store, cwd string, args []string) error {
	if len(args) == 0 {
		return errors.New("record requires a surface: browser, ios, curl, cli, or desktop")
	}
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	switch args[0] {
	case "browser":
		return runRecordBrowser(store, run, args[1:])
	case "ios":
		return runRecordIOS(store, run, args[1:])
	case "curl":
		return runRecordCurl(store, run, args[1:])
	case "cli":
		return runRecordCLI(store, run, args[1:])
	case "desktop":
		return runRecordDesktop(store, run, args[1:])
	default:
		return fmt.Errorf("unknown record surface: %s", args[0])
	}
}

func runRecordBrowser(store *proctor.Store, run proctor.Run, args []string) error {
	fs := flag.NewFlagSet("record browser", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var screenshots stringList
	var passAssertions stringList
	var failAssertions stringList
	var maxScreenshotAge time.Duration
	opts := proctor.BrowserRecordOptions{}
	fs.StringVar(&opts.ScenarioID, "scenario", "", "")
	fs.StringVar(&opts.SessionID, "session", "", "")
	fs.StringVar(&opts.Tool, "tool", "agent-browser", "")
	fs.StringVar(&opts.ReportPath, "report", "", "")
	fs.Var(&screenshots, "screenshot", "")
	fs.Var(&passAssertions, "assert", "")
	fs.Var(&failAssertions, "fail-assert", "")
	fs.DurationVar(&maxScreenshotAge, "max-screenshot-age", proctor.DefaultMaxScreenshotAge, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts.Screenshots = map[string]string{}
	for _, item := range screenshots {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			return fmt.Errorf("invalid screenshot format: %s", item)
		}
		opts.Screenshots[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	opts.PassAssertions = passAssertions
	opts.FailAssertions = failAssertions
	opts.MaxScreenshotAge = maxScreenshotAge
	if err := proctor.RecordBrowser(store, run, opts); err != nil {
		return err
	}
	fmt.Printf("Recorded browser evidence for %s\n", opts.ScenarioID)
	return nil
}

func runRecordIOS(store *proctor.Store, run proctor.Run, args []string) error {
	fs := flag.NewFlagSet("record ios", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var screenshots stringList
	var passAssertions stringList
	var failAssertions stringList
	var maxScreenshotAge time.Duration
	opts := proctor.IOSRecordOptions{}
	fs.StringVar(&opts.ScenarioID, "scenario", "", "")
	fs.StringVar(&opts.SessionID, "session", "", "")
	fs.StringVar(&opts.Tool, "tool", "ios-simulator", "")
	fs.StringVar(&opts.ReportPath, "report", "", "")
	fs.Var(&screenshots, "screenshot", "")
	fs.Var(&passAssertions, "assert", "")
	fs.Var(&failAssertions, "fail-assert", "")
	fs.DurationVar(&maxScreenshotAge, "max-screenshot-age", proctor.DefaultMaxScreenshotAge, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts.Screenshots = map[string]string{}
	for _, item := range screenshots {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			return fmt.Errorf("invalid screenshot format: %s", item)
		}
		opts.Screenshots[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	opts.PassAssertions = passAssertions
	opts.FailAssertions = failAssertions
	opts.MaxScreenshotAge = maxScreenshotAge
	if err := proctor.RecordIOS(store, run, opts); err != nil {
		return err
	}
	fmt.Printf("Recorded ios evidence for %s\n", opts.ScenarioID)
	return nil
}

func runRecordCurl(store *proctor.Store, run proctor.Run, args []string) error {
	flagArgs, command := splitArgsAtDoubleDash(args)
	fs := flag.NewFlagSet("record curl", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var passAssertions stringList
	var failAssertions stringList
	opts := proctor.CurlRecordOptions{}
	fs.StringVar(&opts.ScenarioID, "scenario", "", "")
	fs.Var(&passAssertions, "assert", "")
	fs.Var(&failAssertions, "fail-assert", "")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	opts.PassAssertions = passAssertions
	opts.FailAssertions = failAssertions
	opts.Command = command
	if err := proctor.RecordCurl(store, run, opts); err != nil {
		return err
	}
	fmt.Printf("Recorded curl evidence for %s\n", opts.ScenarioID)
	return nil
}

func runRecordCLI(store *proctor.Store, run proctor.Run, args []string) error {
	fs := flag.NewFlagSet("record cli", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var screenshots stringList
	var passAssertions stringList
	var failAssertions stringList
	var exitCodeText string
	var maxScreenshotAge time.Duration
	opts := proctor.CLIRecordOptions{}
	fs.StringVar(&opts.ScenarioID, "scenario", "", "")
	fs.StringVar(&opts.SessionID, "session", "", "")
	fs.StringVar(&opts.Tool, "tool", "terminal-session", "")
	fs.StringVar(&opts.Command, "command", "", "")
	fs.StringVar(&opts.TranscriptPath, "transcript", "", "")
	fs.StringVar(&exitCodeText, "exit-code", "", "")
	fs.Var(&screenshots, "screenshot", "")
	fs.Var(&passAssertions, "assert", "")
	fs.Var(&failAssertions, "fail-assert", "")
	fs.DurationVar(&maxScreenshotAge, "max-screenshot-age", proctor.DefaultMaxScreenshotAge, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(exitCodeText) != "" {
		value, err := strconv.Atoi(strings.TrimSpace(exitCodeText))
		if err != nil {
			return fmt.Errorf("invalid --exit-code: %w", err)
		}
		opts.ExitCode = &value
	}
	opts.Screenshots = map[string]string{}
	for _, item := range screenshots {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			return fmt.Errorf("invalid screenshot format: %s", item)
		}
		opts.Screenshots[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	opts.PassAssertions = passAssertions
	opts.FailAssertions = failAssertions
	opts.MaxScreenshotAge = maxScreenshotAge
	if err := proctor.RecordCLI(store, run, opts); err != nil {
		return err
	}
	fmt.Printf("Recorded cli evidence for %s\n", opts.ScenarioID)
	return nil
}

func runRecordDesktop(store *proctor.Store, run proctor.Run, args []string) error {
	fs := flag.NewFlagSet("record desktop", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var screenshots stringList
	var passAssertions stringList
	var failAssertions stringList
	var maxScreenshotAge time.Duration
	opts := proctor.DesktopRecordOptions{}
	fs.StringVar(&opts.ScenarioID, "scenario", "", "")
	fs.StringVar(&opts.SessionID, "session", "", "")
	fs.StringVar(&opts.Tool, "tool", "peekaboo", "")
	fs.StringVar(&opts.ReportPath, "report", "", "")
	fs.Var(&screenshots, "screenshot", "")
	fs.Var(&passAssertions, "assert", "")
	fs.Var(&failAssertions, "fail-assert", "")
	fs.DurationVar(&maxScreenshotAge, "max-screenshot-age", proctor.DefaultMaxScreenshotAge, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts.Screenshots = map[string]string{}
	for _, item := range screenshots {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			return fmt.Errorf("invalid screenshot format: %s", item)
		}
		opts.Screenshots[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	opts.PassAssertions = passAssertions
	opts.FailAssertions = failAssertions
	opts.MaxScreenshotAge = maxScreenshotAge
	if err := proctor.RecordDesktop(store, run, opts); err != nil {
		return err
	}
	fmt.Printf("Recorded desktop evidence for %s\n", opts.ScenarioID)
	return nil
}

func runVerify(store *proctor.Store, cwd string, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var scenario, session, verification string
	fs.StringVar(&scenario, "scenario", "", "")
	fs.StringVar(&session, "session", "", "")
	fs.StringVar(&verification, "verification", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var missing []string
	if strings.TrimSpace(scenario) == "" {
		missing = append(missing, "--scenario")
	}
	if strings.TrimSpace(session) == "" {
		missing = append(missing, "--session")
	}
	if strings.TrimSpace(verification) == "" {
		missing = append(missing, "--verification")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	if err := proctor.VerifyEvidence(store, run, scenario, session, verification); err != nil {
		return err
	}
	fmt.Printf("Verified scenario %s\n", scenario)
	return nil
}

func runLog(store *proctor.Store, cwd string, args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var scenario, session, surface, screenshot, action, observation, comparison string
	fs.StringVar(&scenario, "scenario", "", "")
	fs.StringVar(&session, "session", "", "")
	fs.StringVar(&surface, "surface", "", "")
	fs.StringVar(&screenshot, "screenshot", "", "")
	fs.StringVar(&action, "action", "", "")
	fs.StringVar(&observation, "observation", "", "")
	fs.StringVar(&comparison, "comparison", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var missing []string
	if strings.TrimSpace(scenario) == "" {
		missing = append(missing, "--scenario")
	}
	if strings.TrimSpace(session) == "" {
		missing = append(missing, "--session")
	}
	if strings.TrimSpace(surface) == "" {
		missing = append(missing, "--surface")
	}
	if strings.TrimSpace(screenshot) == "" {
		missing = append(missing, "--screenshot")
	}
	if strings.TrimSpace(action) == "" {
		missing = append(missing, "--action")
	}
	if strings.TrimSpace(observation) == "" {
		missing = append(missing, "--observation")
	}
	if strings.TrimSpace(comparison) == "" {
		missing = append(missing, "--comparison")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	entry, err := proctor.LogStep(store, run, proctor.LogStepOptions{
		ScenarioID:     scenario,
		SessionID:      session,
		Surface:        surface,
		ScreenshotPath: screenshot,
		Action:         action,
		Observation:    observation,
		Comparison:     comparison,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Logged step %d for scenario %s session %s\n", entry.Step, entry.ScenarioID, entry.SessionID)
	fmt.Printf("  action:      %s\n", truncate(entry.Action, 80))
	fmt.Printf("  observation: %s\n", truncate(entry.Observation, 80))
	fmt.Printf("  comparison:  %s\n", truncate(entry.Comparison, 80))
	fmt.Printf("  screenshot:  %s\n", entry.ScreenshotPath)
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func runDone(store *proctor.Store, cwd string) error {
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	eval, err := proctor.CompleteRun(store, run)
	if err != nil {
		return err
	}
	if eval.Complete {
		fmt.Printf("PASS\nReport: %s\n", filepath.Join(store.RunDir(run), "report.html"))
		return nil
	}
	fmt.Println("FAIL")
	for _, item := range eval.ScenarioEvaluations {
		if !item.LogOK {
			fmt.Printf("- %s (log): %s\n", item.Scenario.ID, strings.Join(item.LogIssues, ", "))
		}
		for _, surface := range item.Scenario.RequiredSurfaces() {
			ok, _ := item.SurfaceStatus(surface)
			if ok {
				continue
			}
			fmt.Printf("- %s (%s): %s\n", item.Scenario.ID, surface, strings.Join(item.SurfaceIssues(surface), ", "))
		}
	}
	for _, item := range eval.GlobalMissing {
		fmt.Printf("- %s\n", item)
	}
	printRunRecommendations(os.Stdout, "Suggested capture workflows:", run, &eval)
	return errors.New("verification contract incomplete")
}

func runReport(store *proctor.Store, cwd string) error {
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	if err := proctor.WriteReports(store, run); err != nil {
		return err
	}
	fmt.Printf("Contract: %s\n", filepath.Join(store.RunDir(run), "contract.md"))
	fmt.Printf("HTML report: %s\n", filepath.Join(store.RunDir(run), "report.html"))
	return nil
}

func validateStartFlags(opts *proctor.StartOptions) error {
	platform := strings.TrimSpace(opts.Platform)
	if platform == "" {
		platform = proctor.PlatformWeb
	}
	opts.Platform = platform

	var missing []string
	if strings.TrimSpace(opts.Feature) == "" {
		missing = append(missing, "--feature")
	}
	switch proctor.NormalizePlatform(platform) {
	case proctor.PlatformIOS:
		if strings.TrimSpace(opts.IOSScheme) == "" {
			missing = append(missing, "--ios-scheme")
		}
		if strings.TrimSpace(opts.IOSBundleID) == "" {
			missing = append(missing, "--ios-bundle-id")
		}
	case proctor.PlatformCLI:
		if strings.TrimSpace(opts.CLICommand) == "" {
			missing = append(missing, "--cli-command")
		}
	case proctor.PlatformDesktop:
		if strings.TrimSpace(opts.DesktopAppName) == "" {
			missing = append(missing, "--app-name")
		}
	default:
		if strings.TrimSpace(opts.BrowserURL) == "" {
			missing = append(missing, "--url")
		}
	}
	if strings.TrimSpace(opts.HappyPath) == "" {
		missing = append(missing, "--happy-path")
	}
	if strings.TrimSpace(opts.FailurePath) == "" {
		missing = append(missing, "--failure-path")
	}
	if len(opts.EdgeCaseInputs) == 0 {
		missing = append(missing, "--edge-case")
	}
	normalizedPlatform := proctor.NormalizePlatform(platform)
	if normalizedPlatform != proctor.PlatformCLI {
		if strings.TrimSpace(opts.CurlMode) == "" {
			missing = append(missing, "--curl")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}
	return nil
}

func splitSemicolonList(value string) []string {
	var items []string
	for _, part := range strings.Split(value, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func splitPipeList(value string) []string {
	var items []string
	for _, part := range strings.Split(value, "|") {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func splitArgsAtDoubleDash(args []string) ([]string, []string) {
	for idx, value := range args {
		if value == "--" {
			return args[:idx], args[idx+1:]
		}
	}
	return args, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func runInit(store *proctor.Store, cwd string, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	var (
		platform, url, authURL, testEmail, testPassword string
		iosScheme, iosBundle, iosSim                    string
		appName, appBundle                              string
		cliCommand                                      string
		loginTTL                                        string
		forceDetect                                     bool
	)
	fs.StringVar(&platform, "platform", "", "")
	fs.StringVar(&url, "url", "", "")
	fs.StringVar(&authURL, "auth-url", "", "")
	fs.StringVar(&testEmail, "test-email", "", "")
	fs.StringVar(&testPassword, "test-password", "", "")
	fs.StringVar(&iosScheme, "ios-scheme", "", "")
	fs.StringVar(&iosBundle, "ios-bundle-id", "", "")
	fs.StringVar(&iosSim, "ios-simulator", "", "")
	fs.StringVar(&appName, "app-name", "", "")
	fs.StringVar(&appBundle, "app-bundle-id", "", "")
	fs.StringVar(&cliCommand, "cli-command", "", "")
	fs.StringVar(&loginTTL, "login-ttl", "", "")
	fs.BoolVar(&forceDetect, "force-detect", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	repoRoot := proctor.RepoRoot(cwd)
	slug, err := proctor.RepoSlug(repoRoot)
	if err != nil {
		return err
	}

	// Load existing profile if any; otherwise detect.
	p, err := proctor.LoadProfile(store, slug)
	switch {
	case err == nil && forceDetect:
		detected, _ := proctor.DetectProfile(repoRoot)
		p = mergeProfile(detected, p)
	case err == nil:
		// existing profile becomes base; no detection
	case os.IsNotExist(err):
		detected, _ := proctor.DetectProfile(repoRoot)
		p = detected
	default:
		return err
	}

	if platform != "" {
		p.Platform = platform
	}
	switch p.Platform {
	case proctor.PlatformWeb:
		if p.Web == nil {
			p.Web = &proctor.WebProfile{}
		}
		if url != "" {
			p.Web.DevURL = url
		}
		if authURL != "" {
			p.Web.AuthURL = authURL
		}
		if testEmail != "" {
			p.Web.TestEmail = testEmail
		}
		if testPassword != "" {
			p.Web.TestPassword = testPassword
		}
		if loginTTL != "" {
			if p.Web.Login == nil {
				p.Web.Login = &proctor.LoginConfig{File: "session.json"}
			}
			p.Web.Login.TTL = loginTTL
		}
	case proctor.PlatformIOS:
		if p.IOS == nil {
			p.IOS = &proctor.IOSProfile{}
		}
		if iosScheme != "" {
			p.IOS.Scheme = iosScheme
		}
		if iosBundle != "" {
			p.IOS.BundleID = iosBundle
		}
		if iosSim != "" {
			p.IOS.Simulator = iosSim
		}
	case proctor.PlatformDesktop:
		if p.Desktop == nil {
			p.Desktop = &proctor.DesktopProfile{}
		}
		if appName != "" {
			p.Desktop.AppName = appName
		}
		if appBundle != "" {
			p.Desktop.BundleID = appBundle
		}
	case proctor.PlatformCLI:
		if p.CLI == nil {
			p.CLI = &proctor.CLIProfile{}
		}
		if cliCommand != "" {
			p.CLI.Command = cliCommand
		}
	}

	if err := proctor.SaveProfile(store, slug, p); err != nil {
		return err
	}
	loaded, _ := proctor.LoadProfile(store, slug)
	return printProfile(os.Stdout, store, loaded)
}

// mergeProfile returns base with empty fields filled from extra.
func mergeProfile(extra, base proctor.Profile) proctor.Profile {
	if base.Platform == "" {
		base.Platform = extra.Platform
	}
	if extra.Web != nil {
		if base.Web == nil {
			base.Web = &proctor.WebProfile{}
		}
		if base.Web.DevURL == "" {
			base.Web.DevURL = extra.Web.DevURL
		}
	}
	return base
}

// printProfile formats the redacted profile with freshness. Used by both init
// and `project show`.
func printProfile(w io.Writer, store *proctor.Store, p proctor.Profile) error {
	r := p.Redacted()
	data, err := json.MarshalIndent(&r, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(data))
	if p.Platform == proctor.PlatformWeb {
		state := proctor.LoginStateForProfile(store, p)
		fmt.Fprintf(w, "login state: %s", state.Kind)
		if state.Kind == proctor.LoginFresh || state.Kind == proctor.LoginStale {
			fmt.Fprintf(w, " (age %s, ttl %s)", roundDuration(state.Age), state.TTL)
		}
		fmt.Fprintln(w)
	}
	if p.Incomplete {
		fmt.Fprintln(w, "incomplete — missing:")
		for _, f := range p.MissingFieldsList {
			fmt.Fprintf(w, "  - %s\n", f)
		}
	}
	return nil
}

func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Second)
}
