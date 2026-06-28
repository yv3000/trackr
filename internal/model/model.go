// Package model holds the shared data types used across every trackr package.
// Keeping these here (with no internal dependencies) avoids import cycles
// between registry, pkgmanager, filesystem, orphan, db and ui.
package model

import "fmt"

// Tool identifiers.
const (
	ToolPip    = "pip"
	ToolNpm    = "npm"
	ToolYarn   = "yarn"
	ToolPnpm   = "pnpm"
	ToolDocker = "docker"
	ToolExe    = "exe"
	ToolFolder = "folder"
)

// Source identifiers describe where the item was discovered.
const (
	SourcePkg    = "pkg"    // a package-manager managed package
	SourceExe    = "exe"    // a registry (Add/Remove Programs) entry
	SourceFolder = "folder" // a raw folder found on disk
)

// Status / colour-coding hints consumed by the UI layer.
const (
	StatusClean       = "clean"        // green  - has uninstall path, no issues
	StatusNoUninstall = "no-uninstall" // yellow - no uninstall string found
	StatusOrphan      = "orphan"       // red    - registry/folder mismatch
	StatusPkg         = "pkg"          // dim    - package manager item
)

// Orphan classifications.
const (
	OrphanRegistryGhost = "registry-ghost" // registry entry, folder missing
	OrphanFolderGhost   = "folder-ghost"   // folder exists, no registry entry
)

// Item is the universal record for anything trackr can discover.
type Item struct {
	Name            string   `json:"name"`
	Tool            string   `json:"tool"`
	Version         string   `json:"version,omitempty"`
	Command         string   `json:"command,omitempty"`          // how it was installed / found
	RemoveArgs      []string `json:"remove_args,omitempty"`      // exact argv to uninstall (pkg items)
	InstallDir      string   `json:"install_dir,omitempty"`
	RegistryKey     string   `json:"registry_key,omitempty"`
	UninstallString string   `json:"uninstall_string,omitempty"`
	SizeBytes       int64    `json:"size_bytes"`
	Publisher       string   `json:"publisher,omitempty"`
	InstallDate     string   `json:"install_date,omitempty"`
	Status          string   `json:"status,omitempty"`
	Source          string   `json:"source"`
	OrphanType      string   `json:"orphan_type,omitempty"`
	StoreApp        bool     `json:"store_app,omitempty"`
}

// FormatSize renders a byte count as a human friendly string.
// A zero or negative value renders as an em dash.
func FormatSize(bytes int64) string {
	if bytes <= 0 {
		return "—"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}
