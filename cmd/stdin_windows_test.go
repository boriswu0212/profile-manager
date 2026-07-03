//go:build windows

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procWriteConsoleInput = kernel32.NewProc("WriteConsoleInputW")

func TestMain(m *testing.M) {
	if os.Getenv("PM_TEST_MODE") == "console-input" {
		helperConsoleInput()
	}
	os.Exit(m.Run())
}

// TestStdinReadableConsole spawns a helper with its own console and injects
// synthetic key events, reproducing the two queue states readToken depends
// on: key-up residue after Enter must read as "no more input" (the previous
// WaitForSingleObject implementation reported readable here and readToken
// would hang), and a fresh key-down must read as input pending.
func TestStdinReadableConsole(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self)
	cmd.Env = append(os.Environ(), "PM_TEST_MODE=console-input")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if strings.HasPrefix(s, "SKIP:") {
		// fmt so the skip is visible in CI logs without -v.
		fmt.Printf("SKIP TestStdinReadableConsole: %s\n", s)
		t.Skip(s)
	}
	if err != nil || !strings.Contains(s, "OK") {
		t.Fatalf("console helper: %v\n%s", err, s)
	}
	// fmt so "ran, not skipped" is visible in CI logs without -v.
	fmt.Println("CONSOLE VERDICT: TestStdinReadableConsole ran against a real console")
}

func helperConsoleInput() {
	conin, err := windows.CreateFile(
		windows.StringToUTF16Ptr("CONIN$"),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		fmt.Printf("SKIP: open CONIN$: %v\n", err)
		os.Exit(0)
	}
	var mode uint32
	if err := windows.GetConsoleMode(conin, &mode); err != nil {
		fmt.Printf("SKIP: GetConsoleMode: %v\n", err)
		os.Exit(0)
	}
	// The raw-ish mode readToken reads under: per-byte, no echo.
	raw := mode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)
	if err := windows.SetConsoleMode(conin, raw); err != nil {
		fail("SetConsoleMode: %v", err)
	}
	os.Stdin = os.NewFile(uintptr(conin), "CONIN$")

	// A typed "a" + Enter, as the console queues it: down and up events.
	if err := writeKeyEvents(conin,
		keyEvent('a', true), keyEvent('a', false),
		keyEvent('\r', true), keyEvent('\r', false),
	); err != nil {
		fail("WriteConsoleInput: %v", err)
	}

	// Drain the bytes the key-downs produce, like readToken's read loop.
	var got []byte
	buf := make([]byte, 64)
	for !bytes.ContainsRune(got, '\r') {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			fail("read CONIN$: %v (got %q)", err, got)
		}
		got = append(got, buf[:n]...)
	}

	if stdinReadable(200 * time.Millisecond) {
		fail("readable=true with only key-up events queued (would hang readToken)")
	}
	if err := writeKeyEvents(conin, keyEvent('b', true)); err != nil {
		fail("WriteConsoleInput: %v", err)
	}
	if !stdinReadable(2 * time.Second) {
		fail("readable=false with a key-down pending")
	}
	fmt.Println("OK")
	os.Exit(0)
}

func fail(format string, a ...any) {
	fmt.Printf("FAIL: "+format+"\n", a...)
	os.Exit(1)
}

func keyEvent(ch rune, down bool) inputRecord {
	rec := inputRecord{
		eventType:   windows.KEY_EVENT,
		repeatCount: 1,
		unicodeChar: uint16(ch),
	}
	if down {
		rec.keyDown = 1
	}
	return rec
}

func writeKeyEvents(h windows.Handle, recs ...inputRecord) error {
	var written uint32
	ok, _, err := procWriteConsoleInput.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&recs[0])),
		uintptr(len(recs)),
		uintptr(unsafe.Pointer(&written)),
	)
	if ok == 0 {
		return err
	}
	return nil
}
