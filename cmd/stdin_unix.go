//go:build !windows

package cmd

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// stdinReadable reports whether stdin has bytes available within d.
func stdinReadable(d time.Duration) bool {
	fds := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, int(d.Milliseconds()))
		if err == unix.EINTR {
			continue
		}
		return err == nil && n > 0
	}
}
