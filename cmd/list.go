package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}

			if len(cfg.Profiles) == 0 {
				fmt.Println("No profiles configured. Run 'pm add' to create one.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "  \tNAME\tTOOL\tPROVIDER\tMODEL\tENDPOINT")
			for _, p := range cfg.Profiles {
				marker := " "
				if p.Name == cfg.DefaultProfile {
					marker = "*"
				}
				endpoint := p.BaseURL
				if p.Provider == "bedrock" {
					endpoint = p.Region
				}
				if p.Provider == "subscription" {
					endpoint = "(subscription)"
				}
				model := p.Model
				if model == "" {
					model = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", marker, p.Name, p.EffectiveTool(), p.Provider, model, endpoint)
			}
			w.Flush()
			return nil
		},
	})
}
