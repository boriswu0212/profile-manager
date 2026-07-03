package cmd

import (
	"fmt"

	"github.com/boriswu0212/profile-manager/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "logout <profile>",
		Short: "Remove the stored subscription token from a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := loadConfig()
			if err != nil {
				return err
			}
			profile, err := cfg.GetProfile(args[0])
			if err != nil {
				return err
			}
			if profile.Provider != config.ProviderSubscription {
				return fmt.Errorf("profile %q is not a subscription profile", args[0])
			}

			// Only delete the keychain entry `pm login` owns (keychain://<name>)
			// and only when no other profile points at the same entry.
			if profile.APIKey == "keychain://"+profile.Name &&
				!otherProfileReferences(cfg, profile.Name, profile.APIKey) {
				if err := config.DeleteOAuthToken(profile.Name); err != nil {
					return err
				}
			}
			profile.APIKey = ""
			profile.Account = ""
			profile.TokenBoundAt = ""
			if err := cfg.Save(path); err != nil {
				return err
			}

			fmt.Printf("Token removed; profile %q now uses the shared claude.ai login.\n", args[0])
			return nil
		},
	})
}
