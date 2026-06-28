package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"trackr/internal/filesystem"
	"trackr/internal/model"
	"trackr/internal/pkgmanager"
	"trackr/internal/ui"
)

var whereCmd = &cobra.Command{
	Use:   "where <name>",
	Short: "Find every location a package or program occupies",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		res, err := ui.RunScan(runFullScan)
		if err != nil {
			return err
		}

		matches := matchItems(combinedItems(res), query)
		if len(matches) == 0 {
			fmt.Printf("No matches found for %q. Try: trackr scan\n", query)
			return nil
		}

		chosen, err := selectMatch(matches, "inspect")
		if err != nil {
			return err
		}
		if chosen == nil {
			return nil
		}
		printWhere(*chosen)
		return nil
	},
}

func printWhere(it model.Item) {
	title := it.Name
	if it.Version != "" {
		title += "  v" + it.Version
	}
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render("  " + title))
	fmt.Println(ui.DividerStyle.Render("  " + strings.Repeat("─", 58)))

	var total int64
	line := func(label, value string) {
		fmt.Printf("  %-15s %s\n", label, value)
	}

	if it.RegistryKey != "" {
		line("Registry key", it.RegistryKey)
	}
	if it.InstallDir != "" {
		size := int64(0)
		if filesystem.Exists(it.InstallDir) {
			size, _ = filesystem.DirSize(it.InstallDir)
			total += size
		}
		line("Install folder", fmt.Sprintf("%s   %s", it.InstallDir, model.FormatSize(size)))
	}
	if it.UninstallString != "" {
		line("Uninstall cmd", it.UninstallString)
	}

	// Package-manager specific locations.
	switch it.Tool {
	case model.ToolPip:
		if loc := pkgmanager.PipLocation(it.Name); loc != "" {
			size, _ := filesystem.DirSize(loc)
			line("pip location", fmt.Sprintf("%s   %s", loc, model.FormatSize(size)))
		}
	case model.ToolNpm:
		if root := pkgmanager.NpmGlobalRoot(); root != "" {
			line("npm global", root)
		}
	}

	// PATH entries that reference this item.
	for _, p := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lp := strings.ToLower(p)
		if (it.InstallDir != "" && strings.Contains(lp, strings.ToLower(it.InstallDir))) ||
			strings.Contains(lp, strings.ToLower(it.Name)) {
			line("PATH entry", p)
		}
	}

	if total > 0 {
		fmt.Println()
		fmt.Printf("  %-15s ~%s\n", "Total footprint", model.FormatSize(total))
	}
	fmt.Println()
}
