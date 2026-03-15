package proctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreateRunWritesExpectedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	run, err := CreateRun(store, repo, StartOptions{
		Feature:        "auth flow",
		BrowserURL:     "http://127.0.0.1:3000/login",
		CurlMode:       "required",
		CurlEndpoints:  []string{"POST /api/login"},
		HappyPath:      "valid login goes to dashboard",
		FailurePath:    "invalid password shows error",
		EdgeCaseInputs: []string{"validation and malformed input=bad email shows validation", "empty or missing input=n/a: covered by validation", "retry or double-submit=double submit is ignored", "loading, latency, and race conditions=slow response keeps button disabled", "network or server failure=500 shows generic error", "auth and session state=existing session redirects away", "refresh, back-navigation, and state persistence=refresh keeps session", "mobile or responsive behavior=layout remains usable on mobile", "accessibility and keyboard behavior=enter submits from password field", "any feature-specific risks=remember me extends session"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(store.RunDir(run), "run.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.RunDir(run), "contract.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.RunDir(run), "report.html")); err != nil {
		t.Fatal(err)
	}
}

func TestDonePassesWhenRequiredEvidenceExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, StartOptions{
		Feature:        "auth flow",
		BrowserURL:     "http://127.0.0.1:3000/login",
		CurlMode:       "required",
		CurlEndpoints:  []string{"POST /api/login"},
		HappyPath:      "valid login goes to dashboard",
		FailurePath:    "invalid password shows error",
		EdgeCaseInputs: []string{"validation and malformed input=bad email shows validation", "empty or missing input=n/a: covered elsewhere", "retry or double-submit=double submit is ignored", "loading, latency, and race conditions=n/a: not relevant", "network or server failure=n/a: not relevant", "auth and session state=n/a: not relevant", "refresh, back-navigation, and state persistence=n/a: not relevant", "mobile or responsive behavior=layout remains usable on mobile", "accessibility and keyboard behavior=n/a: not relevant", "any feature-specific risks=n/a: not relevant"},
	})
	if err != nil {
		t.Fatal(err)
	}

	desktopShot := writeFixture(t, repo, "desktop.png", "desktop-image")
	mobileShot := writeFixture(t, repo, "mobile.png", "mobile-image")
	report := writeFixture(t, repo, "report.json", `{"failedRequests":[]}`)

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "happy-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-success": desktopShot,
			"mobile-success":  mobileShot,
		},
		PassAssertions: []string{"dashboard visible"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID: "failure-path",
		SessionID:  "browser-1",
		ReportPath: report,
		Screenshots: map[string]string{
			"desktop-failure": desktopShot,
		},
		PassAssertions: []string{"error visible"},
	}); err != nil {
		t.Fatal(err)
	}

	for _, scenario := range run.Scenarios {
		if scenario.Kind != "edge-case" {
			continue
		}
		if err := RecordBrowser(store, run, BrowserRecordOptions{
			ScenarioID: scenario.ID,
			SessionID:  "browser-1",
			ReportPath: report,
			Screenshots: map[string]string{
				"desktop-edge": desktopShot,
			},
			PassAssertions: []string{"edge case covered"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID:     "happy-path",
		Command:        []string{"/bin/sh", "-lc", "printf 'HTTP/1.1 200 OK\\n\\npass'"},
		PassAssertions: []string{"status 200"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := RecordCurl(store, run, CurlRecordOptions{
		ScenarioID:     "failure-path",
		Command:        []string{"/bin/sh", "-lc", "printf 'HTTP/1.1 401 Unauthorized\\n\\nfail'"},
		PassAssertions: []string{"status 401"},
	}); err != nil {
		t.Fatal(err)
	}

	eval, err := CompleteRun(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if !eval.Complete {
		t.Fatalf("expected run to pass, got %#v", eval)
	}
}

func initGitRepo(t *testing.T, repo, remote string) {
	t.Helper()
	mustRun(t, repo, "git", "init")
	mustRun(t, repo, "git", "config", "user.email", "test@example.com")
	mustRun(t, repo, "git", "config", "user.name", "Test User")
	mustRun(t, repo, "git", "remote", "add", "origin", remote)
}

func mustRun(t *testing.T, dir string, command string, args ...string) {
	t.Helper()
	if out, err := execCommand(dir, command, args...); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", command, args, err, out)
	}
}

func execCommand(dir string, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
