package proctor

import (
	"strings"
	"testing"
)

func TestRecordBrowserRejectsCrossPlatformEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleCLIStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://localhost:3000", 0, 0, 0, 0))
	screenshot := writeFixture(t, repo, "desktop.png", "image")

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": screenshot},
		PassAssertions: []string{"console_errors = 0"},
	})
	if err == nil {
		t.Fatal("expected error recording browser evidence on a cli platform run, got nil")
	}
	if !strings.Contains(err.Error(), "not valid") || !strings.Contains(err.Error(), "cli") {
		t.Fatalf("expected error mentioning surface not valid for cli platform, got: %s", err.Error())
	}
}

func TestRecordIOSRejectsCrossPlatformEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	report := writeFixture(t, repo, "ios-report.json", `{
  "app": {"bundleId": "com.test.app", "screen": "main", "state": "running"},
  "issues": {"launchErrors": 0, "crashes": 0, "fatalLogs": 0}
}`)
	screenshot := writeFixture(t, repo, "ios.png", "image")

	err = RecordIOS(store, run, IOSRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "ios-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"main": screenshot},
		PassAssertions: []string{"bundle_id = com.test.app"},
	})
	if err == nil {
		t.Fatal("expected error recording ios evidence on a web platform run, got nil")
	}
	if !strings.Contains(err.Error(), "not valid") || !strings.Contains(err.Error(), "web") {
		t.Fatalf("expected error mentioning surface not valid for web platform, got: %s", err.Error())
	}
}

func TestRecordBrowserFailsAtRecordTimeWhenAssertionsFail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PROCTOR_HOME", home)
	repo := t.TempDir()
	initGitRepo(t, repo, "https://github.com/nclandrei/proctor-test")

	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	run, err := CreateRun(store, repo, sampleStartOptions())
	if err != nil {
		t.Fatal(err)
	}

	// Report has 5 console errors — the assertion "console_errors = 0" should fail.
	report := writeFixture(t, repo, "report.json", sampleBrowserReport("http://127.0.0.1:3000/login", 5, 0, 0, 0))
	screenshot := writeFixture(t, repo, "desktop.png", "image")

	err = RecordBrowser(store, run, BrowserRecordOptions{
		ScenarioID:     "happy-path",
		SessionID:      "browser-1",
		ReportPath:     report,
		Screenshots:    map[string]string{"desktop": screenshot},
		PassAssertions: []string{"console_errors = 0"},
	})
	if err == nil {
		t.Fatal("expected error when assertions fail at record time, got nil")
	}
	if !strings.Contains(err.Error(), "assertion failed") {
		t.Fatalf("expected assertion failure error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "console_errors") {
		t.Fatalf("expected error to mention which assertion failed, got: %s", err.Error())
	}
}
