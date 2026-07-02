package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "use <profile>",
		Short: "Set the default profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := loadConfig()
			if err != nil {
				return err
			}

			if _, err := cfg.GetProfile(args[0]); err != nil {
				return err
			}

			cfg.DefaultProfile = args[0]
			if err := cfg.Save(path); err != nil {
				return err
			}

			fmt.Printf("Default profile set to %q\n", args[0])
			return nil
		},
	})
}
