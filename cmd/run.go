package cmd

import (
	"github.com/boriswu0212/profile-manager/internal/runner"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "run [profile] [-- claude-args...]",
		Short: "Launch claude with the specified profile",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			var profileName string
			var claudeArgs []string

			// cobra strips the "--" separator; ArgsLenAtDash marks where it was.
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				claudeArgs = args[dash:]
				args = args[:dash]
			}
			if len(args) > 0 {
				profileName = args[0]
			}

			profile, err := resolveProfile(cfg, profileName)
			if err != nil {
				return err
			}

			model, _ := cmd.Flags().GetString("model")
			return runner.Run(profile, model, claudeArgs)
		},
	}
	cmd.Flags().StringP("model", "m", "", "override model for this run")
	rootCmd.AddCommand(cmd)
}
