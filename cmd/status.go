package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show current active profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			profile, err := cfg.GetDefaultProfile()
			if err != nil {
				return err
			}

			fmt.Printf("Active profile: %s\n", profile.Name)
			fmt.Printf("  Tool:      %s\n", profile.EffectiveTool())
			fmt.Printf("  Provider:  %s\n", profile.Provider)
			if profile.BaseURL != "" {
				fmt.Printf("  Endpoint:  %s\n", profile.BaseURL)
			}
			if profile.Model != "" {
				fmt.Printf("  Model:     %s\n", profile.Model)
			}
			if profile.Region != "" {
				fmt.Printf("  Region:    %s\n", profile.Region)
			}
			if profile.AWSProfile != "" {
				fmt.Printf("  AWS Prof:  %s\n", profile.AWSProfile)
			}

			keyType := "none"
			switch {
			case profile.Provider == "subscription" || profile.Provider == "bedrock":
				keyType = "(not applicable)"
			case profile.APIKeyCmd != "":
				keyType = "command: " + profile.APIKeyCmd
			case len(profile.APIKey) > 10:
				keyType = profile.APIKey[:10] + "..."
			case profile.APIKey != "":
				keyType = profile.APIKey
			}
			fmt.Printf("  API Key:   %s\n", keyType)

			if recent := cfg.RecentForProfile(profile.Name); len(recent) > 0 {
				fmt.Println("\nRecent:")
				for _, r := range recent {
					fmt.Printf("  %s  (%s)\n", r.Model, r.UsedAt[:10])
				}
			}

			return nil
		},
	})
}
