package main

import (
	"errors"
	"flag"
	"fmt"
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
	case "start":
		return runStart(store, cwd, args[1:])
	case "status":
		return runStatus(store, cwd)
	case "record":
		return runRecord(store, cwd, args[1:])
	case "done":
		return runDone(store, cwd)
	case "report":
		return runReport(store, cwd)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
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
	fmt.Printf("Run: %s\n", run.ID)
	fmt.Printf("Feature: %s\n", run.Feature)
	for _, item := range eval.ScenarioEvaluations {
		fmt.Printf("- %s (%s)\n", item.Scenario.Label, item.Scenario.ID)
		if item.Scenario.CurlRequired {
			if len(item.Scenario.CurlEndpoints) > 0 {
				fmt.Printf("  curl contract: %s\n", strings.Join(item.Scenario.CurlEndpoints, "; "))
			}
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
