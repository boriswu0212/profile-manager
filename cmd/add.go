package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/boriswu0212/profile-manager/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a new profile interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := loadConfig()
			if err != nil {
				return err
			}

			reader := bufio.NewReader(os.Stdin)
			prompt := func(label, defaultVal string) string {
				if defaultVal != "" {
					fmt.Printf("%s [%s]: ", label, defaultVal)
				} else {
					fmt.Printf("%s: ", label)
				}
				line, _ := reader.ReadString('\n')
				line = strings.TrimSpace(line)
				if line == "" {
					return defaultVal
				}
				return line
			}

			p := config.Profile{}
			p.Name = prompt("Profile name", "")
			if p.Name == "" {
				return fmt.Errorf("name is required")
			}
			if _, err := cfg.GetProfile(p.Name); err == nil {
				return fmt.Errorf("profile %q already exists", p.Name)
			}

			p.Tool = prompt("Tool (claude/codex)", "claude")
			p.Provider = prompt("Provider (anthropic/openai/bedrock/subscription)", "anthropic")

			switch p.Provider {
			case config.ProviderAnthropic, config.ProviderOpenAI:
				defaultURL := "https://api.anthropic.com"
				defaultModel := "claude-sonnet-4-20250514"
				if p.EffectiveTool() == config.ToolCodex {
					defaultURL = "https://api.openai.com/v1"
					defaultModel = "openai/gpt-5.4-mini-codex"
				}
				p.BaseURL = prompt("Base URL", defaultURL)
				fmt.Println("API key formats: sk-xxx (literal), ${ENV_VAR}, keychain://name")
				fmt.Println("Or leave empty and set api_key_cmd in config for shell command")
				p.APIKey = prompt("API key", "")
				p.Model = prompt("Default model", defaultModel)

			case config.ProviderBedrock:
				p.Region = prompt("AWS region", "us-east-1")
				p.AWSProfile = prompt("AWS profile", "")
				p.Model = prompt("Default model", "us.anthropic.claude-sonnet-4-20250514-v1:0")

			case config.ProviderSubscription:
				p.Model = prompt("Default model (optional)", "")

			default:
				return fmt.Errorf("unknown provider: %s", p.Provider)
			}

			ctxStr := prompt("Max context tokens (e.g. 1000000 for 1M, empty for default 256k)", "")
			if ctxStr != "" {
				v, err := strconv.Atoi(ctxStr)
				if err != nil || v <= 0 {
					return fmt.Errorf("invalid context tokens: %s", ctxStr)
				}
				p.MaxContextTokens = v
			}

			cfg.Profiles = append(cfg.Profiles, p)
			if cfg.DefaultProfile == "" {
				cfg.DefaultProfile = p.Name
			}

			if err := cfg.Save(path); err != nil {
				return err
			}

			fmt.Printf("Profile %q added.\n", p.Name)
			return nil
		},
	})
}
