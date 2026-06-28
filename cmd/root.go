// Package cmd wires up the trackr cobra command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "trackr",
	Short: "trackr — discover, locate and uninstall everything on your Windows machine",
	Long: `trackr finds every package, global tool and EXE-based program installed on
your system, shows where it lives and how much disk it eats, and helps you
remove it cleanly — all from one terminal.`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
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
