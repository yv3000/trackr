// Package cmd wires up the trackr cobra command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"trackr/internal/ui"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:           "trackr",
	Short:         "caveman tool. me show you all thing. me find. me kill.",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ui.RunTUI(tuiHandlers())
	},
}

// tuiHandlers builds the dashboard callbacks from the existing command logic,
// so the TUI reuses the exact same scan/where/remove/log behaviour.
func tuiHandlers() ui.Handlers {
	return ui.Handlers{
		Scan: func() error {
			res, err := ui.RunScan(runFullScan)
			if err != nil {
				return err
			}
			return showScan(res)
		},
		Orphans: func() error {
			res, err := ui.RunScan(runFullScan)
			if err != nil {
				return err
			}
			return showOrphans(res)
		},
		Where: func(name string) error {
			res, err := ui.RunScan(runFullScan)
			if err != nil {
				return err
			}
			matches := matchItems(combinedItems(res), name)
			if len(matches) == 0 {
				fmt.Printf("\n  No matches found for %q. Try a scan first.\n", name)
				return nil
			}
			chosen, err := selectMatch(matches, "inspect")
			if err != nil {
				return err
			}
			if chosen != nil {
				printWhere(*chosen)
			}
			return nil
		},
		Remove: func(name string) error {
			res, err := ui.RunScan(runFullScan)
			if err != nil {
				return err
			}
			matches := matchItems(combinedItems(res), name)
			if len(matches) == 0 {
				fmt.Printf("\n  No matches found for %q. Try a scan first.\n", name)
				return nil
			}
			chosen, err := selectMatch(matches, "remove")
			if err != nil {
				return err
			}
			if chosen != nil {
				return removeItem(*chosen)
			}
			return nil
		},
		Log: func() error {
			return logCmd.RunE(logCmd, nil)
		},
	}
}

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(scanCmd, whereCmd, removeCmd, logCmd)
}
