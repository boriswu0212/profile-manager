//go:build windows

package runner

import (
	"os"
	"os/exec"
	"os/signal"
)

func execProcess(binary string, argv []string) error {
	// Windows delivers console ctrl events (Ctrl+C/Break) to every process
	// on the console, so pm must survive them while the child owns the
	// session — and must not fire a pre-exec cleanup handler registered via
	// setupSignalHandler mid-session. The runtime's ctrl handler only
	// reports the event as handled (keeping the process alive) when a
	// signal.Notify registration exists; signal.Ignore does NOT stop the
	// OS default of terminating the process (exit 0xc000013a — see runtime
	// ctrlHandler/sigsend, proven by TestExecProcessIgnoresCtrlBreak).
	// Reset cancels the pre-exec handler the way unix exec would wipe it;
	// Notify on a channel nobody reads swallows the events.
	signal.Reset(os.Interrupt)
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)

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
