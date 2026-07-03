//go:build !windows

package runner

import (
	"os"
	"os/signal"
	"syscall"
)

func execProcess(binary string, argv []string) error {
	return syscall.Exec(binary, argv, os.Environ())
}

func setupSignalHandler(cleanup func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cleanup()
		os.Exit(130)
	}()
}
