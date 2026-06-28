package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"trackr/internal/filesystem"
	"trackr/internal/model"
	"trackr/internal/registry"
	"trackr/internal/ui"
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Safely uninstall a package or program (always dry-runs first)",
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

		chosen, err := selectMatch(matches, "remove")
		if err != nil {
			return err
		}
		if chosen == nil {
			return nil
		}
		return removeItem(*chosen)
	},
}

// dirTarget is a folder slated for deletion along with its size.
type dirTarget struct {
	path string
	size int64
}

func removeItem(it model.Item) error {
	if it.StoreApp {
		fmt.Printf("\n  %s is a Windows Store app.\n  Remove it with:  winget uninstall %q\n\n", it.Name, it.Name)
		return nil
	}

	// Build the plan.
	var uninstallArgv []string
	var isMsi bool
	if it.Source == model.SourceExe && it.UninstallString != "" {
		uninstallArgv, isMsi = parseUninstall(it.UninstallString)
	}

	var dirs []dirTarget
	var warnings []string
	if it.InstallDir != "" && filesystem.Exists(it.InstallDir) {
		if prot, reason := isProtectedPath(it.InstallDir); prot {
			warnings = append(warnings, fmt.Sprintf("Will NOT delete %s (%s)", it.InstallDir, reason))
		} else {
			sz, _ := filesystem.DirSize(it.InstallDir)
			dirs = append(dirs, dirTarget{it.InstallDir, sz})
		}
	}

	regKey := ""
	if it.RegistryKey != "" && registry.IsUninstallKey(it.RegistryKey) {
		regKey = it.RegistryKey
	}

	// Detect upfront whether registry key removal will need elevation.
	// Used to warn in dry-run and hard-block before execution.
	needsAdmin := false
	if regKey != "" && registry.IsHKLM(regKey) {
		if err := registry.CanWrite(regKey); err != nil {
			needsAdmin = true
		}
	}

	// ----- Dry run (always shown first) -----
	fmt.Println()
	fmt.Println(ui.TitleStyle.Render(fmt.Sprintf("  trackr remove %s", it.Name)))
	fmt.Println(ui.YellowStyle.Render("  DRY RUN — nothing deleted yet"))
	fmt.Println(ui.DividerStyle.Render("  " + strings.Repeat("─", 58)))

	var freed int64
	switch {
	case len(it.RemoveArgs) > 0: // package manager item
		fmt.Printf("  Will run:     %s\n", strings.Join(it.RemoveArgs, " "))
		freed += it.SizeBytes
	case len(uninstallArgv) > 0:
		fmt.Printf("  Will run:     %s\n", strings.Join(uninstallArgv, " "))
	default:
		fmt.Println("  Will run:     (no uninstaller — manual file/key cleanup only)")
	}
	for _, d := range dirs {
		fmt.Printf("  Will delete:  %s   %s\n", d.path, model.FormatSize(d.size))
		freed += d.size
	}
	if regKey != "" {
		fmt.Printf("  Registry key: %s  (will remove)\n", regKey)
	}
	if needsAdmin {
		fmt.Println(ui.YellowStyle.Render("  ⚠ Registry key requires administrator rights."))
		fmt.Println(ui.YellowStyle.Render("    Run trackr from an elevated terminal, or the key will not be removed."))
	}
	for _, w := range warnings {
		fmt.Println(ui.OrphanStyle.Render("  " + w))
	}
	fmt.Printf("\n  Total freed: ~%s\n\n", model.FormatSize(freed))

	if !confirm("  Proceed? [y/N]") {
		fmt.Println("  Cancelled — nothing was changed.")
		return nil
	}

	// ----- Execution -----
	fmt.Println()

	// Hard block: if registry key needs admin and we cannot get write access,
	// stop before touching anything. User must re-run elevated.
	if needsAdmin {
		fmt.Println(ui.OrphanStyle.Render("  ! Registry key is under HKLM — administrator rights required."))
		fmt.Println(ui.OrphanStyle.Render("    Re-run trackr from an elevated terminal (right-click → Run as administrator)."))
		fmt.Println(ui.OrphanStyle.Render("    Nothing was changed."))
		return nil
	}

	var actualFreed int64
	switch {
	case len(it.RemoveArgs) > 0:
		out, err := runArgv(it.RemoveArgs)
		if err != nil {
			fmt.Printf("  ✗ Uninstall command failed: %v\n", err)
			if strings.TrimSpace(out) != "" {
				fmt.Println(indent(out))
			}
			return nil
		}
		fmt.Printf("  ✓ Uninstalled %s\n", it.Name)
		actualFreed += it.SizeBytes

	case len(uninstallArgv) > 0:
		_ = isMsi
		out, err := runArgv(uninstallArgv)
		if err != nil {
			if needsElevation(out, err) {
				fmt.Println(ui.OrphanStyle.Render("  ✗ Administrator rights required. Re-run trackr from an elevated terminal."))
				return nil
			}
			fmt.Printf("  ✗ Uninstaller returned an error: %v\n", err)
		} else {
			fmt.Printf("  ✓ Uninstalled %s\n", it.Name)
		}
	}

	// Admin pre-check: removing an HKLM uninstall key requires elevation. If we
	// cannot obtain write access, stop before deleting any folders — the user
	// will need to re-run the whole command from an elevated terminal anyway.
	if regKey != "" && registry.IsHKLM(regKey) {
		if err := registry.CanWrite(regKey); err != nil {
			fmt.Println(ui.OrphanStyle.Render("  ! Registry key is under HKLM — administrator rights required."))
			fmt.Println(ui.OrphanStyle.Render("    Re-run trackr from an elevated terminal (right-click → Run as administrator)."))
			return nil
		}
	}

	// Folder cleanup.
	for _, d := range dirs {
		if !filesystem.Exists(d.path) {
			fmt.Printf("  ✓ Folder already gone: %s\n", d.path)
			actualFreed += d.size
			continue
		}
		// For registry-based uninstalls the uninstaller may have left files.
		prompt := fmt.Sprintf("  Delete remaining folder %s? [y/N]", d.path)
		if it.Source == model.SourceFolder {
			// Pure folder ghost — main confirm already covers it.
			if deleteDir(d.path) {
				actualFreed += d.size
			}
			continue
		}
		if confirm(prompt) {
			if deleteDir(d.path) {
				actualFreed += d.size
			}
		}
	}

	// Registry key removal.
	if regKey != "" {
		if err := registry.DeleteKey(regKey); err != nil {
			fmt.Printf("  ! Could not remove registry key (%v)\n", err)
		} else {
			fmt.Println("  ✓ Removed registry key")
		}
	}

	fmt.Printf("  ✓ Freed ~%s\n\n", model.FormatSize(actualFreed))
	return nil
}

func deleteDir(path string) bool {
	if prot, reason := isProtectedPath(path); prot {
		fmt.Printf("  ! Skipped protected path %s (%s)\n", path, reason)
		return false
	}
	if err := os.RemoveAll(path); err != nil {
		fmt.Printf("  ✗ Failed to delete %s: %v\n", path, err)
		return false
	}
	fmt.Printf("  ✓ Deleted %s\n", path)
	return true
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("      " + line + "\n")
	}
	return b.String()
}
