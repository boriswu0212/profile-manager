package cmd

import (
	"fmt"

	"github.com/b0riswu/profile-manager/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:     "remove <profile>",
		Short:   "Remove a profile",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := loadConfig()
			if err != nil {
				return err
			}

			// Best-effort cleanup of the keychain entry `pm login` created;
			// never blocks removal, and never touches an entry another
			// profile still references.
			if p, lookupErr := cfg.GetProfile(args[0]); lookupErr == nil &&
				p.Provider == config.ProviderSubscription &&
				p.APIKey == "keychain://"+p.Name &&
				!otherProfileReferences(cfg, p.Name, p.APIKey) {
				_ = config.DeleteOAuthToken(p.Name)
			}

			if err := cfg.RemoveProfile(args[0]); err != nil {
				return err
			}

			if err := cfg.Save(path); err != nil {
				return err
			}

			fmt.Printf("Profile %q removed.\n", args[0])
			return nil
		},
	})
}

// otherProfileReferences reports whether any profile other than name has
// apiKey as its api_key — guards shared keychain:// entries from deletion.
func otherProfileReferences(cfg *config.Config, name, apiKey string) bool {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name != name && cfg.Profiles[i].APIKey == apiKey {
			return true
		}
	}
	return false
}
