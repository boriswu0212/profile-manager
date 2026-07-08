package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/b0riswu/profile-manager/internal/config"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
	"golang.org/x/sys/unix"
)

func init() {
	var pasteOnly bool
	var account string
	c := &cobra.Command{
		Use:   "login <profile>",
		Short: "Bind a Claude subscription account to a profile",
		Long: `Runs "claude setup-token" and stores the resulting OAuth token in the OS
keychain, bound to the given profile. Each profile can hold a different
claude.ai account; "pm run <profile>" launches claude as that account.
The profile is created (provider=subscription) if it does not exist.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(args[0], pasteOnly, account)
		},
	}
	c.Flags().BoolVar(&pasteOnly, "paste", false, "skip running claude setup-token and paste an existing token")
	c.Flags().StringVar(&account, "account", "", "label for the claude.ai account this token belongs to (shown at launch)")
	rootCmd.AddCommand(c)
}

func runLogin(name string, pasteOnly bool, account string) error {
	cfg, path, err := loadConfig()
	if err != nil {
		return err
	}

	profile, err := cfg.GetProfile(name)
	if err != nil {
		cfg.Profiles = append(cfg.Profiles, config.Profile{
			Name:     name,
			Provider: config.ProviderSubscription,
			Tool:     config.ToolClaude,
		})
		profile = &cfg.Profiles[len(cfg.Profiles)-1]
		fmt.Printf("Creating subscription profile %q.\n", name)
	}

	if profile.Provider != config.ProviderSubscription {
		return fmt.Errorf("profile %q has provider %q — pm login only applies to subscription profiles", name, profile.Provider)
	}
	if profile.EffectiveTool() != config.ToolClaude {
		return fmt.Errorf("profile %q uses tool %q — subscription login requires tool=claude", name, profile.EffectiveTool())
	}
	// Fingerprint of the token being replaced, so the outcome message can
	// prove whether the stored token actually changed.
	oldFP := ""
	if strings.HasPrefix(profile.APIKey, "keychain://") {
		if old, err := keyring.Get("pm", name); err == nil {
			oldFP = config.TokenFingerprint(old)
		}
		fmt.Printf("Profile %q already has a token (%s); it will be replaced.\n", name, oldFP)
	}

	if !pasteOnly {
		fmt.Println("Sign your browser into the claude.ai account to bind to this profile, then")
		fmt.Println("complete the flow below. Note: setup-token invalidates any token previously")
		fmt.Println("minted for the same account (use --paste to store an existing token instead).")
		fmt.Println()

		claudeBin, err := exec.LookPath("claude")
		if err != nil {
			return fmt.Errorf("claude not found in PATH: %w", err)
		}
		setup := exec.Command(claudeBin, "setup-token")
		setup.Stdin, setup.Stdout, setup.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := setup.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "claude setup-token exited with an error (%v); you can still paste a token.\n", err)
		}
		fmt.Println()
	}

	token, err := readToken()
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}
	if !strings.HasPrefix(token, "sk-ant-oat") {
		return fmt.Errorf("input does not look like a Claude OAuth token (expected sk-ant-oat… prefix)")
	}

	// The API cannot identify a setup-token (user:inference scope only), so
	// the account binding is a user-declared label. Asked before anything is
	// persisted; Ctrl-C here aborts cleanly.
	if account == "" && term.IsTerminal(os.Stdin.Fd()) {
		def := ""
		if profile.Account != "" {
			def = fmt.Sprintf(" [%s]", profile.Account)
		}
		fmt.Fprintf(os.Stderr, "Account this token belongs to (label shown at launch, e.g. email)%s: ", def)
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" && profile.Account == "" {
			fmt.Fprintln(os.Stderr, "(no label set)")
		}
		account = strings.TrimSpace(line)
	}

	if err := keyring.Set("pm", name, token); err != nil {
		return fmt.Errorf("store token in keychain: %w", err)
	}
	profile.APIKey = "keychain://" + name
	if account != "" {
		profile.Account = account
	}
	profile.TokenBoundAt = time.Now().Format("2006-01-02")
	if err := cfg.Save(path); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	newFP := config.TokenFingerprint(token)
	switch {
	case oldFP == "":
		fmt.Printf("Token %s stored in the OS keychain (service \"pm\").\n", newFP)
	case oldFP == newFP:
		fmt.Printf("Token %s stored — unchanged, same token as before.\n", newFP)
	default:
		fmt.Printf("Token %s stored — replaces %s.\n", newFP, oldFP)
	}
	if profile.Account != "" {
		fmt.Printf("Account label: %s\n", profile.Account)
	}
	fmt.Printf("Launch with: pm run %s\n", name)
	return nil
}

// readToken reads the pasted token without echo when stdin is a terminal;
// piped stdin reads to EOF. Terminals hard-wrap long pastes with embedded
// newlines, so a line read would keep only the first fragment (and leak the
// rest to the shell). Instead the tty is read in raw mode and a newline ends
// input only once the tty goes idle: a pasted burst keeps flowing past its
// embedded newlines, a typed Enter is followed by silence.
func readToken() (string, error) {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return normalizeToken(string(b)), nil
	}

	fmt.Fprint(os.Stderr, "Paste token (input hidden): ")
	defer fmt.Fprintln(os.Stderr)
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, state)

	var buf []byte
	chunk := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(chunk)
		for _, c := range chunk[:n] {
			switch c {
			case 0x03: // Ctrl-C
				return "", fmt.Errorf("cancelled")
			case 0x08, 0x7f: // backspace
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
				}
			default:
				buf = append(buf, c)
			}
		}
		if err != nil { // EOF: submit what we have
			break
		}
		if n > 0 {
			last := chunk[n-1]
			if last == 0x04 { // Ctrl-D
				break
			}
			if (last == '\r' || last == '\n') && !stdinReadable(200*time.Millisecond) {
				break
			}
		}
	}
	return normalizeToken(string(buf)), nil
}

// normalizeToken strips whitespace and control bytes anywhere in the input,
// rejoining a paste that a terminal wrapped across lines. Tokens are
// printable ASCII, so nothing legitimate is removed.
func normalizeToken(raw string) string {
	// A terminal left in bracketed-paste mode (e.g. by a TUI that exited
	// without resetting it) wraps pastes in ESC[200~ / ESC[201~ markers;
	// their printable tails would survive the byte filter below.
	raw = strings.ReplaceAll(raw, "\x1b[200~", "")
	raw = strings.ReplaceAll(raw, "\x1b[201~", "")
	var b strings.Builder
	for _, r := range raw {
		if r > ' ' && r < 0x7f {
			b.WriteRune(r)
		}
	}
	return b.String()
}

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
