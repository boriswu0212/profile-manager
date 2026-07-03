package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/boriswu0212/profile-manager/internal/config"
)

// anthropicBaseURL normalises a user-supplied base URL for ANTHROPIC_BASE_URL,
// which must not contain the "/v1" segment — Claude Code's Anthropic SDK
// appends "/v1/messages" itself. OpenAI-convention URLs keep "/v1" in the
// profile (codex needs it); strip it here for claude only.
//
//	"https://api.anthropic.com"                     → "https://api.anthropic.com"
//	"https://proxy.example.com/prod/aiendpoint/v1/" → "https://proxy.example.com/prod/aiendpoint"
func anthropicBaseURL(raw string) string {
	return strings.TrimSuffix(strings.TrimRight(raw, "/"), "/v1")
}

func claudeSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "pm"
	}
	return p
}

func claudePath() (string, error) {
	return exec.LookPath("claude")
}

type settingsBackup struct {
	path            string
	hadHelper       bool
	originalHelper  string
	originalContent map[string]any
}

func backupSettings(path string) (*settingsBackup, error) {
	backup := &settingsBackup{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			backup.originalContent = make(map[string]any)
			return backup, nil
		}
		return nil, fmt.Errorf("read settings: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}

	backup.originalContent = settings
	if helper, ok := settings["apiKeyHelper"]; ok {
		backup.hadHelper = true
		backup.originalHelper = fmt.Sprintf("%v", helper)
	}

	return backup, nil
}

func (b *settingsBackup) restore() error {
	if b.hadHelper {
		b.originalContent["apiKeyHelper"] = b.originalHelper
	} else {
		delete(b.originalContent, "apiKeyHelper")
	}
	return writeSettings(b.path, b.originalContent)
}

func writeSettings(path string, settings map[string]any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

func Run(profile *config.Profile, model string, extraArgs []string) error {
	applyModelAndRecord(profile, model)

	if profile.EffectiveTool() == config.ToolCodex {
		return RunCodex(profile, model, extraArgs)
	}

	cp, err := claudePath()
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}

	switch profile.Provider {
	case config.ProviderAnthropic, config.ProviderOpenAI:
		return runWithAPIKeyHelper(cp, profile, extraArgs)
	case config.ProviderBedrock:
		return runBedrock(cp, profile, extraArgs)
	case config.ProviderSubscription:
		return runSubscription(cp, profile, extraArgs)
	default:
		return fmt.Errorf("unknown provider: %s", profile.Provider)
	}
}

// applyModelAndRecord resolves which model this launch uses — explicit -m
// flag > model last used in the current directory > the profile's default —
// and records the launch (recent list + per-directory last model).
func applyModelAndRecord(profile *config.Profile, model string) {
	if model != "" {
		profile.Model = model
	}

	cwd, _ := os.Getwd()
	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return
	}

	if model == "" && cwd != "" {
		if last := cfg.LastModel(cwd, profile.Name); last != "" {
			profile.Model = last
		}
	}

	cfg.RecordUsage(profile.Name, profile.Model)
	cfg.RecordLastModel(cwd, profile.Name, profile.Model)
	_ = cfg.Save(cfgPath)
}

// announce prints a one-line launch identity to stderr (it stays in the
// terminal scrollback above the tool's own banner) and exports PM_PROFILE
// into the launched process. Claude Code labels every token launch just
// "CLAUDE_CODE_OAUTH_TOKEN", so with several token-bound accounts the
// session itself never says which one it is — this line and
// `!echo $PM_PROFILE` inside the session are the tell. Stderr, not stdout,
// so piped headless runs (`pm run p -- -p ...`) stay clean.
func announce(profile *config.Profile, auth string) {
	os.Setenv("PM_PROFILE", profile.Name)
	fmt.Fprintf(os.Stderr, "pm ▸ profile %q · %s · %s\n", profile.Name, profile.EffectiveTool(), auth)
}

func runWithAPIKeyHelper(cp string, profile *config.Profile, args []string) error {
	settingsPath := claudeSettingsPath()

	backup, err := backupSettings(settingsPath)
	if err != nil {
		return err
	}

	delete(backup.originalContent, "apiKeyHelper")
	if err := writeSettings(settingsPath, backup.originalContent); err != nil {
		return fmt.Errorf("clean apiKeyHelper: %w", err)
	}

	// Fail early if the key cannot be resolved. The key itself is delivered
	// via a per-invocation apiKeyHelper (--settings flag) instead of
	// ANTHROPIC_API_KEY: an env key alongside a claude.ai login makes Claude
	// Code print "Both claude.ai and ANTHROPIC_API_KEY set" on every start,
	// while the apiKeyHelper source is exempt from that auth-conflict warning.
	if _, err := config.ResolveAPIKey(profile); err != nil {
		return fmt.Errorf("resolve API key: %w", err)
	}

	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_AUTH_TOKEN")
	os.Unsetenv("CLAUDE_CODE_OAUTH_TOKEN")

	if profile.BaseURL != "" {
		os.Setenv("ANTHROPIC_BASE_URL", anthropicBaseURL(profile.BaseURL))
	}

	flagSettings, err := json.Marshal(map[string]any{
		"apiKeyHelper": fmt.Sprintf("%q _resolve-key %q", selfPath(), profile.Name),
		// claude.ai connectors cannot work through a non-claude.ai gateway;
		// disabling them explicitly also silences the startup notice about it.
		"disableClaudeAiConnectors": true,
	})
	if err != nil {
		return fmt.Errorf("marshal flag settings: %w", err)
	}

	announce(profile, fmt.Sprintf("%s API", profile.Provider))
	argv := append([]string{"claude", "--settings", string(flagSettings)}, args...)
	if profile.Model != "" {
		argv = append(argv, "--model", profile.Model)
	}
	return execProcess(cp, argv)
}

func runBedrock(cp string, profile *config.Profile, args []string) error {
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_AUTH_TOKEN")
	os.Unsetenv("ANTHROPIC_BASE_URL")
	os.Unsetenv("CLAUDE_CODE_OAUTH_TOKEN")

	os.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	if profile.Region != "" {
		os.Setenv("AWS_REGION", profile.Region)
	}
	if profile.AWSProfile != "" {
		os.Setenv("AWS_PROFILE", profile.AWSProfile)
	}
	auth := "bedrock"
	if profile.Region != "" {
		auth += " " + profile.Region
	}
	announce(profile, auth)
	argv := append([]string{"claude"}, args...)
	if profile.Model != "" {
		argv = append(argv, "--model", profile.Model)
	}
	return execProcess(cp, argv)
}

func runSubscription(cp string, profile *config.Profile, args []string) error {
	settingsPath := claudeSettingsPath()

	backup, err := backupSettings(settingsPath)
	if err == nil {
		delete(backup.originalContent, "apiKeyHelper")
		_ = writeSettings(settingsPath, backup.originalContent)
	}

	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_AUTH_TOKEN")
	os.Unsetenv("ANTHROPIC_BASE_URL")
	os.Unsetenv("CLAUDE_CODE_USE_BEDROCK")
	os.Unsetenv("CLAUDE_CODE_USE_VERTEX")

	// A profile-bound token (`pm login`) authenticates this launch as that
	// subscription account. It sits below ANTHROPIC_AUTH_TOKEN/API_KEY and
	// apiKeyHelper in Claude Code's credential precedence, which is why all
	// of those are cleared above. Without a token the ambient claude.ai
	// login is used, and a stray token from the shell must not override it.
	token, err := config.ResolveOAuthToken(profile)
	if err != nil {
		return fmt.Errorf("resolve OAuth token for %q: %w", profile.Name, err)
	}
	if token != "" {
		os.Setenv("CLAUDE_CODE_OAUTH_TOKEN", token)
		// Fingerprint the token actually being exported (not stored
		// metadata) so a re-login is visible as a changed fingerprint.
		id := "token " + config.TokenFingerprint(token)
		if profile.Account != "" {
			id = profile.Account + " · " + id
		}
		announce(profile, "subscription ("+id+")")
	} else {
		os.Unsetenv("CLAUDE_CODE_OAUTH_TOKEN")
		announce(profile, "subscription (shared claude.ai login)")
	}

	argv := append([]string{"claude"}, args...)
	if profile.Model != "" {
		argv = append(argv, "--model", profile.Model)
	}
	return execProcess(cp, argv)
}
