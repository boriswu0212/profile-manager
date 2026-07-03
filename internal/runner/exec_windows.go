//go:build windows

package runner

import (
	"os"
	"os/exec"
	"os/signal"
)

func execProcess(binary string, argv []string) error {
	// Windows delivers console ctrl events (Ctrl+C/Break) to every process
	// on the console, so without this the user pressing Ctrl+C inside the
	// child would also kill pm — or worse, fire a pre-exec cleanup handler
	// registered via setupSignalHandler mid-session. Unix exec wipes those
	// handlers when the child image takes over; signal.Ignore reproduces
	// that (it undoes prior Notify calls), leaving pm to just wait and
	// forward the child's exit code.
	signal.Ignore(os.Interrupt)

	cmd := exec.Command(binary, argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}

func setupSignalHandler(cleanup func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cleanup()
		os.Exit(130)
	}()
}
