package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nclandrei/proctor/internal/proctor"
)

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
	opts := proctor.StartOptions{}
	fs.StringVar(&opts.Feature, "feature", "", "")
	fs.StringVar(&opts.BrowserURL, "url", "", "")
	fs.StringVar(&opts.CurlMode, "curl", "", "")
	fs.Var(&endpoints, "curl-endpoint", "")
	fs.StringVar(&opts.CurlSkipReason, "curl-skip-reason", "", "")
	fs.StringVar(&opts.HappyPath, "happy-path", "", "")
	fs.StringVar(&opts.FailurePath, "failure-path", "", "")
	fs.Var(&edgeCases, "edge-case", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts.CurlEndpoints = endpoints
	opts.EdgeCaseInputs = edgeCases
	if err := fillStartPrompts(os.Stdin, os.Stdout, &opts); err != nil {
		return err
	}
	run, err := proctor.CreateRun(store, cwd, opts)
	if err != nil {
		return err
	}
	fmt.Printf("Created run %s\n", run.ID)
	fmt.Printf("Run directory: %s\n", store.RunDir(run))
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
		if item.Scenario.BrowserRequired {
			if item.BrowserOK {
				fmt.Println("  browser: pass")
			} else {
				fmt.Printf("  browser: fail (%s)\n", strings.Join(item.BrowserIssues, ", "))
			}
		}
		if item.Scenario.CurlRequired {
			if item.CurlOK {
				fmt.Println("  curl: pass")
			} else {
				fmt.Printf("  curl: fail (%s)\n", strings.Join(item.CurlIssues, ", "))
			}
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
	}
	return nil
}

func runRecord(store *proctor.Store, cwd string, args []string) error {
	if len(args) == 0 {
		return errors.New("record requires a surface: browser or curl")
	}
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	switch args[0] {
	case "browser":
		return runRecordBrowser(store, run, args[1:])
	case "curl":
		return runRecordCurl(store, run, args[1:])
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
	opts := proctor.BrowserRecordOptions{}
	fs.StringVar(&opts.ScenarioID, "scenario", "", "")
	fs.StringVar(&opts.SessionID, "session", "", "")
	fs.StringVar(&opts.Tool, "tool", "agent-browser", "")
	fs.StringVar(&opts.ReportPath, "report", "", "")
	fs.Var(&screenshots, "screenshot", "")
	fs.Var(&passAssertions, "assert", "")
	fs.Var(&failAssertions, "fail-assert", "")
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
	if err := proctor.RecordBrowser(store, run, opts); err != nil {
		return err
	}
	fmt.Printf("Recorded browser evidence for %s\n", opts.ScenarioID)
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
		if item.Scenario.BrowserRequired && !item.BrowserOK {
			fmt.Printf("- %s: %s\n", item.Scenario.ID, strings.Join(item.BrowserIssues, ", "))
		}
		if item.Scenario.CurlRequired && !item.CurlOK {
			fmt.Printf("- %s: %s\n", item.Scenario.ID, strings.Join(item.CurlIssues, ", "))
		}
	}
	for _, item := range eval.GlobalMissing {
		fmt.Printf("- %s\n", item)
	}
	return errors.New("verification contract incomplete")
}

func runReport(store *proctor.Store, cwd string) error {
	run, err := store.LoadRun(proctor.RepoRoot(cwd))
	if err != nil {
		return err
	}
	fmt.Printf("Contract: %s\n", filepath.Join(store.RunDir(run), "contract.md"))
	fmt.Printf("HTML report: %s\n", filepath.Join(store.RunDir(run), "report.html"))
	return nil
}

func fillStartPrompts(in *os.File, out *os.File, opts *proctor.StartOptions) error {
	reader := bufio.NewReader(in)
	var err error
	if strings.TrimSpace(opts.Feature) == "" {
		if opts.Feature, err = prompt(reader, out, "Feature / flow name"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(opts.BrowserURL) == "" {
		if opts.BrowserURL, err = prompt(reader, out, "Browser URL"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(opts.CurlMode) == "" {
		if opts.CurlMode, err = prompt(reader, out, "Direct HTTP verification? (required/skip)"); err != nil {
			return err
		}
	}
	if strings.EqualFold(opts.CurlMode, "required") && len(opts.CurlEndpoints) == 0 {
		value, err := prompt(reader, out, "curl endpoint(s), separated by ';'")
		if err != nil {
			return err
		}
		opts.CurlEndpoints = splitSemicolonList(value)
	}
	if strings.EqualFold(opts.CurlMode, "skip") && strings.TrimSpace(opts.CurlSkipReason) == "" {
		if opts.CurlSkipReason, err = prompt(reader, out, "Reason for skipping direct HTTP verification"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(opts.HappyPath) == "" {
		if opts.HappyPath, err = prompt(reader, out, "Happy path"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(opts.FailurePath) == "" {
		if opts.FailurePath, err = prompt(reader, out, "Main failure path"); err != nil {
			return err
		}
	}
	if len(opts.EdgeCaseInputs) == 0 {
		for _, category := range proctor.EdgeCaseCategories {
			answer, err := prompt(reader, out, fmt.Sprintf("%s (scenario(s) separated by ';' or N/A: reason)", category))
			if err != nil {
				return err
			}
			opts.EdgeCaseInputs = append(opts.EdgeCaseInputs, category+"="+answer)
		}
	}
	return nil
}

func prompt(reader *bufio.Reader, out *os.File, label string) (string, error) {
	fmt.Fprintf(out, "%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
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
