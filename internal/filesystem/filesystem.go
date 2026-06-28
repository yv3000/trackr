// Package filesystem provides directory size calculation, drive discovery and
// scanning of common Windows install locations. All operations are tolerant of
// permission errors and never follow symlinks (avoids infinite-loop risk).
package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"trackr/internal/model"
)

// foldersToSkip are top-level names under install roots that are never real
// "applications" and only add noise / false orphan positives.
var foldersToSkip = map[string]bool{
	"common files":           true,
	"windowsapps":            true,
	"modifiablewindowsapps":  true,
	"windows defender":       true,
	"windows nt":             true,
	"windows mail":           true,
	"windows photo viewer":   true,
	"windows portable devices": true,
	"internet explorer":      true,
	"windows media player":   true,
	"$recycle.bin":           true,
	"system volume information": true,
}

// DirSize returns the total size in bytes of every regular file under path.
// Symlinks are skipped (both symlinked dirs are not descended into by WalkDir,
// and symlinked files are ignored). Access-denied errors are swallowed so a
// single unreadable subfolder never aborts the whole calculation.
func DirSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permission denied / transient errors: skip this entry, keep going.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		// Skip symlinks (and other non-regular files) entirely.
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// AvailableDrives returns mounted drive letters as "C:", "D:" ...
func AvailableDrives() []string {
	var drives []string
	for c := 'A'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err == nil {
			drives = append(drives, string(c)+":")
		}
	}
	return drives
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		k := strings.ToLower(s)
		if s == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

// CommonInstallRoots returns every directory trackr scans for installed apps,
// including Program Files on all mounted drives plus per-user locations.
func CommonInstallRoots() []string {
	var roots []string
	add := func(p string) {
		if p != "" {
			roots = append(roots, p)
		}
	}

	add(os.Getenv("ProgramFiles"))
	add(os.Getenv("ProgramFiles(x86)"))

	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		add(filepath.Join(local, "Programs"))
		add(local)
		add(filepath.Join(local, "Microsoft", "WindowsApps"))
	}
	add(os.Getenv("APPDATA"))

	// Program Files on every other drive (D:\, E:\ ...).
	for _, d := range AvailableDrives() {
		add(filepath.Join(d+`\`, "Program Files"))
		add(filepath.Join(d+`\`, "Program Files (x86)"))
	}

	return dedupe(roots)
}

// ScanFolders walks the common install roots one level deep and returns each
// top-level folder as a model.Item with its computed on-disk size. Returns a
// slice of human-readable notes for any roots that could not be read.
func ScanFolders() ([]model.Item, []string) {
	var items []model.Item
	var notes []string
	seen := map[string]bool{}

	for _, root := range CommonInstallRoots() {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsPermission(err) {
				notes = append(notes, "Access denied: "+root+", skipped")
			}
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if foldersToSkip[strings.ToLower(e.Name())] {
				continue
			}
			full := filepath.Join(root, e.Name())
			key := strings.ToLower(full)
			if seen[key] {
				continue
			}
			seen[key] = true

			items = append(items, model.Item{
				Name:       e.Name(),
				Tool:       model.ToolFolder,
				Source:     model.SourceFolder,
				InstallDir: full,
				StoreApp:   strings.Contains(key, "windowsapps"),
			})
		}
	}

	// Calculate folder sizes concurrently with a capped worker pool so the scan
	// does not block serially on dozens of large directories.
	sem := make(chan struct{}, 8)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sz, _ := DirSize(items[idx].InstallDir)
			mu.Lock()
			items[idx].SizeBytes = sz
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	return items, notes
}

// Exists reports whether the given path exists on disk.
func Exists(path string) bool {
	path = strings.Trim(path, `"`)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
