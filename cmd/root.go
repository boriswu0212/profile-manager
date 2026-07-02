package cmd

import (
	"fmt"
	"os"

	"github.com/boriswu0212/profile-manager/internal/config"
	"github.com/boriswu0212/profile-manager/internal/tui"
	"github.com/spf13/cobra"
)

var cfgPath string

var rootCmd = &cobra.Command{
	Use:   "pm",
	Short: "Claude Code environment profile manager",
	Long:  "Manage multiple Claude Code profiles (endpoint, API key, model) with secure credential handling.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "config file (default ~/.pm.yaml)")
}

func loadConfig() (*config.Config, string, error) {
	path := cfgPath
	if path == "" {
		path = config.DefaultPath()
	}

	if err := config.CheckConfigPermissions(path); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}
	return cfg, path, nil
}

func resolveProfile(cfg *config.Config, name string) (*config.Profile, error) {
	if name != "" {
		return cfg.GetProfile(name)
	}
	return cfg.GetDefaultProfile()
}

func runTUI() error {
	cfg, path, err := loadConfig()
	if err != nil {
		return err
	}

	p := tui.New(cfg, path)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	if m, ok := finalModel.(interface{ ShouldLaunch() bool }); ok && m.ShouldLaunch() {
		if launcher, ok := finalModel.(interface{ Launch() error }); ok {
			return launcher.Launch()
		}
	}

	return nil
}
