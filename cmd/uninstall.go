package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"trackr/internal/ui"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove trackr from this system",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUninstall()
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}
	trackrDir := filepath.Join(home, ".trackr")
	binDir := filepath.Join(trackrDir, "bin")
	exePath := filepath.Join(binDir, "trackr.exe")
	dbPath := filepath.Join(trackrDir, "trackr.db")

	fmt.Println()
	fmt.Println("  trackr uninstall")
	fmt.Println("  ─────────────────────────────────")
	fmt.Println()
	fmt.Println("  This will remove:")
	fmt.Printf("    %s\n", exePath)
	fmt.Printf("    %s  (install history)\n", dbPath)
	fmt.Printf("    PATH entry: %s\n", binDir)
	fmt.Println()

	if !confirm("  Proceed? [y/N]") {
		fmt.Println("  Cancelled. Nothing removed.")
		return nil
	}

	// Step 1: Remove from User PATH first (always safe, no file lock issue).
	// removeFromUserPath edits HKCU\Environment directly; the live process
	// PATH is irrelevant to what persists for future shells.
	if err := removeFromUserPath(binDir); err != nil {
		fmt.Println(ui.OrphanStyle.Render("  ! Could not update PATH automatically: " + err.Error()))
		fmt.Println(ui.OrphanStyle.Render("    You may need to remove it manually from Environment Variables."))
	} else {
		fmt.Println("  ✓ Removed from PATH")
	}

	// Step 2: Remove the database (history), if present.
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Remove(dbPath); err != nil {
			fmt.Println(ui.OrphanStyle.Render("  ! Could not delete history DB: " + err.Error()))
		} else {
			fmt.Println("  ✓ Removed install history")
		}
	}

	// Step 3: Self-delete the running exe.
	// Windows allows a running exe to be deleted while it's executing,
	// but the file handle stays locked until the process exits. We launch
	// a detached cmd.exe that waits briefly, then deletes the exe and the
	// (now empty) bin/.trackr folders, after this process has exited.
	fmt.Println("  Scheduling self-removal...")
	if err := scheduleSelfDelete(exePath, binDir, trackrDir); err != nil {
		fmt.Println(ui.OrphanStyle.Render("  ! Could not schedule exe removal: " + err.Error()))
		fmt.Println(ui.OrphanStyle.Render("    Delete manually: " + exePath))
		return nil
	}

	fmt.Println()
	fmt.Println("  ✓ trackr will finish uninstalling in a moment.")
	fmt.Println("  Goodbye!")
	fmt.Println()

	return nil
}
