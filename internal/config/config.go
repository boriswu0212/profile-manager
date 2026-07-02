package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

const (
	MaxRecent     = 10
	MaxLastModels = 50
)

const (
	ProviderAnthropic    = "anthropic"
	ProviderOpenAI       = "openai"
	ProviderBedrock      = "bedrock"
	ProviderSubscription = "subscription"

	ToolClaude = "claude"
	ToolCodex  = "codex"
)

type Profile struct {
	Name       string `yaml:"name"`
	Tool       string `yaml:"tool,omitempty"`
	Provider   string `yaml:"provider"`
	BaseURL    string `yaml:"base_url,omitempty"`
	APIKey     string `yaml:"api_key,omitempty"`
	APIKeyCmd  string `yaml:"api_key_cmd,omitempty"`
	Model      string `yaml:"model,omitempty"`
	Region     string `yaml:"region,omitempty"`
	AWSProfile string `yaml:"aws_profile,omitempty"`
	EnvKey     string `yaml:"env_key,omitempty"`
}

func (p *Profile) EffectiveTool() string {
	if p.Tool != "" {
		return p.Tool
	}
	return ToolClaude
}

type RecentEntry struct {
	Profile string `yaml:"profile"`
	Model   string `yaml:"model"`
	UsedAt  string `yaml:"used_at"`
}

// LastModelEntry remembers which model a profile was last launched with in
// a given working directory.
type LastModelEntry struct {
	Dir     string `yaml:"dir"`
	Profile string `yaml:"profile"`
	Model   string `yaml:"model"`
	UsedAt  string `yaml:"used_at"`
}

type Config struct {
	DefaultProfile string           `yaml:"default_profile"`
	Profiles       []Profile        `yaml:"profiles"`
	Recent         []RecentEntry    `yaml:"recent,omitempty"`
	LastModels     []LastModelEntry `yaml:"last_models,omitempty"`
}

// RecordLastModel stores model as the most recent launch for (dir, profile),
// keeping at most MaxLastModels entries in MRU order.
func (c *Config) RecordLastModel(dir, profile, model string) {
	if dir == "" || model == "" {
		return
	}
	entry := LastModelEntry{
		Dir:     dir,
		Profile: profile,
		Model:   model,
		UsedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	var filtered []LastModelEntry
	for _, e := range c.LastModels {
		if e.Dir == dir && e.Profile == profile {
			continue
		}
		filtered = append(filtered, e)
	}
	c.LastModels = append([]LastModelEntry{entry}, filtered...)
	if len(c.LastModels) > MaxLastModels {
		c.LastModels = c.LastModels[:MaxLastModels]
	}
}

// LastModel returns the model last launched for (dir, profile), or "".
func (c *Config) LastModel(dir, profile string) string {
	for _, e := range c.LastModels {
		if e.Dir == dir && e.Profile == profile {
			return e.Model
		}
	}
	return ""
}

func (c *Config) RecordUsage(profileName, model string) {
	entry := RecentEntry{
		Profile: profileName,
		Model:   model,
		UsedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	var filtered []RecentEntry
	for _, r := range c.Recent {
		if r.Profile == profileName && r.Model == model {
			continue
		}
		filtered = append(filtered, r)
	}

	c.Recent = append([]RecentEntry{entry}, filtered...)
	if len(c.Recent) > MaxRecent {
		c.Recent = c.Recent[:MaxRecent]
	}
}

func (c *Config) RecentForProfile(profileName string) []RecentEntry {
	var out []RecentEntry
	for _, r := range c.Recent {
		if r.Profile == profileName {
			out = append(out, r)
		}
	}
	return out
}

var envVarRe = regexp.MustCompile(`^\$\{(.+)\}$`)
var keychainRe = regexp.MustCompile(`^keychain://(.+)$`)

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pm.yaml")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) GetProfile(name string) (*Profile, error) {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			return &c.Profiles[i], nil
		}
	}
	return nil, fmt.Errorf("profile %q not found", name)
}

func (c *Config) GetDefaultProfile() (*Profile, error) {
	if c.DefaultProfile == "" {
		if len(c.Profiles) > 0 {
			return &c.Profiles[0], nil
		}
		return nil, fmt.Errorf("no profiles configured")
	}
	return c.GetProfile(c.DefaultProfile)
}

func (c *Config) RemoveProfile(name string) error {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			c.Profiles = append(c.Profiles[:i], c.Profiles[i+1:]...)
			if c.DefaultProfile == name {
				c.DefaultProfile = ""
			}
			return nil
		}
	}
	return fmt.Errorf("profile %q not found", name)
}

// ResolveAPIKey resolves the API key from the profile. api_key is
// interpreted as keychain://name (OS keychain), ${ENV_VAR}, or a literal
// string; api_key_cmd (shell command stdout) is used only when api_key is
// empty.
func ResolveAPIKey(p *Profile) (string, error) {
	if p.Provider == ProviderBedrock || p.Provider == ProviderSubscription {
		return "", nil
	}

	if p.APIKey != "" {
		if m := keychainRe.FindStringSubmatch(p.APIKey); m != nil {
			key, err := keyring.Get("pm", m[1])
			if err != nil {
				return "", fmt.Errorf("keychain lookup %q: %w", m[1], err)
			}
			return key, nil
		}

		if m := envVarRe.FindStringSubmatch(p.APIKey); m != nil {
			val := os.Getenv(m[1])
			if val == "" {
				return "", fmt.Errorf("env var %q is empty", m[1])
			}
			return val, nil
		}

		return p.APIKey, nil
	}

	if p.APIKeyCmd != "" {
		out, err := exec.Command("sh", "-c", p.APIKeyCmd).Output()
		if err != nil {
			return "", fmt.Errorf("api_key_cmd failed: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}

	return "", fmt.Errorf("no api_key or api_key_cmd configured for profile %q", p.Name)
}

func CheckConfigPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return fmt.Errorf("config file %s has permissions %o (should be 0600). Fix with: chmod 600 %s", path, perm, path)
	}
	return nil
}
