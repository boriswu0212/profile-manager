package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/b0riswu/profile-manager/internal/updater"
	"github.com/spf13/cobra"
)

var checkOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update pm to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		current := rootCmd.Version
		if current == "dev" {
			fmt.Println("Running development build — use 'go install github.com/b0riswu/profile-manager/cmd/pm@latest' to update.")
			return nil
		}

		fmt.Printf("Current version: %s\n", current)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fmt.Println("Checking for updates...")
		rel, err := updater.Latest(ctx)
		if err != nil {
			return fmt.Errorf("check for updates: %w", err)
		}

		if !updater.IsNewer(current, rel.Version) {
			fmt.Println("Already up to date.")
			return nil
		}

		fmt.Printf("New version available: %s\n", rel.Version)
		if checkOnly {
			return nil
		}

		target, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate binary: %w", err)
		}

		fmt.Printf("Downloading and installing %s...\n", rel.Version)
		if err := updater.Apply(ctx, rel, target); err != nil {
			return fmt.Errorf("update: %w", err)
		}

		fmt.Printf("Updated to %s.\n", rel.Version)
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVar(&checkOnly, "check", false, "only check whether an update is available")
	rootCmd.AddCommand(updateCmd)
}
