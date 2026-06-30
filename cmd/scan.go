package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"trackr/internal/model"
	"trackr/internal/ui"
)

var (
	scanOrphans bool
	scanJSON    bool
	scanSort    string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Discover everything installed on the system",
	Long: `Scan package managers, the Windows registry and common install folders to
build a full picture of what is installed, where it lives and how big it is.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var res ui.ScanResult
		if scanJSON {
			// No UI in JSON mode — run the scan directly.
			ch := make(chan string, 32)
			go func() {
				for range ch {
				}
			}()
			res = runFullScan(ch)
			close(ch)
			return printJSON(res)
		}

		r, err := ui.RunScan(runFullScan)
		if err != nil {
			return err
		}
		res = r

		if scanOrphans {
			return showOrphans(res)
		}
		return showScan(res)
	},
}

func init() {
	scanCmd.Flags().BoolVar(&scanOrphans, "orphans", false, "show only orphaned installs")
	scanCmd.Flags().BoolVar(&scanJSON, "json", false, "output raw JSON (for scripting)")
	scanCmd.Flags().StringVar(&scanSort, "sort", "size", "sort order: size|name")
}

func printJSON(res ui.ScanResult) error {
	out := map[string]any{
		"package_managers": res.Pkg,
		"installed_software": res.Exe,
		"folders":            res.Folders,
		"registry_ghosts":    res.RegistryGhosts,
		"folder_ghosts":      res.FolderGhosts,
		"notes":              res.Notes,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func fmtPkgRow(it model.Item) string {
	return fmt.Sprintf("%-8s %-30s %-14s %10s",
		trunc(it.Tool, 8), trunc(it.Name, 30), trunc(it.Version, 14),
		model.FormatSize(it.SizeBytes))
}

func fmtExeRow(it model.Item) string {
	dir := it.InstallDir
	if dir == "" {
		dir = "(install folder unknown)"
	}
	tag := ""
	if it.StoreApp {
		tag = " [Store app — use winget]"
	} else if it.Status == model.StatusNoUninstall {
		tag = " [no uninstall string]"
	}
	if it.SystemComponent {
		tag += " [system component]"
	}
	return fmt.Sprintf("%-34s %-12s %10s  %s%s",
		trunc(it.Name, 34), trunc(it.Version, 12),
		model.FormatSize(it.SizeBytes), trunc(dir, 48), tag)
}

func showScan(res ui.ScanResult) error {
	pkg := sortItems(res.Pkg, scanSort)
	exe := sortItems(res.Exe, scanSort)

	var rows []ui.Row
	rows = append(rows,
		ui.Row{Header: true, Text: "PACKAGE MANAGERS"},
		ui.Row{Separator: true},
	)
	for i := range pkg {
		it := pkg[i]
		rows = append(rows, ui.Row{Item: &pkg[i], Tone: ui.TonePkg, Text: fmtPkgRow(it)})
	}
	if len(pkg) == 0 {
		rows = append(rows, ui.Row{Text: "(none found)", Tone: ui.TonePlain})
	}

	rows = append(rows,
		ui.Row{Text: ""},
		ui.Row{Header: true, Text: "INSTALLED SOFTWARE (EXE)"},
		ui.Row{Separator: true},
	)
	for i := range exe {
		it := exe[i]
		rows = append(rows, ui.Row{Item: &exe[i], Tone: ui.ToneForStatus(it.Status), Text: fmtExeRow(it)})
	}
	if len(exe) == 0 {
		rows = append(rows, ui.Row{Text: "(none found)", Tone: ui.TonePlain})
	}

	// Footer totals.
	var totalBytes int64
	for _, it := range pkg {
		totalBytes += it.SizeBytes
	}
	for _, it := range exe {
		totalBytes += it.SizeBytes
	}
	rows = append(rows,
		ui.Row{Text: ""},
		ui.Row{Header: true, Text: fmt.Sprintf("Total: %d packages · %d software installs · %s",
			len(pkg), len(exe), model.FormatSize(totalBytes))},
	)
	rows = appendNotes(rows, res)

	_, err := ui.RunList("trackr scan", rows, false)
	return err
}

func showOrphans(res ui.ScanResult) error {
	rg := sortItems(res.RegistryGhosts, scanSort)
	fg := sortItems(filterFolderGhosts(res.FolderGhosts, res.Pkg), scanSort)

	var rows []ui.Row
	rows = append(rows,
		ui.Row{Header: true, Text: "REGISTRY GHOSTS (safe to clean)"},
		ui.Row{Separator: true},
	)
	for i := range rg {
		it := rg[i]
		rows = append(rows, ui.Row{
			Item: &rg[i], Tone: ui.ToneOrphan,
			Text: fmt.Sprintf("! %-32s registry says %s but folder missing",
				trunc(it.Name, 32), it.InstallDir),
		})
	}
	if len(rg) == 0 {
		rows = append(rows, ui.Row{Text: "(none)", Tone: ui.TonePlain})
	}

	rows = append(rows,
		ui.Row{Text: ""},
		ui.Row{Header: true, Text: "FOLDER GHOSTS (verify before removing)"},
		ui.Row{Separator: true},
	)
	for i := range fg {
		it := fg[i]
		rows = append(rows, ui.Row{
			Item: &fg[i], Tone: ui.ToneOrphan,
			Text: fmt.Sprintf("? %-32s %-48s %10s  no registry entry found",
				trunc(it.Name, 32), trunc(it.InstallDir, 48),
				model.FormatSize(it.SizeBytes)),
		})
	}
	if len(fg) == 0 {
		rows = append(rows, ui.Row{Text: "(none)", Tone: ui.TonePlain})
	}

	rows = append(rows,
		ui.Row{Text: ""},
		ui.Row{Header: true, Text: "Run: trackr remove <name>  to clean any of these"},
	)
	rows = appendNotes(rows, res)

	_, err := ui.RunList("trackr scan --orphans", rows, false)
	return err
}

func appendNotes(rows []ui.Row, res ui.ScanResult) []ui.Row {
	if len(res.Notes) == 0 {
		return rows
	}
	rows = append(rows, ui.Row{Text: ""})
	for _, n := range res.Notes {
		rows = append(rows, ui.Row{Text: n, Tone: ui.TonePkg})
	}
	return rows
}
