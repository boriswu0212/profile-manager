package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/boriswu0212/profile-manager/internal/config"
)

func codexPath() (string, error) {
	return exec.LookPath("codex")
}

func codexConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml")
}

type codexBackup struct {
	path    string
	existed bool
	content []byte
}

func backupCodexConfig(path string) (*codexBackup, error) {
	backup := &codexBackup{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return backup, nil
		}
		return nil, fmt.Errorf("read codex config: %w", err)
	}
	backup.existed = true
	backup.content = data
	return backup, nil
}

func (b *codexBackup) restore() error {
	if !b.existed {
		os.Remove(b.path)
		return nil
	}
	return os.WriteFile(b.path, b.content, 0600)
}

func RunCodex(profile *config.Profile, model string, codexArgs []string) error {
	cp, err := codexPath()
	if err != nil {
		return fmt.Errorf("codex not found in PATH: %w", err)
	}

	if model != "" {
		profile.Model = model
	}

	key, err := config.ResolveAPIKey(profile)
	if err != nil {
		return err
	}

	envKeyName := profile.EnvKey
	if envKeyName == "" {
		envKeyName = "PM_CODEX_API_KEY"
	}
	os.Setenv(envKeyName, key)

	cfgPath := codexConfigPath()
	backup, err := backupCodexConfig(cfgPath)
	if err != nil {
		return err
	}

	providerID := "pm"
	toml := buildCodexTOML(providerID, profile, envKeyName)

	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create codex config dir: %w", err)
	}
	if err := os.WriteFile(cfgPath, []byte(toml), 0600); err != nil {
		_ = backup.restore()
		return fmt.Errorf("write codex config: %w", err)
	}

	// Covers only the window between here and the Exec below: exec replaces
	// the process (including this goroutine), so once codex is running,
	// signals go to codex and config.toml stays overwritten.
	setupSignalHandler(func() { _ = backup.restore() })

	announce(profile, fmt.Sprintf("%s API", profile.Provider))
	argv := []string{"codex"}
	if profile.Model != "" {
		argv = append(argv, "--model", profile.Model)
	}
	argv = append(argv, codexArgs...)

	return syscall.Exec(cp, argv, os.Environ())
}

func buildCodexTOML(providerID string, profile *config.Profile, envKeyName string) string {
	baseURL := profile.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	s := ""
	if profile.Model != "" {
		s += fmt.Sprintf("model = %q\n", profile.Model)
	}
	s += fmt.Sprintf("model_provider = %q\n\n", providerID)
	s += fmt.Sprintf("[model_providers.%s]\n", providerID)
	s += fmt.Sprintf("name = %q\n", profile.Name)
	s += fmt.Sprintf("base_url = %q\n", baseURL)
	s += fmt.Sprintf("env_key = %q\n", envKeyName)

	return s
}
