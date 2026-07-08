package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/b0riswu/profile-manager/internal/provider"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "models [profile]",
		Short: "List available models from the endpoint",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			var profileName string
			if len(args) > 0 {
				profileName = args[0]
			}

			profile, err := resolveProfile(cfg, profileName)
			if err != nil {
				return err
			}

			prov, err := provider.ForProfile(profile)
			if err != nil {
				return fmt.Errorf("[%s] %s", profile.Name, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			fmt.Printf("Fetching models from %s (%s)...\n", profile.Name, profile.Provider)
			models, err := prov.ListModels(ctx)
			if err != nil {
				return fmt.Errorf("list models: %w", err)
			}

			if len(models) == 0 {
				fmt.Println("No models found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "MODEL ID\tDISPLAY NAME\tINPUT\tOUTPUT\tCAPABILITIES")
			for _, m := range models {
				input := fmtTokens(m.MaxInputTokens)
				output := fmtTokens(m.MaxOutputTokens)
				caps := "-"
				if len(m.Capabilities) > 0 {
					caps = strings.Join(m.Capabilities, ",")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", m.ID, m.DisplayName, input, output, caps)
			}
			w.Flush()
			return nil
		},
	})
}

func fmtTokens(n int) string {
	if n <= 0 {
		return "-"
	}
	if n >= 1000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%d", n)
}
