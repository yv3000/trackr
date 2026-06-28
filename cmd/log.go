package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"trackr/internal/db"
	"trackr/internal/ui"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show install history tracked by trackr",
	Long: `Display the install history recorded in the local trackr database.
Note: trackr can only show installs that happened after it was first used —
it cannot retroactively learn about software installed before then.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.Open()
		if err != nil {
			return err
		}
		defer database.Close()

		installs, err := database.ListInstalls()
		if err != nil {
			return err
		}

		fmt.Println()
		fmt.Println(ui.TitleStyle.Render("  trackr log"))
		fmt.Println(ui.DividerStyle.Render("  " + repeat(58)))
		fmt.Printf("  %-12s %-8s %-16s %s\n", "DATE", "TOOL", "PACKAGE", "WHY")
		fmt.Println(ui.DividerStyle.Render("  " + repeat(58)))

		if len(installs) == 0 {
			fmt.Println(ui.NoteStyle.Render("  No install history yet. trackr starts tracking from first use."))
			fmt.Println()
			return nil
		}

		for _, in := range installs {
			date := in.InstalledAt
			if len(date) >= 10 {
				date = date[:10]
			}
			why := in.WhyTag
			if why == "" {
				why = "untagged"
			}
			fmt.Printf("  %-12s %-8s %-16s %s\n", date, in.Tool, trunc(in.Name, 16), why)
		}
		fmt.Println()
		return nil
	},
}

func repeat(n int) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = '─'
	}
	return string(s)
}
