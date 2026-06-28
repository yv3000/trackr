// Package orphan detects two kinds of leftover installs:
//
//	Registry ghost - a registry uninstall entry whose InstallLocation folder
//	                 no longer exists on disk (safe to clean).
//	Folder ghost   - a folder under a common install root with no matching
//	                 registry entry (lower confidence, verify before removing).
package orphan

import (
	"strings"

	"trackr/internal/filesystem"
	"trackr/internal/model"
)

// Detect classifies orphans from the registry items and folder items produced
// by the registry and filesystem scanners.
func Detect(regItems, folderItems []model.Item) (registryGhosts, folderGhosts []model.Item) {
	// Type A — registry ghosts.
	for _, it := range regItems {
		loc := strings.Trim(it.InstallDir, `"`)
		if loc == "" || it.StoreApp {
			continue
		}
		if !filesystem.Exists(loc) {
			g := it
			g.OrphanType = model.OrphanRegistryGhost
			g.Status = model.StatusOrphan
			registryGhosts = append(registryGhosts, g)
		}
	}

	// Type B — folder ghosts.
	for _, f := range folderItems {
		if f.StoreApp {
			continue
		}
		if hasRegistryMatch(f.Name, regItems) {
			continue
		}
		g := f
		g.OrphanType = model.OrphanFolderGhost
		g.Status = model.StatusOrphan
		folderGhosts = append(folderGhosts, g)
	}
	return
}

// hasRegistryMatch reports whether folderName fuzzily matches any registry
// DisplayName (either contains the other, case-insensitively). It also matches
// against the publisher/last path segment of install locations.
func hasRegistryMatch(folderName string, regItems []model.Item) bool {
	fn := normalize(folderName)
	if fn == "" {
		return true // treat unnameable folders as matched (skip)
	}
	for _, r := range regItems {
		rn := normalize(r.Name)
		if rn == "" {
			continue
		}
		if strings.Contains(rn, fn) || strings.Contains(fn, rn) {
			return true
		}
		// Compare against the install folder's leaf name too.
		if loc := strings.Trim(r.InstallDir, `"`); loc != "" {
			leaf := normalize(lastSegment(loc))
			if leaf != "" && (strings.Contains(leaf, fn) || strings.Contains(fn, leaf)) {
				return true
			}
		}
	}
	return false
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Drop characters that vary between folder names and display names.
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", "(x86)", "", "®", "", "™", "")
	return replacer.Replace(s)
}

func lastSegment(path string) string {
	path = strings.TrimRight(path, `\/`)
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}
