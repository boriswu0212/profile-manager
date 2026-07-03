//go:build windows

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/boriswu0212/profile-manager/internal/config"
)

// TestMain doubles as the helper-process entry point: execProcess os.Exits
// in-process, so both sides of a launch have to run as child processes.
func TestMain(m *testing.M) {
	switch os.Getenv("PM_TEST_MODE") {
	case "signal-parent":
		helperSignalParent()
	case "signal-child":
		helperSignalChild()
	case "pm-run":
		helperPMRun()
	case "stub-claude":
		helperStubClaude()
	default:
		os.Exit(m.Run())
	}
}

// helperSignalParent replays the codex launch shape: a cleanup handler is
// registered before exec, then execProcess hands the console to a child.
func helperSignalParent() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper:", err)
		os.Exit(3)
	}
	setupSignalHandler(func() {
		_ = os.WriteFile(os.Getenv("PM_TEST_MARKER"), []byte("cleanup ran"), 0600)
	})
	os.Setenv("PM_TEST_MODE", "signal-child")
	if err := execProcess(self, []string{"child"}); err != nil {
		fmt.Fprintln(os.Stderr, "helper: execProcess:", err)
		os.Exit(3)
	}
}

// helperSignalChild stands in for a claude/codex TUI that handles Ctrl
// events itself and keeps running.
func helperSignalChild() {
	signal.Ignore(os.Interrupt)
	if err := os.WriteFile(os.Getenv("PM_TEST_READY"), []byte("up"), 0600); err != nil {
		fmt.Fprintln(os.Stderr, "helper:", err)
		os.Exit(3)
	}
	time.Sleep(3 * time.Second)
	os.Exit(7)
}

// helperPMRun performs a real anthropic-provider launch against whatever
// "claude" is on PATH (a stub installed by the test).
func helperPMRun() {
	cp, err := claudePath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper: claude lookup:", err)
		os.Exit(3)
	}
	os.Setenv("PM_TEST_MODE", "stub-claude")
	profile := &config.Profile{
		Name:     "e2e",
		Provider: config.ProviderAnthropic,
		APIKey:   "sk-test-literal",
		BaseURL:  "https://gw.example.invalid/v1",
		Model:    "claude-test-1",
	}
	err = runWithAPIKeyHelper(cp, profile, []string{"--extra", "value with space"})
	// execProcess exits the process on success; reaching here means failure.
	fmt.Fprintln(os.Stderr, "helper: runWithAPIKeyHelper returned:", err)
	os.Exit(3)
}

// helperStubClaude echoes what a launched claude would have received.
func helperStubClaude() {
	for _, a := range os.Args[1:] {
		fmt.Printf("ARG\t%s\n", a)
	}
	fmt.Printf("BASE\t%s\n", os.Getenv("ANTHROPIC_BASE_URL"))
	os.Exit(0)
}

// TestExecProcessIgnoresCtrlBreak locks the unix-exec signal semantics on
// Windows: once the child owns the session, a console ctrl event must not
// kill pm or fire the pre-exec cleanup handler — pm waits and forwards the
// child's exit code. CTRL_BREAK is used because, unlike CTRL_C, it can be
// sent to a single process group; the Go runtime maps both to os.Interrupt.
func TestExecProcessIgnoresCtrlBreak(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "cleanup-marker")
	ready := filepath.Join(tmp, "child-ready")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self)
	cmd.Env = append(os.Environ(),
		"PM_TEST_MODE=signal-parent",
		"PM_TEST_MARKER="+marker,
		"PM_TEST_READY="+ready,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	waitForFile(t, ready)
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
		t.Skipf("cannot send CTRL_BREAK (no console?): %v", err)
	}

	err = cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("pm should forward the child's exit code 7; got %v\noutput:\n%s", err, out.String())
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("pre-exec cleanup handler fired during the child session (marker written)")
	}
}

// TestLaunchThroughClaudeExe is the Windows port of the stub-claude E2E from
// .claude/rules/verify.md: launch through a stub claude.exe and assert argv
// and env arrived intact.
func TestLaunchThroughClaudeExe(t *testing.T) {
	args, base := launchThroughStub(t, false)

	if base != "https://gw.example.invalid" {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want the /v1-stripped base", base)
	}

	settings, ok := argAfter(args, "--settings")
	if !ok {
		t.Fatalf("--settings flag missing; args: %q", args)
	}
	var parsed struct {
		APIKeyHelper              string `json:"apiKeyHelper"`
		DisableClaudeAiConnectors bool   `json:"disableClaudeAiConnectors"`
	}
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("--settings JSON mangled: %v\nsettings: %q", err, settings)
	}
	if !strings.Contains(parsed.APIKeyHelper, "_resolve-key") || !strings.Contains(parsed.APIKeyHelper, "e2e") {
		t.Errorf("apiKeyHelper = %q, want a pm _resolve-key command for profile e2e", parsed.APIKeyHelper)
	}
	if !parsed.DisableClaudeAiConnectors {
		t.Errorf("disableClaudeAiConnectors lost from --settings: %q", settings)
	}

	if got, ok := argAfter(args, "--model"); !ok || got != "claude-test-1" {
		t.Errorf("--model = %q, want claude-test-1", got)
	}
	if got, ok := argAfter(args, "--extra"); !ok || got != "value with space" {
		t.Errorf("space-containing passthrough arg arrived as %q", got)
	}
}

// TestLaunchThroughClaudeCmdShim routes the same launch through a claude.cmd
// batch shim — the shape of an npm-installed claude. Go quotes argv for
// CommandLineToArgvW but cmd.exe unquotes with different rules (see the
// os/exec.Command docs), so whether the --settings JSON survives is
// empirical: the test asserts the launch itself works and *logs* what
// arrived, so a mangled-but-launchable shim shows up in CI logs rather than
// as a red build.
func TestLaunchThroughClaudeCmdShim(t *testing.T) {
	args, _ := launchThroughStub(t, true)
	t.Logf("args through claude.cmd shim: %q", args)

	settings, ok := argAfter(args, "--settings")
	if !ok {
		t.Log("NOTE: --settings flag did not survive the .cmd shim")
		return
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Logf("NOTE: --settings JSON did NOT survive the .cmd shim: %v (got %q)", err, settings)
	} else {
		t.Log("--settings JSON survived the .cmd shim intact")
	}
}

// launchThroughStub installs the test binary as a stub claude (directly as
// claude.exe, or behind a claude.cmd shim), runs the pm-run helper against a
// scratch HOME/PATH, and returns the argv and ANTHROPIC_BASE_URL the stub saw.
func launchThroughStub(t *testing.T, useCmdShim bool) ([]string, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	home := filepath.Join(tmp, "home")
	for _, d := range []string{binDir, home} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}

	if useCmdShim {
		copyFile(t, self, filepath.Join(binDir, "claude-stub.exe"))
		shim := "@echo off\r\n\"%~dp0claude-stub.exe\" %*\r\n"
		if err := os.WriteFile(filepath.Join(binDir, "claude.cmd"), []byte(shim), 0700); err != nil {
			t.Fatal(err)
		}
	} else {
		copyFile(t, self, filepath.Join(binDir, "claude.exe"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self)
	cmd.Env = append(os.Environ(),
		"PM_TEST_MODE=pm-run",
		"USERPROFILE="+home,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pm-run helper failed: %v\noutput:\n%s", err, out)
	}

	var args []string
	var base string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if v, ok := strings.CutPrefix(line, "ARG\t"); ok {
			args = append(args, v)
		}
		if v, ok := strings.CutPrefix(line, "BASE\t"); ok {
			base = v
		}
	}
	if len(args) == 0 {
		t.Fatalf("stub claude never ran; output:\n%s", out)
	}
	return args, base
}

func argAfter(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
