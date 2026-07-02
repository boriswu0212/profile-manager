package cmd

import (
	"fmt"

	"github.com/boriswu0212/profile-manager/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:    "_resolve-key <profile>",
		Short:  "Resolve and output API key (for apiKeyHelper)",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			profile, err := cfg.GetProfile(args[0])
			if err != nil {
				return err
			}

			key, err := config.ResolveAPIKey(profile)
			if err != nil {
				return err
			}

			fmt.Print(key)
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
