package cmd

import (
	"fmt"

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
