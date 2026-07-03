//go:build windows

package cmd

import (
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procPeekConsoleInput = kernel32.NewProc("PeekConsoleInputW")
)

// inputRecord is INPUT_RECORD with the event union flattened to
// KEY_EVENT_RECORD, the only variant read here (20 bytes either way).
type inputRecord struct {
	eventType       uint16
	_               uint16
	keyDown         int32
	repeatCount     uint16
	virtualKeyCode  uint16
	virtualScanCode uint16
	unicodeChar     uint16
	controlKeyState uint32
}

// stdinReadable reports whether stdin has bytes available within d.
//
// A console handle is signaled whenever *any* input records are queued —
// including the key-up, focus, and mouse events a read never turns into
// bytes — so waiting on the handle would report "readable" right after the
// user releases Enter and the next read would block. Peek the queue instead
// and count only key-down events that carry a character.
func stdinReadable(d time.Duration) bool {
	h := windows.Handle(os.Stdin.Fd())

	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		// Not a console (pipe or file): pending bytes keep it signaled.
		r, _ := windows.WaitForSingleObject(h, uint32(d.Milliseconds()))
		return r == windows.WAIT_OBJECT_0
	}

	deadline := time.Now().Add(d)
	for {
		if consoleCharPending(h) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// consoleCharPending reports whether the console input queue holds a
// key-down event with a character, i.e. whether a read would return bytes.
func consoleCharPending(h windows.Handle) bool {
	var n uint32
	if err := windows.GetNumberOfConsoleInputEvents(h, &n); err != nil || n == 0 {
		return false
	}
	records := make([]inputRecord, n)
	var read uint32
	ok, _, _ := procPeekConsoleInput.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&records[0])),
		uintptr(n),
		uintptr(unsafe.Pointer(&read)),
	)
	if ok == 0 {
		return false
	}
	for _, rec := range records[:read] {
		if rec.eventType == windows.KEY_EVENT && rec.keyDown != 0 && rec.unicodeChar != 0 {
			return true
		}
	}
	return false
}
